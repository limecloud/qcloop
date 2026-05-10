package executor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// AgentCLISpec 描述一个可通过本机 CLI headless 调用的 AI agent。
type AgentCLISpec struct {
	DisplayName string
	BinaryName  string
	BinaryEnv   string
	WorkDirEnv  []string
	VersionArgs []string
	BuildArgs   func(prompt string) ([]string, error)
}

// AgentCLIExecutor 是 Claude Code / Gemini CLI / Kiro CLI 的共享进程执行器。
type AgentCLIExecutor struct {
	spec       AgentCLISpec
	timeout    time.Duration
	path       string
	probeError string
}

func NewAgentCLIExecutor(spec AgentCLISpec) *AgentCLIExecutor {
	return &AgentCLIExecutor{spec: spec, timeout: 5 * time.Minute}
}

func (e *AgentCLIExecutor) Execute(ctx context.Context, prompt string) (stdout, stderr string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	path, err := e.resolvePath(ctx)
	if err != nil {
		return "", "", -1, err
	}
	args, err := e.spec.BuildArgs(prompt)
	if err != nil {
		return "", "", -1, err
	}
	return runCLICommand(ctx, path, args, firstNonEmptyEnv(e.spec.WorkDirEnv...))
}

func (e *AgentCLIExecutor) resolvePath(ctx context.Context) (string, error) {
	path, probeError, err := resolveExecutablePath(ctx, executableProbeConfig{
		DisplayName: e.spec.DisplayName,
		BinaryName:  e.spec.BinaryName,
		BinaryEnv:   e.spec.BinaryEnv,
		CachedPath:  e.path,
		VersionArgs: e.spec.VersionArgs,
	})
	if err != nil {
		e.probeError = probeError
		return "", err
	}
	e.path = path
	e.probeError = ""
	return path, nil
}

func claudeCodeArgsFromEnv(prompt string) ([]string, error) {
	args := []string{}
	if isTruthy(os.Getenv("QCLOOP_CLAUDE_DANGEROUSLY_SKIP_PERMISSIONS")) || isTruthy(os.Getenv("QCLOOP_CLAUDE_BYPASS_PERMISSIONS")) {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = appendStringFlag(args, "--permission-mode", os.Getenv("QCLOOP_CLAUDE_PERMISSION_MODE"))
	args = appendStringFlag(args, "--model", os.Getenv("QCLOOP_CLAUDE_MODEL"))
	args = appendStringFlag(args, "--max-turns", os.Getenv("QCLOOP_CLAUDE_MAX_TURNS"))
	args = appendStringFlag(args, "--settings", os.Getenv("QCLOOP_CLAUDE_SETTINGS"))

	if teammateMode := strings.TrimSpace(os.Getenv("QCLOOP_CLAUDE_TEAMMATE_MODE")); teammateMode != "" {
		if err := validateClaudeTeammateMode(teammateMode); err != nil {
			return nil, err
		}
		args = append(args, "--teammate-mode", teammateMode)
	}
	var err error
	args, err = appendExtraArgs(args, "QCLOOP_CLAUDE_EXTRA_ARGS")
	if err != nil {
		return nil, err
	}

	return append(args, "-p", prompt), nil
}

func geminiCLIArgsFromEnv(prompt string) ([]string, error) {
	args := []string{}
	args = appendStringFlag(args, "--approval-mode", os.Getenv("QCLOOP_GEMINI_APPROVAL_MODE"))
	args = appendStringFlag(args, "--model", os.Getenv("QCLOOP_GEMINI_MODEL"))
	if isTruthy(os.Getenv("QCLOOP_GEMINI_YOLO")) {
		args = append(args, "--yolo")
	}
	if sandbox := strings.TrimSpace(os.Getenv("QCLOOP_GEMINI_SANDBOX")); sandbox != "" && !isFalsy(sandbox) {
		args = append(args, "--sandbox")
		if !isTruthy(sandbox) {
			args = append(args, sandbox)
		}
	}
	var err error
	args, err = appendExtraArgs(args, "QCLOOP_GEMINI_EXTRA_ARGS")
	if err != nil {
		return nil, err
	}
	return append(args, "-p", prompt), nil
}

func kiroCLIArgsFromEnv(prompt string) ([]string, error) {
	args := []string{"chat", "--no-interactive"}
	if isTruthy(os.Getenv("QCLOOP_KIRO_TRUST_ALL_TOOLS")) {
		args = append(args, "--trust-all-tools")
	}
	args = appendStringFlag(args, "--trust-tools", os.Getenv("QCLOOP_KIRO_TRUST_TOOLS"))
	if isTruthy(os.Getenv("QCLOOP_KIRO_REQUIRE_MCP_STARTUP")) {
		args = append(args, "--require-mcp-startup")
	}
	args = appendStringFlag(args, "--agent", os.Getenv("QCLOOP_KIRO_AGENT"))
	var err error
	args, err = appendExtraArgs(args, "QCLOOP_KIRO_EXTRA_ARGS")
	if err != nil {
		return nil, err
	}
	return append(args, prompt), nil
}

func appendStringFlag(args []string, flag, value string) []string {
	if strings.TrimSpace(value) == "" {
		return args
	}
	return append(args, flag, strings.TrimSpace(value))
}

func appendExtraArgs(args []string, envName string) ([]string, error) {
	extraArgs, err := splitCommandLine(os.Getenv(envName))
	if err != nil {
		return nil, fmt.Errorf("%s 解析失败: %w", envName, err)
	}
	return append(args, extraArgs...), nil
}

func validateClaudeTeammateMode(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "in-process", "tmux":
		return nil
	default:
		return fmt.Errorf("QCLOOP_CLAUDE_TEAMMATE_MODE=%q 无效,可选: auto, in-process, tmux", value)
	}
}

func isFalsy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "n", "off", "disabled", "none":
		return true
	default:
		return false
	}
}

var _ Executor = (*AgentCLIExecutor)(nil)
