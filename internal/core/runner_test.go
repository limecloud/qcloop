package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// newTestDB 在 t.TempDir() 下开一个干净 sqlite,测试结束自动清理
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "qcloop-test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_ = os.Remove(path)
	})
	return database
}

// makeJob 在 DB 里落一个最小 job + items,返回 jobID
func makeJob(t *testing.T, database *db.DB, verifierPrompt string, maxRounds int, items []string) string {
	t.Helper()
	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:                     jobID,
		Name:                   "test-" + t.Name(),
		PromptTemplate:         "do {{item}}",
		VerifierPromptTemplate: verifierPrompt,
		MaxQCRounds:            maxRounds,
		Status:                 "pending",
		CreatedAt:              time.Now(),
	}
	if err := database.CreateJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	for _, v := range items {
		it := &db.BatchItem{
			ID:         db.GenerateID(),
			BatchJobID: jobID,
			ItemValue:  v,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}
		if err := database.CreateItem(it); err != nil {
			t.Fatalf("create item: %v", err)
		}
	}
	return jobID
}

// ---------- 测试用例 ----------

// 无 verifier: 成功的 worker 应直接把 item 标记为 success
func TestRunBatch_NoVerifier_AllSuccess(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 3, []string{"a", "b", "c"})

	fake := executor.NewFakeExecutor() // 默认全部成功
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	// 3 个 item 各执行一次 worker
	if fake.CallCount() != 3 {
		t.Errorf("expected 3 executor calls, got %d", fake.CallCount())
	}

	items, _ := database.ListItems(jobID)
	for _, it := range items {
		if it.Status != "success" {
			t.Errorf("item %s: want status=success, got %s", it.ItemValue, it.Status)
		}
	}

	job, _ := database.GetJob(jobID)
	if job.Status != "completed" {
		t.Errorf("job status: want completed, got %s", job.Status)
	}
}

// 无 verifier: worker 非 0 退出应标记 failed
func TestRunBatch_NoVerifier_WorkerFails(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 3, []string{"a"})

	fake := executor.NewFakeExecutor(executor.FakeResponse{
		Stdout: "", Stderr: "boom", ExitCode: 1,
	})
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	items, _ := database.ListItems(jobID)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Status != "failed" {
		t.Errorf("want failed, got %s", items[0].Status)
	}
	job, _ := database.GetJob(jobID)
	if job.Status != "failed" {
		t.Errorf("job status: want failed when item failed, got %s", job.Status)
	}
}

// 有 verifier 且首轮就 pass: 应该只跑 worker + 1 轮 verifier
func TestRunBatch_Verifier_FirstRoundPass(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 3, []string{"x"})

	fake := executor.NewFakeExecutor(
		// worker
		executor.FakeResponse{Stdout: "worker done", ExitCode: 0},
		// verifier (JSON pass=true)
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok"}`, ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	if fake.CallCount() != 2 {
		t.Errorf("want 2 calls (worker+verifier), got %d", fake.CallCount())
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "success" {
		t.Errorf("want success, got %s", items[0].Status)
	}
}

