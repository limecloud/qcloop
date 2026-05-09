package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodexExecutor codex exec 执行器
type CodexExecutor struct {
	timeout    time.Duration
	codexPath  string
	probeError string
}

// NewCodexExecutor 创建 codex 执行器
func NewCodexExecutor() *CodexExecutor {
	return &CodexExecutor{
		timeout:   5 * time.Minute,
		codexPath: os.Getenv("QCLOOP_CODEX_BIN"),
	}
}

// Execute 执行 codex exec
func (e *CodexExecutor) Execute(ctx context.Context, prompt string) (stdout, stderr string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	codexPath, err := e.resolveCodexPath(ctx)
	if err != nil {
		return "", "", -1, err
	}

	// codex exec 直接接受 prompt 作为参数，不需要 --prompt 标志
	cmd := exec.CommandContext(ctx, codexPath, "exec", prompt)

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

func (e *CodexExecutor) resolveCodexPath(ctx context.Context) (string, error) {
	if e.codexPath != "" {
		if err := probeCodex(ctx, e.codexPath); err == nil {
			return e.codexPath, nil
		} else if os.Getenv("QCLOOP_CODEX_BIN") != "" {
			return "", fmt.Errorf("QCLOOP_CODEX_BIN 不可用: %s: %w", e.codexPath, err)
		}
	}

	var probeErrors []string
	for _, candidate := range codexCandidates() {
		if err := probeCodex(ctx, candidate); err == nil {
			e.codexPath = candidate
			e.probeError = ""
			return candidate, nil
		} else {
			probeErrors = append(probeErrors, fmt.Sprintf("%s: %v", candidate, err))
		}
	}

	e.probeError = strings.Join(probeErrors, "; ")
	return "", fmt.Errorf("没有找到可用 codex；请修复 PATH 或设置 QCLOOP_CODEX_BIN。探测结果: %s", e.probeError)
}

func codexCandidates() []string {
	seen := map[string]bool{}
	var candidates []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "codex")
		if seen[candidate] || !isExecutable(candidate) {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0111 != 0
}

func probeCodex(ctx context.Context, path string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, path, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(out.String())
		if text == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, text)
	}
	return nil
}
