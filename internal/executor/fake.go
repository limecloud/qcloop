package executor

import (
	"context"
	"sync"
)

// FakeExecutor 是用于测试的脚本化 Executor。
//
// 用法:按调用顺序提供 Responses。第 i 次 Execute 返回 Responses[i]。
// 超出范围时返回最后一个响应(便于"一直成功/一直失败"这种简单场景)。
// 并发安全。
type FakeExecutor struct {
	mu        sync.Mutex
	calls     []FakeCall
	Responses []FakeResponse
}

// FakeCall 记录一次 Execute 的输入,方便测试断言
type FakeCall struct {
	Prompt string
}

// FakeResponse 描述一次 Execute 的返回
type FakeResponse struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	TokensUsed int
	Err        error
}

// NewFakeExecutor 创建脚本化 fake,按顺序消费 responses
func NewFakeExecutor(responses ...FakeResponse) *FakeExecutor {
	return &FakeExecutor{Responses: responses}
}

// Execute 返回下一个脚本化响应(legacy 接口,不带 tokens)
func (f *FakeExecutor) Execute(ctx context.Context, prompt string) (string, string, int, error) {
	r := f.ExecuteWithTokens(ctx, prompt)
	var err error
	if r.ExitCode < 0 {
		// 保留一种表达"qcloop 自己出错"的方式:ExitCode < 0 时取 Responses.Err
		f.mu.Lock()
		idx := len(f.calls) - 1
		if idx >= 0 && idx < len(f.Responses) {
			err = f.Responses[idx].Err
		}
		f.mu.Unlock()
	}
	return r.Stdout, r.Stderr, r.ExitCode, err
}

// ExecuteWithTokens 返回下一个脚本化响应,含 token 数
func (f *FakeExecutor) ExecuteWithTokens(_ context.Context, prompt string) Result {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, FakeCall{Prompt: prompt})

	if len(f.Responses) == 0 {
		return Result{Stdout: "ok: " + prompt, ExitCode: 0, TokensUsed: 0}
	}

	idx := len(f.calls) - 1
	if idx >= len(f.Responses) {
		idx = len(f.Responses) - 1
	}
	r := f.Responses[idx]
	return Result{
		Stdout:     r.Stdout,
		Stderr:     r.Stderr,
		ExitCode:   r.ExitCode,
		TokensUsed: r.TokensUsed,
	}
}

// Calls 返回所有已发生的调用(用于断言)
func (f *FakeExecutor) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// CallCount 返回已发生的调用次数
func (f *FakeExecutor) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// 编译期断言
var _ Executor = (*FakeExecutor)(nil)
var _ TokenAwareExecutor = (*FakeExecutor)(nil)
