package executor

import (
	"context"
	"fmt"
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
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// NewFakeExecutor 创建脚本化 fake,按顺序消费 responses
func NewFakeExecutor(responses ...FakeResponse) *FakeExecutor {
	return &FakeExecutor{Responses: responses}
}

// Execute 返回下一个脚本化响应
func (f *FakeExecutor) Execute(_ context.Context, prompt string) (string, string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, FakeCall{Prompt: prompt})

	if len(f.Responses) == 0 {
		// 未配置时默认成功,输出 echo
		return "ok: " + prompt, "", 0, nil
	}

	idx := len(f.calls) - 1
	if idx >= len(f.Responses) {
		idx = len(f.Responses) - 1
	}
	r := f.Responses[idx]
	return r.Stdout, r.Stderr, r.ExitCode, r.Err
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

// 为了避免 fmt 未用报错,保留一个占位 helper
var _ = fmt.Sprintf
