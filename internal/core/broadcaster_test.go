package core

import (
	"context"
	"sync"
	"testing"

	"github.com/coso/qcloop/internal/executor"
)

// fakeBroadcaster 记录所有广播调用,用于断言。
type fakeBroadcaster struct {
	mu        sync.Mutex
	itemCalls []fakeItemCall
	jobCalls  []fakeJobCall
}

type fakeItemCall struct {
	JobID, ItemID string
}
type fakeJobCall struct {
	JobID string
}

func (f *fakeBroadcaster) BroadcastItemUpdate(jobID, itemID string, _ interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.itemCalls = append(f.itemCalls, fakeItemCall{jobID, itemID})
}
func (f *fakeBroadcaster) BroadcastJobUpdate(jobID string, _ interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobCalls = append(f.jobCalls, fakeJobCall{jobID})
}
func (f *fakeBroadcaster) itemCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.itemCalls)
}
func (f *fakeBroadcaster) jobCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobCalls)
}
func (f *fakeBroadcaster) jobIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.jobCalls))
	for i, c := range f.jobCalls {
		out[i] = c.JobID
	}
	return out
}

// 注入 broadcaster 后,无 verifier 情况下每个 item 至少产生两次广播:
// 一次 attempt 落库(成功),一次终态(success)。整个 RunBatch 还会广播
// 两次 job 状态(running → completed)。
func TestBroadcaster_NoVerifier_EmitsOnAttemptAndTerminal(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 3, []string{"a", "b"})

	exec := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "ok", ExitCode: 0},
		executor.FakeResponse{Stdout: "ok", ExitCode: 0},
	)
	r := NewRunnerWithExecutor(database, exec)
	bc := &fakeBroadcaster{}
	r.SetBroadcaster(bc)

	if err := r.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	// 2 items × (1 attempt + 1 终态) = 4 次 item 广播
	if got := bc.itemCallCount(); got != 4 {
		t.Errorf("item broadcast count = %d, want 4", got)
	}
	// job 状态:running(开始) + completed(结束) = 2 次
	if got := bc.jobCallCount(); got != 2 {
		t.Errorf("job broadcast count = %d, want 2", got)
	}
	for _, id := range bc.jobIDs() {
		if id != jobID {
			t.Errorf("job broadcast carried wrong id: %s (want %s)", id, jobID)
		}
	}
}

// 有 verifier 首轮通过:每个 item 的广播流应为:
//   worker attempt → qc_round → item 终态 success
// 共 3 次 item 广播。
func TestBroadcaster_VerifierFirstPass_EmitsThreeTimesPerItem(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "verify", 3, []string{"x"})

	exec := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "done", ExitCode: 0},                // worker
		executor.FakeResponse{Stdout: `{"pass":true,"feedback":"ok"}`, ExitCode: 0}, // verifier
	)
	r := NewRunnerWithExecutor(database, exec)
	bc := &fakeBroadcaster{}
	r.SetBroadcaster(bc)

	if err := r.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	if got := bc.itemCallCount(); got != 3 {
		t.Errorf("item broadcast count = %d, want 3 (worker+qc+terminal)", got)
	}
}

// 未注入 broadcaster(nil)时,Runner 必须保持原有行为,不 panic、不阻塞。
// 同时证明旧测试不受影响的机制是对的。
func TestBroadcaster_NilDoesNotPanic(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 3, []string{"a"})

	r := NewRunnerWithExecutor(database, executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "ok", ExitCode: 0},
	))
	// 故意不 SetBroadcaster

	if err := r.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch should not fail without broadcaster: %v", err)
	}
}
