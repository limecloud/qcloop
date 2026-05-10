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

	args, err := codexExecArgsFromEnv(prompt)
	if err != nil {
		return "", "", -1, err
	}
	cmd := exec.CommandContext(ctx, codexPath, args...)

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

func codexExecArgsFromEnv(prompt string) ([]string, error) {
	args := []string{"exec"}

	bypassSandbox := isTruthy(os.Getenv("QCLOOP_CODEX_BYPASS_SANDBOX")) ||
		isTruthy(os.Getenv("QCLOOP_CODEX_DANGEROUSLY_BYPASS"))
	if bypassSandbox {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		if approvalPolicy := strings.TrimSpace(os.Getenv("QCLOOP_CODEX_APPROVAL_POLICY")); approvalPolicy != "" {
			args = append(args, "-c", fmt.Sprintf("approval_policy=%q", approvalPolicy))
		}
		sandbox, err := normalizeCodexSandbox(os.Getenv("QCLOOP_CODEX_SANDBOX"))
		if err != nil {
			return nil, err
		}
		if sandbox != "" {
			args = append(args, "--sandbox", sandbox)
		}
	}

	if cwd := firstNonEmptyEnv("QCLOOP_CODEX_CWD", "QCLOOP_CODEX_WORKDIR"); cwd != "" {
		args = append(args, "-C", cwd)
	}

	extraArgs, err := splitCommandLine(os.Getenv("QCLOOP_CODEX_EXTRA_ARGS"))
	if err != nil {
		return nil, fmt.Errorf("QCLOOP_CODEX_EXTRA_ARGS 解析失败: %w", err)
	}
	args = append(args, extraArgs...)

	// codex exec 直接接受 prompt 作为最后一个位置参数。
	return append(args, prompt), nil
}

func normalizeCodexSandbox(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "default", "inherit", "codex-default":
		return "", nil
	case "read-only", "readonly":
		return "read-only", nil
	case "workspace-write", "workspace", "workspace_write":
		return "workspace-write", nil
	case "danger-full-access", "danger", "dangerous", "full", "full-access", "none", "off", "disabled", "false", "no":
		return "danger-full-access", nil
	default:
		return "", fmt.Errorf("QCLOOP_CODEX_SANDBOX=%q 无效,可选: read-only, workspace-write, danger-full-access, off", value)
	}
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func splitCommandLine(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}

	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("存在未闭合引号")
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
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
