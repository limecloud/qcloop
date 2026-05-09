package executor

import "context"

// Executor 抽象出对外部 agent 的一次无状态调用。
// 真实实现是 CodexExecutor;测试用 FakeExecutor。
//
// 合约:
//   - stdout/stderr 已读取完整(不是流)
//   - exitCode 非 0 表示执行失败
//   - 返回 err 仅用于"qcloop 自己的错误"(如进程启动失败),
//     外部命令非 0 退出码不视为 err
type Executor interface {
	Execute(ctx context.Context, prompt string) (stdout, stderr string, exitCode int, err error)
}

// 编译期断言 CodexExecutor 满足 Executor 接口
var _ Executor = (*CodexExecutor)(nil)
