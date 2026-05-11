package executor

import (
	"context"
	"fmt"
	"os"
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
		timeout: executorTimeoutFromEnv(
			5*time.Minute,
			"QCLOOP_CODEX_TIMEOUT",
			"QCLOOP_CODEX_TIMEOUT_MS",
			"QCLOOP_EXECUTOR_TIMEOUT",
			"QCLOOP_EXECUTOR_TIMEOUT_MS",
		),
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
	return runCLICommand(ctx, codexPath, args, "")
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
	path, probeError, err := resolveExecutablePath(ctx, executableProbeConfig{
		DisplayName: "codex",
		BinaryName:  "codex",
		BinaryEnv:   "QCLOOP_CODEX_BIN",
		CachedPath:  e.codexPath,
		VersionArgs: []string{"--version"},
	})
	if err != nil {
		e.probeError = probeError
		return "", err
	}
	e.codexPath = path
	e.probeError = ""
	return path, nil
}
