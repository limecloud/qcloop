package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CodexExecutor codex exec 执行器
type CodexExecutor struct {
	timeout time.Duration
}

// NewCodexExecutor 创建 codex 执行器
func NewCodexExecutor() *CodexExecutor {
	return &CodexExecutor{
		timeout: 5 * time.Minute,
	}
}

// Execute 执行 codex exec
func (e *CodexExecutor) Execute(ctx context.Context, prompt string) (stdout, stderr string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// codex exec 直接接受 prompt 作为参数，不需要 --prompt 标志
	cmd := exec.CommandContext(ctx, "codex", "exec", prompt)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			err = fmt.Errorf("执行失败: %w", err)
		}
	} else {
		exitCode = 0
	}

	return
}
