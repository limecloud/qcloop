package executor

import (
	"context"
	"fmt"
	"strings"
)

// GoalAssistedExecutor 用 "Goal-flavored prompt" 包装一个底层 Executor。
//
// 灵感来自 Codex /goal:每次调用 codex 前,把业务 prompt 重写成一段
// 有明确 objective / 停止条件 / 反馈循环指令的结构化 prompt,让 Codex
// 在单次 exec 内部就尽可能朝目标收敛,减少外层 qcloop 轮次。
//
// 与原生 /goal 的区别:
//   - 不依赖 codex app-server / thread/goal/* RPC(那部分仍 experimental)
//   - 单次 exec 之间无 session 连续性(每次都是 fresh thread)
//   - 由 qcloop 外层 max_qc_rounds 负责"硬停止",不相信 Codex 自评
//
// 用途:让 qcloop 的 Runner 在不接 app-server 的前提下,也能享受 Goal 风格
// 的"目标导向 + 自我审视"prompt 工程红利。
type GoalAssistedExecutor struct {
	inner     Executor
	goalHint  string // 可选的额外目标指令,会被拼进 prompt
	stopHint  string // 可选的停止条件指令
}

// NewGoalAssistedExecutor 包装一个底层 executor。
//
// goalHint 会被拼入"GOAL:"段落,提示 Codex 整体目标;
// stopHint 会被拼入"STOP WHEN:"段落,提示 Codex 何时应当停止迭代。
// 两者都可为空字符串。
func NewGoalAssistedExecutor(inner Executor, goalHint, stopHint string) *GoalAssistedExecutor {
	return &GoalAssistedExecutor{
		inner:    inner,
		goalHint: goalHint,
		stopHint: stopHint,
	}
}

// Execute 用 goal-flavored prompt 包装原始 prompt 后调用内部 executor
func (g *GoalAssistedExecutor) Execute(ctx context.Context, prompt string) (string, string, int, error) {
	return g.inner.Execute(ctx, g.wrapPrompt(prompt))
}

// ExecuteWithTokens 若内部 executor 支持 TokenAware,则透传;否则 tokens=0
func (g *GoalAssistedExecutor) ExecuteWithTokens(ctx context.Context, prompt string) Result {
	wrapped := g.wrapPrompt(prompt)
	if tae, ok := g.inner.(TokenAwareExecutor); ok {
		return tae.ExecuteWithTokens(ctx, wrapped)
	}
	stdout, stderr, code, _ := g.inner.Execute(ctx, wrapped)
	return Result{Stdout: stdout, Stderr: stderr, ExitCode: code, TokensUsed: 0}
}

// wrapPrompt 是本类唯一的业务逻辑:把普通 prompt 套进 goal-style 结构
func (g *GoalAssistedExecutor) wrapPrompt(prompt string) string {
	var b strings.Builder

	if g.goalHint != "" {
		fmt.Fprintf(&b, "GOAL:\n%s\n\n", g.goalHint)
	}

	b.WriteString("TASK:\n")
	b.WriteString(prompt)
	b.WriteString("\n\n")

	if g.stopHint != "" {
		fmt.Fprintf(&b, "STOP WHEN:\n%s\n\n", g.stopHint)
	} else {
		b.WriteString("STOP WHEN:\n- 任务明确完成,或\n- 你判断继续迭代也不会改善结果\n\n")
	}

	b.WriteString("INSTRUCTIONS:\n")
	b.WriteString("- 先简要规划再动手,避免无效循环\n")
	b.WriteString("- 在单次响应内尽可能朝目标收敛,但不要做超出 TASK 的修改\n")
	b.WriteString("- 完成后给出简短结论;失败时说明卡在哪里\n")

	return b.String()
}

// 编译期断言
var _ Executor = (*GoalAssistedExecutor)(nil)
var _ TokenAwareExecutor = (*GoalAssistedExecutor)(nil)
