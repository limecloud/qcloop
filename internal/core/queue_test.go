package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

type blockingExecutor struct {
	delay time.Duration

	mu        sync.Mutex
	active    int
	maxActive int
	calls     int
}

func (e *blockingExecutor) Execute(ctx context.Context, prompt string) (string, string, int, error) {
	res := e.ExecuteWithTokens(ctx, prompt)
	return res.Stdout, res.Stderr, res.ExitCode, nil
}

func (e *blockingExecutor) ExecuteWithTokens(ctx context.Context, prompt string) executor.Result {
	e.mu.Lock()
	e.active++
	e.calls++
	if e.active > e.maxActive {
		e.maxActive = e.active
	}
	e.mu.Unlock()

	select {
	case <-ctx.Done():
	case <-time.After(e.delay):
	}

	e.mu.Lock()
	e.active--
	e.mu.Unlock()
	return executor.Result{Stdout: "ok: " + prompt, ExitCode: 0}
}

func (e *blockingExecutor) snapshot() (calls, maxActive int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls, e.maxActive
}

func TestQueueManagerProcessesItemsWithGlobalWorkers(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"a", "b", "c", "d"})
	fake := &blockingExecutor{delay: 40 * time.Millisecond}
	runner := NewRunnerWithExecutor(database, fake)
	queue := NewQueueManager(database, runner, QueueOptions{
		WorkerCount:   2,
		LeaseDuration: time.Second,
		PollInterval:  5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue.Start(ctx)
	defer queue.Stop()

	if _, err := queue.EnqueueJob(jobID, RunModeAuto); err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}
	waitForJobStatus(t, database, jobID, "completed", time.Second)

	calls, maxActive := fake.snapshot()
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
	if maxActive < 2 {
		t.Fatalf("maxActive = %d, want at least 2", maxActive)
	}
	items, _ := database.ListItems(jobID)
	for _, item := range items {
		if item.Status != "success" {
			t.Fatalf("item %s status = %s, want success", item.ItemValue, item.Status)
		}
	}
}

func TestPrepareRunModeRetryUnfinishedKeepsSuccessAndQueuesOthers(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"ok", "bad", "spent"})
	items, _ := database.ListItems(jobID)
	_ = database.FinishItem(items[0].ID, "success")
	_ = database.FinishItem(items[1].ID, "failed")
	_ = database.FinishItem(items[2].ID, "exhausted")
	_ = database.FinishJob(jobID, "completed")

	runner := NewRunnerWithExecutor(database, executor.NewFakeExecutor())
	job, prepared, err := runner.PrepareRunMode(jobID, RunModeRetryUnfinished)
	if err != nil {
		t.Fatalf("PrepareRunMode: %v", err)
	}
	if job.RunNo != 2 {
		t.Fatalf("run_no = %d, want 2", job.RunNo)
	}

	statuses := map[string]string{}
	for _, item := range prepared {
		statuses[item.ItemValue] = item.Status
		if item.CurrentAttemptNo != 0 || item.CurrentQCNo != 0 || item.TokensUsed != 0 {
			t.Fatalf("item %s counters not reset: attempt=%d qc=%d tokens=%d", item.ItemValue, item.CurrentAttemptNo, item.CurrentQCNo, item.TokensUsed)
		}
	}
	if statuses["ok"] != "success" {
		t.Fatalf("success item status = %s, want success", statuses["ok"])
	}
	if statuses["bad"] != "pending" || statuses["spent"] != "pending" {
		t.Fatalf("unfinished statuses = bad:%s spent:%s, want pending/pending", statuses["bad"], statuses["spent"])
	}
}

func TestRecoverStaleRunningItemsRequeuesExpiredLease(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"stale"})
	items, _ := database.ListItems(jobID)
	itemID := items[0].ID
	_ = database.StartJob(jobID)
	expiredAt := time.Now().Add(-time.Minute).Format(time.RFC3339)
	_, err := database.Conn().Exec(`UPDATE batch_items SET status = 'running', lock_owner = 'dead-worker', lock_expires_at = ? WHERE id = ?`, expiredAt, itemID)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}

	recovered, err := database.RecoverStaleRunningItems(time.Now())
	if err != nil {
		t.Fatalf("RecoverStaleRunningItems: %v", err)
	}
	if len(recovered) != 1 || recovered[0] != itemID {
		t.Fatalf("recovered = %#v, want [%s]", recovered, itemID)
	}
	item, _ := database.GetItem(itemID)
	if item.Status != "pending" {
		t.Fatalf("status = %s, want pending", item.Status)
	}
	if item.LockOwner != "" || item.LockExpiresAt != nil {
		t.Fatalf("lease not cleared: owner=%q expires=%v", item.LockOwner, item.LockExpiresAt)
	}
	if item.LastError == "" {
		t.Fatal("last_error should explain stale recovery")
	}
}

func waitForJobStatus(t *testing.T, database *db.DB, jobID, status string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := database.GetJob(jobID)
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if job != nil && job.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := database.GetJob(jobID)
	if job == nil {
		t.Fatalf("job not found")
	}
	t.Fatalf("job status = %s, want %s", job.Status, status)
}
