package core

import (
	"context"
	"testing"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// makeJobWithBudget 是 makeJob 的变体,带 token 预算
func makeJobWithBudget(t *testing.T, database *db.DB, verifierPrompt string, maxRounds int, budget int, items []string) string {
	t.Helper()
	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:                     jobID,
		Name:                   "budget-" + t.Name(),
		PromptTemplate:         "do {{item}}",
		VerifierPromptTemplate: verifierPrompt,
		MaxQCRounds:            maxRounds,
		TokenBudgetPerItem:     budget,
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

// 无预算(=0): 即便 executor 返回 tokens,也不熔断,行为等同基线
func TestBudget_ZeroMeansUnlimited(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJobWithBudget(t, database, `check`, 3, 0, []string{"x"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "w", ExitCode: 0, TokensUsed: 1_000_000},
		executor.FakeResponse{Stdout: `{"pass": true}`, ExitCode: 0, TokensUsed: 1_000_000},
	)
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "success" {
		t.Errorf("with budget=0 want success, got %s", items[0].Status)
	}
}

// 预算够:normal pass 应当顺利 success,tokens_used 被累加
func TestBudget_Sufficient(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJobWithBudget(t, database, `check`, 3, 1000, []string{"x"})

	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "w", ExitCode: 0, TokensUsed: 100},
		executor.FakeResponse{Stdout: `{"pass": true}`, ExitCode: 0, TokensUsed: 50},
	)
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	var used int
	_ = database.Conn().QueryRow(`SELECT tokens_used FROM batch_items WHERE batch_job_id = ?`, jobID).Scan(&used)
	if used != 150 {
		t.Errorf("tokens_used: want 150, got %d", used)
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "success" {
		t.Errorf("want success, got %s", items[0].Status)
	}
}

// 预算不够:worker 跑完预算耗尽,进入质检前熔断标记 exhausted
func TestBudget_ExceededAfterWorker(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJobWithBudget(t, database, `check`, 3, 100, []string{"x"})

	// worker 就把 150 tokens 打掉了,超过 budget 100
	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "w", ExitCode: 0, TokensUsed: 150},
	)
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	// 只应有 1 次调用(worker),verifier 在 runQCLoop 入口就因超预算被短路
	if fake.CallCount() != 1 {
		t.Errorf("want 1 call (worker only), got %d", fake.CallCount())
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "exhausted" {
		t.Errorf("want exhausted, got %s", items[0].Status)
	}
}

// 预算在 verifier 首轮后耗尽:不再进入 repair,直接 exhausted
func TestBudget_ExceededAfterVerifier(t *testing.T) {
	database := newTestDB(t)
	jobID := makeJobWithBudget(t, database, `check`, 3, 200, []string{"x"})

	// worker 消耗 80,verifier 第一轮再 fail 消耗 150 → 累计 230 > 200
	fake := executor.NewFakeExecutor(
		executor.FakeResponse{Stdout: "w", ExitCode: 0, TokensUsed: 80},
		executor.FakeResponse{Stdout: `{"pass": false, "feedback": "x"}`, ExitCode: 0, TokensUsed: 150},
	)
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	// worker + verifier 共 2 次,不应进入 repair
	if fake.CallCount() != 2 {
		t.Errorf("want 2 calls (worker+verifier), got %d", fake.CallCount())
	}

	items, _ := database.ListItems(jobID)
	if items[0].Status != "exhausted" {
		t.Errorf("want exhausted after budget exceeded mid-qc, got %s", items[0].Status)
	}
}