// verifier 前 2 轮 fail、第 3 轮 pass: 应 repair 2 次、verifier 3 次,最终 success
func TestRunBatch_Verifier_RepairThenPass(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 3, []string{"y"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "w1", ExitCode: 0},                                   // worker 1
		executor.FakeResponse{Stdout: `{"pass": false, "feedback": "fix x"}`, ExitCode: 0}, // qc 1 fail
		executor.FakeResponse{Stdout: "w2", ExitCode: 0},                                   // repair 1
		executor.FakeResponse{Stdout: `{"pass": false, "feedback": "fix y"}`, ExitCode: 0}, // qc 2 fail
		executor.FakeResponse{Stdout: "w3", ExitCode: 0},                                   // repair 2
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok"}`, ExitCode: 0},     // qc 3 pass
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	if fake.CallCount() != 6 {
		t.Errorf("want 6 calls (1 worker + 2 repair + 3 verifier), got %d", fake.CallCount())
	}
	calls := fake.Calls()
	if !strings.Contains(calls[2].Prompt, "质检反馈：fix x") {
		t.Errorf("repair prompt missing qc feedback: %q", calls[2].Prompt)
	}
	if !strings.Contains(calls[4].Prompt, "质检反馈：fix y") {
		t.Errorf("second repair prompt missing qc feedback: %q", calls[4].Prompt)
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "success" {
		t.Errorf("want success after repair, got %s", items[0].Status)
	}
}

// verifier 始终 fail 且达到 max_qc_rounds: 应标记 exhausted,不再继续调用
func TestRunBatch_Verifier_Exhausted(t *testing.T) {
	database := newTestDB(t)
	const maxRounds = 2
	jobID := makeJob(t, database, `check {{item}}`, maxRounds, []string{"z"})

	// 构造足够多的 fail 响应,实际不会全部用到
	failVerdict := executor.FakeResponse{Stdout: `{"pass": false, "feedback": "no"}`, ExitCode: 0}
	okWorker := executor.FakeResponse{Stdout: "w", ExitCode: 0}
	fake := executor.NewFakeExecutor(
		okWorker, failVerdict,
		okWorker, failVerdict,
		okWorker, failVerdict,
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "exhausted" {
		t.Errorf("want exhausted after %d rounds, got %s", maxRounds, items[0].Status)
	}
	job, _ := database.GetJob(jobID)
	if job.Status != "failed" {
		t.Errorf("job status: want failed when item exhausted, got %s", job.Status)
	}
}

func TestRunBatch_VerifierNeedsConfirmationStopsItem(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 3, []string{"risky-change"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "worker done", ExitCode: 0},
		executor.FakeResponse{Stdout: `{"pass": false, "needs_confirmation": true, "question": "是否允许修改生成文件？", "feedback": "需要确认风险边界"}`, ExitCode: 0},
		executor.FakeResponse{Stdout: "should-not-repair", ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	if fake.CallCount() != 2 {
		t.Fatalf("call count = %d, want worker + verifier only", fake.CallCount())
	}
	items, _ := database.ListItems(jobID)
	if items[0].Status != "awaiting_confirmation" {
		t.Fatalf("item status = %s, want awaiting_confirmation", items[0].Status)
	}
	if !strings.Contains(items[0].ConfirmationQuestion, "是否允许修改生成文件") {
		t.Fatalf("confirmation question = %q", items[0].ConfirmationQuestion)
	}
	job, _ := database.GetJob(jobID)
	if job.Status != "waiting_confirmation" {
		t.Fatalf("job status = %s, want waiting_confirmation", job.Status)
	}
}

func TestRunBatch_AnsweredConfirmationIsInjectedIntoPrompt(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"doc-a"})
	items, _ := database.ListItems(jobID)
	if err := database.MarkItemAwaitingConfirmation(items[0].ID, "是否允许修改文档？"); err != nil {
		t.Fatalf("MarkItemAwaitingConfirmation: %v", err)
	}
	if err := database.AnswerItemConfirmation(items[0].ID, "允许修改文档，但不要提交。", true); err != nil {
		t.Fatalf("AnswerItemConfirmation: %v", err)
	}
	if err := database.StartJob(jobID); err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	fake := executor.NewFakeExecutor(executor.FakeResponse{Stdout: "ok", ExitCode: 0})
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if !strings.Contains(calls[0].Prompt, "qcloop 确认上下文") || !strings.Contains(calls[0].Prompt, "允许修改文档") {
		t.Fatalf("prompt missing confirmation context: %q", calls[0].Prompt)
	}
}

func TestGetJobReconcilesCompletedJobWithExhaustedItems(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 1, []string{"ok", "spent"})
	items, _ := database.ListItems(jobID)
	_ = database.FinishItem(items[0].ID, "success")
	_ = database.FinishItem(items[1].ID, "exhausted")
	_ = database.FinishJob(jobID, "completed")

	job, err := database.GetJob(jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.Status != "failed" {
		t.Fatalf("job status = %s, want failed", job.Status)
	}
	if job.FinishedAt == nil {
		t.Fatal("reconciled terminal job should keep or set finished_at")
	}
}

func TestRunBatch_VerifierReceivesAttemptEvidencePlaceholders(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `item={{item}} stdout={{stdout}} output={{output}} stderr={{stderr}} code={{exit_code}} status={{attempt_status}} type={{attempt_type}}`, 2, []string{"case-1"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "test stdout", Stderr: "test stderr", ExitCode: 7},
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok"}`, ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("want worker + verifier calls, got %d", len(calls))
	}
	verifierPrompt := calls[1].Prompt
	for _, want := range []string{
		"item=case-1",
		"stdout=test stdout",
		"output=test stdout",
		"stderr=test stderr",
		"code=7",
		"status=failed",
		"type=worker",
	} {
		if !strings.Contains(verifierPrompt, want) {
			t.Fatalf("verifier prompt missing %q: %s", want, verifierPrompt)
		}
	}
}

