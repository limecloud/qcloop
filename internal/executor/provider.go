package executor

import (
	"fmt"
	"os"
	"strings"
)

const (
	ProviderCodex      = "codex"
	ProviderClaudeCode = "claude_code"
	ProviderGeminiCLI  = "gemini_cli"
	ProviderKiroCLI    = "kiro_cli"
)

// SupportedProviders 返回 API/UI 可用的执行器 provider。
func SupportedProviders() []string {
	return []string{ProviderCodex, ProviderClaudeCode, ProviderGeminiCLI, ProviderKiroCLI}
}

// DefaultProviderFromEnv 返回 qcloop 创建批次时的默认执行器。
func DefaultProviderFromEnv() (string, error) {
	value := strings.TrimSpace(os.Getenv("QCLOOP_EXECUTOR_PROVIDER"))
	if value == "" {
		return ProviderCodex, nil
	}
	return NormalizeProvider(value)
}

// NormalizeProvider 归一化 provider 名称,允许常见 CLI 别名。
func NormalizeProvider(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")

	switch normalized {
	case "", "default", "codex", "codex_cli", "codex_exec":
		return ProviderCodex, nil
	case "claude", "claude_code", "claude_cli", "claude_code_cli":
		return ProviderClaudeCode, nil
	case "gemini", "gemini_cli", "google_gemini", "google_gemini_cli":
		return ProviderGeminiCLI, nil
	case "kiro", "kiro_cli", "kiro_dev", "kiro_dev_cli":
		return ProviderKiroCLI, nil
	default:
		return "", fmt.Errorf("executor_provider must be one of: %s", strings.Join(SupportedProviders(), ", "))
	}
}

// NewExecutorForProvider 创建指定 provider 的 CLI 执行器。
func NewExecutorForProvider(provider string) (Executor, error) {
	normalized, err := NormalizeProvider(provider)
	if err != nil {
		return nil, err
	}

	switch normalized {
	case ProviderCodex:
		return NewCodexExecutor(), nil
	case ProviderClaudeCode:
		return NewAgentCLIExecutor(AgentCLISpec{
			DisplayName: "Claude Code",
			BinaryName:  "claude",
			BinaryEnv:   "QCLOOP_CLAUDE_BIN",
			WorkDirEnv:  []string{"QCLOOP_CLAUDE_CWD", "QCLOOP_CLAUDE_WORKDIR"},
			VersionArgs: []string{"--version"},
			BuildArgs:   claudeCodeArgsFromEnv,
		}), nil
	case ProviderGeminiCLI:
		return NewAgentCLIExecutor(AgentCLISpec{
			DisplayName: "Gemini CLI",
			BinaryName:  "gemini",
			BinaryEnv:   "QCLOOP_GEMINI_BIN",
			WorkDirEnv:  []string{"QCLOOP_GEMINI_CWD", "QCLOOP_GEMINI_WORKDIR"},
			VersionArgs: []string{"--version"},
			BuildArgs:   geminiCLIArgsFromEnv,
		}), nil
	case ProviderKiroCLI:
		return NewAgentCLIExecutor(AgentCLISpec{
			DisplayName: "Kiro CLI",
			BinaryName:  "kiro-cli",
			BinaryEnv:   "QCLOOP_KIRO_BIN",
			WorkDirEnv:  []string{"QCLOOP_KIRO_CWD", "QCLOOP_KIRO_WORKDIR"},
			VersionArgs: []string{"--version"},
			BuildArgs:   kiroCLIArgsFromEnv,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported executor_provider: %s", provider)
	}
}
