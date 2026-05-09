package executor

import "context"

// Result 是一次 Execute 的统一结果。
type Result struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	TokensUsed int // 真实 token 数,未知时为 0(CodexExecutor 当前无法拿到,返回 0)
}

// Executor 抽象出对外部 agent 的一次无状态调用。
//
// 合约:
//   - stdout/stderr 已读取完整(不是流)
//   - exitCode 非 0 表示执行失败
//   - 返回 err 仅用于"qcloop 自己的错误"(如进程启动失败),
//     外部命令非 0 退出码不视为 err
type Executor interface {
	Execute(ctx context.Context, prompt string) (stdout, stderr string, exitCode int, err error)
}

// TokenAwareExecutor 是可选的扩展接口:实现了这个接口的 Executor
// 能返回 token 消耗量,Runner 会调用 ExecuteWithTokens 代替 Execute。
//
// CodexExecutor 暂不实现(codex exec 目前不回传 token);
// FakeExecutor 实现,用于测试。
type TokenAwareExecutor interface {
	Executor
	ExecuteWithTokens(ctx context.Context, prompt string) Result
}

// 编译期断言 CodexExecutor 满足 Executor 接口
var _ Executor = (*CodexExecutor)(nil)
