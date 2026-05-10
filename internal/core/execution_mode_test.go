package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coso/qcloop/internal/db"
	"github.com/coso/qcloop/internal/executor"
)

// execution_mode=goal_assisted 时,Runner 调用 executor 的 prompt 应含 GOAL: 段
func TestRunBatch_ExecutionMode_GoalAssisted(t *testing.T) {
	database := newTestDB(t)

	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:             jobID,
		Name:           "goal-mode",
		PromptTemplate: "refactor {{item}}",
		MaxQCRounds:    3,
		ExecutionMode:  "goal_assisted",
		Status:         "pending",
		CreatedAt:      time.Now(),
	}
	if err := database.CreateJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	item := &db.BatchItem{
		ID:         db.GenerateID(),
		BatchJobID: jobID,
		ItemValue:  "foo.go",
		Status:     "pending",
		CreatedAt:  time.Now(),
	}
	if err := database.CreateItem(item); err != nil {
		t.Fatalf("create item: %v", err)
	}

	fake := executor.NewFakeExecutor() // 默认成功
	runner := NewRunnerWithExecutor(database, fake)
	if err := runner.RunBatch(context.Background(), jobID); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	// goal_assisted 应在 prompt 里注入 GOAL / TASK / STOP WHEN 段
	calls := fake.Calls()
	if len(calls) == 0 {
		t.Fatal("expected at least one executor call")
	}
	got := calls[0].Prompt
	for _, must := range []string{"GOAL:", "TASK:", "STOP WHEN:", "refactor foo.go"} {
		if !strings.Contains(got, must) {
			t.Errorf("goal_assisted prompt should contain %q, got:\n%s", must, got)
		}
	}
}

// execution_mode=standard(或空)时,prompt 不应被 wrap
func TestRunBatch_ExecutionMode_StandardDoesNotWrap(t *testing.T) {
	database := newTestDB(t)

	jobID := db.GenerateID()
	job := &db.BatchJob{
		ID:             jobID,
		Name:           "std-mode",
		PromptTemplate: "do {{item}}",
		MaxQCRounds:    3,
		ExecutionMode:  "", // 空,默认 standard
		Status:         "pending",
		CreatedAt:      time.Now(),
	}
	_ = database.CreateJob(job)
	_ = database.CreateItem(&db.BatchItem{
		ID: db.GenerateID(), BatchJobID: jobID, ItemValue: "x", Status: "pending", CreatedAt: time.Now(),
	})

	fake := executor.NewFakeExecutor()
	runner := NewRunnerWithExecutor(database, fake)
	_ = runner.RunBatch(context.Background(), jobID)

	got := fake.Calls()[0].Prompt
	if strings.Contains(got, "GOAL:") {
		t.Errorf("standard mode should NOT wrap prompt, but saw GOAL: in:\n%s", got)
	}
	if got != "do x" {
		t.Errorf("standard mode prompt should be literal render, got %q", got)
	}
}

func TestRunnerSelectsJobExecutorProviderBeforeGoalWrapper(t *testing.T) {
	fake := executor.NewFakeExecutor()
	runner := &Runner{
		executorFactory: func(provider string) (executor.Executor, error) {
			if provider != executor.ProviderGeminiCLI {
				t.Fatalf("provider = %q, want %q", provider, executor.ProviderGeminiCLI)
			}
			return fake, nil
		},
	}
	job := &db.BatchJob{
		PromptTemplate:         "do {{item}}",
		VerifierPromptTemplate: "verify {{output}}",
		ExecutionMode:          "goal_assisted",
		ExecutorProvider:       executor.ProviderGeminiCLI,
	}

	active, err := runner.buildActiveExecutor(job)
	if err != nil {
		t.Fatalf("buildActiveExecutor: %v", err)
	}
	_, _, _, err = active.Execute(context.Background(), "raw task")
	if err != nil {
		t.Fatalf("active.Execute: %v", err)
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if !strings.Contains(calls[0].Prompt, "GOAL:") || !strings.Contains(calls[0].Prompt, "raw task") {
		t.Fatalf("goal wrapper prompt missing expected content: %s", calls[0].Prompt)
	}
}
