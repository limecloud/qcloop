package core

import (
	"context"
	"strings"
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

func TestQueueManagerMarksJobFailedWhenAnyItemFails(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"bad"})
	fake := executor.NewFakeExecutor(executor.FakeResponse{Stdout: "", Stderr: "test failed", ExitCode: 1})
	runner := NewRunnerWithExecutor(database, fake)
	queue := NewQueueManager(database, runner, QueueOptions{
		WorkerCount:   1,
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
	waitForJobStatus(t, database, jobID, "failed", time.Second)

	items, _ := database.ListItems(jobID)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Status != "failed" {
		t.Fatalf("item status = %s, want failed", items[0].Status)
	}
}

func TestPrepareRunModeRetryUnfinishedKeepsSuccessAndQueuesOthers(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"ok", "bad", "spent", "confirm"})
	items, _ := database.ListItems(jobID)
	_ = database.FinishItem(items[0].ID, "success")
	_ = database.FinishItem(items[1].ID, "failed")
	_ = database.FinishItem(items[2].ID, "exhausted")
	_ = database.MarkItemAwaitingConfirmation(items[3].ID, "需要确认")
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
	if statuses["bad"] != "pending" || statuses["spent"] != "pending" || statuses["confirm"] != "pending" {
		t.Fatalf("unfinished statuses = bad:%s spent:%s confirm:%s, want pending/pending/pending", statuses["bad"], statuses["spent"], statuses["confirm"])
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

func TestQueueManagerCancelItemMarksFailedJobAndDoesNotRequeue(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"slow"})
	fake := &blockingExecutor{delay: 200 * time.Millisecond}
	runner := NewRunnerWithExecutor(database, fake)
	queue := NewQueueManager(database, runner, QueueOptions{
		WorkerCount:   1,
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
	item := waitForFirstItemStatus(t, database, jobID, "running", time.Second)
	fresh, err := queue.CancelItem(item.ID, "用户跳过该项")
	if err != nil {
		t.Fatalf("CancelItem: %v", err)
	}
	if fresh.Status != "canceled" {
		t.Fatalf("fresh status = %s, want canceled", fresh.Status)
	}
	waitForJobStatus(t, database, jobID, "failed", time.Second)
	fresh, _ = database.GetItem(item.ID)
	if fresh.Status != "canceled" {
		t.Fatalf("item status after runner returned = %s, want canceled", fresh.Status)
	}
	if fresh.LastError != "用户跳过该项" {
		t.Fatalf("last_error = %q, want cancel reason", fresh.LastError)
	}
}

func TestQueueManagerMetricsExposeActiveAndPersistedCounts(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"a", "b"})
	fake := &blockingExecutor{delay: 150 * time.Millisecond}
	runner := NewRunnerWithExecutor(database, fake)
	queue := NewQueueManager(database, runner, QueueOptions{
		WorkerCount:   1,
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
	_ = waitForFirstItemStatus(t, database, jobID, "running", time.Second)
	metrics, err := queue.Metrics()
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.WorkerCount != 1 || metrics.LeaseDurationSeconds != 1 {
		t.Fatalf("queue options not reflected: %#v", metrics)
	}
	if metrics.ActiveItems != 1 || metrics.ActiveJobs != 1 {
		t.Fatalf("active counts = items:%d jobs:%d, want 1/1", metrics.ActiveItems, metrics.ActiveJobs)
	}
	if metrics.Items["total_items"] != 2 || metrics.Items["running_items"] != 1 || metrics.Items["pending_items"] != 1 {
		t.Fatalf("item metrics = %#v, want total=2 running=1 pending=1", metrics.Items)
	}
}

func TestQueueManagerRejectsCancelTerminalJob(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"done"})
	if err := database.FinishJob(jobID, "completed"); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
	queue := NewQueueManager(database, NewRunnerWithExecutor(database, executor.NewFakeExecutor()), QueueOptions{})

	err := queue.CancelJob(jobID, "too late")
	if err == nil || !strings.Contains(err.Error(), "terminal job cannot be canceled") {
		t.Fatalf("CancelJob err = %v, want terminal guard", err)
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

func waitForFirstItemStatus(t *testing.T, database *db.DB, jobID, status string, timeout time.Duration) *db.BatchItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		items, err := database.ListItems(jobID)
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		for _, item := range items {
			if item.Status == status {
				return item
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	items, _ := database.ListItems(jobID)
	statuses := make([]string, 0, len(items))
	for _, item := range items {
		statuses = append(statuses, item.Status)
	}
	t.Fatalf("no item status = %s within %s; statuses=%v", status, timeout, statuses)
	return nil
}
