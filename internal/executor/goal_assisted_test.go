package executor

import (
	"context"
	"strings"
	"testing"
)

func TestGoalAssisted_WrapPrompt_ContainsOriginalTask(t *testing.T) {
	fake := NewFakeExecutor()
	g := NewGoalAssistedExecutor(fake, "重构成 Go 风格", "所有测试通过")

	_, _, _, _ = g.Execute(context.Background(), "review file foo.go")

	if len(fake.Calls()) != 1 {
		t.Fatalf("want 1 inner call, got %d", len(fake.Calls()))
	}
	wrapped := fake.Calls()[0].Prompt

	for _, must := range []string{
		"GOAL:",
		"重构成 Go 风格",
		"TASK:",
		"review file foo.go",
		"STOP WHEN:",
		"所有测试通过",
		"INSTRUCTIONS:",
	} {
		if !strings.Contains(wrapped, must) {
			t.Errorf("wrapped prompt missing %q:\n---\n%s\n---", must, wrapped)
		}
	}
}

func TestGoalAssisted_DefaultStopHint(t *testing.T) {
	fake := NewFakeExecutor()
	g := NewGoalAssistedExecutor(fake, "", "") // 两个 hint 都空

	_, _, _, _ = g.Execute(context.Background(), "do x")
	wrapped := fake.Calls()[0].Prompt

	// 没给 goalHint 时不应出现 GOAL: 区块
	if strings.Contains(wrapped, "GOAL:\n") {
		t.Errorf("empty goalHint should not emit GOAL: section, got:\n%s", wrapped)
	}
	// 没给 stopHint 时应该用默认的兜底 stop hint
	if !strings.Contains(wrapped, "任务明确完成") {
		t.Errorf("empty stopHint should emit default stop hint, got:\n%s", wrapped)
	}
}

func TestGoalAssisted_TokensPassThrough(t *testing.T) {
	fake := NewFakeExecutor(FakeResponse{Stdout: "done", ExitCode: 0, TokensUsed: 42})
	g := NewGoalAssistedExecutor(fake, "", "")

	res := g.ExecuteWithTokens(context.Background(), "x")
	if res.TokensUsed != 42 {
		t.Errorf("tokens should pass through from inner executor: want 42, got %d", res.TokensUsed)
	}
	if res.Stdout != "done" {
		t.Errorf("stdout passthrough broken: %q", res.Stdout)
	}
}

func TestGoalAssisted_ExitCodePassThrough(t *testing.T) {
	fake := NewFakeExecutor(FakeResponse{Stdout: "", Stderr: "boom", ExitCode: 2})
	g := NewGoalAssistedExecutor(fake, "", "")

	_, stderr, code, _ := g.Execute(context.Background(), "x")
	if code != 2 {
		t.Errorf("want exit code 2, got %d", code)
	}
	if stderr != "boom" {
		t.Errorf("want stderr 'boom', got %q", stderr)
	}
}