func TestRunBatch_RepairPromptIncludesPreviousAttemptEvidence(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 2, []string{"repairable"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "red test stdout", Stderr: "red test stderr", ExitCode: 3},
		executor.FakeResponse{Stdout: `{"pass": false, "feedback": "需要修复断言失败"}`, ExitCode: 0},
		executor.FakeResponse{Stdout: "green test stdout", ExitCode: 0},
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok"}`, ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	calls := fake.Calls()
	if len(calls) != 4 {
		t.Fatalf("want worker + verifier + repair + verifier calls, got %d", len(calls))
	}
	repairPrompt := calls[2].Prompt
	for _, want := range []string{
		"这是 qcloop 的 repair 轮次",
		"red test stdout",
		"red test stderr",
		"退出码: 3",
		"质检反馈：需要修复断言失败",
		"重新运行与该测试项相关的最小验证命令",
		"不要执行 git commit / git push / git reset",
	} {
		if !strings.Contains(repairPrompt, want) {
			t.Fatalf("repair prompt missing %q: %s", want, repairPrompt)
		}
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "success" {
		t.Fatalf("item status = %s, want success after repair", items[0].Status)
	}
	job, _ := database.GetJob(jobID)
	if job.Status != "completed" {
		t.Fatalf("job status = %s, want completed after repaired item passed", job.Status)
	}
}

func TestRunBatch_CompletedJobRerunsAllItemsAndAppendsHistory(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 1, []string{"again"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "first worker", ExitCode: 0},
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok first"}`, ExitCode: 0},
		executor.FakeResponse{Stdout: "second worker", ExitCode: 0},
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok second"}`, ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("first RunBatch: %v", err)
	}
	job, _ := database.GetJob(jobID)
	firstFinishedAt := job.FinishedAt
	if firstFinishedAt == nil {
		t.Fatal("first run should set finished_at")
	}

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("second RunBatch: %v", err)
	}
	if fake.CallCount() != 4 {
		t.Fatalf("want 4 calls after rerun, got %d", fake.CallCount())
	}

	items, _ := database.ListItems(jobID)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Status != "success" {
		t.Fatalf("item status = %s, want success", items[0].Status)
	}
	if items[0].CurrentAttemptNo != 1 {
		t.Fatalf("current_attempt_no = %d, want 1", items[0].CurrentAttemptNo)
	}
	if items[0].CurrentQCNo != 1 {
		t.Fatalf("current_qc_no = %d, want 1", items[0].CurrentQCNo)
	}
	if items[0].FinishedAt == nil {
		t.Fatal("rerun success should set item finished_at")
	}

	attempts, err := runner.listAttempts(items[0].ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("want 2 attempts preserved, got %d", len(attempts))
	}
	if attempts[0].AttemptNo != 1 || attempts[1].AttemptNo != 2 {
		t.Fatalf("attempt numbers = %d/%d, want 1/2", attempts[0].AttemptNo, attempts[1].AttemptNo)
	}

	rounds, err := runner.listQCRounds(items[0].ID)
	if err != nil {
		t.Fatalf("list qc rounds: %v", err)
	}
	if len(rounds) != 2 {
		t.Fatalf("want 2 qc rounds preserved, got %d", len(rounds))
	}
	if rounds[0].QCNo != 1 || rounds[1].QCNo != 2 {
		t.Fatalf("qc numbers = %d/%d, want 1/2", rounds[0].QCNo, rounds[1].QCNo)
	}
}

func TestPrepareRun_CompletedJobResetsVisibleItemStateBeforeWorkerStarts(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, `check {{item}}`, 1, []string{"again"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "first worker", ExitCode: 0},
		executor.FakeResponse{Stdout: `{"pass": true, "feedback": "ok first"}`, ExitCode: 0},
	)
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("first RunBatch: %v", err)
	}

	_, preparedItems, err := runner.PrepareRun(jobID)
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}
	if len(preparedItems) != 1 {
		t.Fatalf("want 1 prepared item, got %d", len(preparedItems))
	}
	if preparedItems[0].Status != "pending" {
		t.Fatalf("prepared item status = %s, want pending", preparedItems[0].Status)
	}
	if preparedItems[0].CurrentAttemptNo != 0 || preparedItems[0].CurrentQCNo != 0 {
		t.Fatalf("prepared current counters = %d/%d, want 0/0", preparedItems[0].CurrentAttemptNo, preparedItems[0].CurrentQCNo)
	}
	if preparedItems[0].FinishedAt != nil {
		t.Fatal("prepared item finished_at should be cleared")
	}

	job, _ := database.GetJob(jobID)
	if job.Status != "running" {
		t.Fatalf("prepared job status = %s, want running", job.Status)
	}
	if job.FinishedAt != nil {
		t.Fatal("prepared job finished_at should be cleared")
	}

	attempts, err := runner.listAttempts(preparedItems[0].ID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	rounds, err := runner.listQCRounds(preparedItems[0].ID)
	if err != nil {
		t.Fatalf("list qc rounds: %v", err)
	}
	if len(attempts) != 1 || len(rounds) != 1 {
		t.Fatalf("history should be preserved, got attempts=%d qc=%d", len(attempts), len(rounds))
	}
}

// prompt 模板里的 {{item}} 必须被真实替换后传给 executor
func TestRunBatch_PromptTemplateRendered(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJob(t, database, "", 3, []string{"workspace-create"})

	fake := executor.NewFakeExecutor()
	runner := NewRunnerWithExecutor(database, fake)

	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(calls))
	}
	if !strings.Contains(calls[0].Prompt, "workspace-create") {
		t.Errorf("prompt not rendered: %q", calls[0].Prompt)
	}
	if strings.Contains(calls[0].Prompt, "{{item}}") {
		t.Errorf("template placeholder leaked into prompt: %q", calls[0].Prompt)
	}
}
