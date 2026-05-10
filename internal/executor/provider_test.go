package executor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeProviderAcceptsAliases(t *testing.T) {
	cases := map[string]string{
		"":                  ProviderCodex,
		"codex-cli":         ProviderCodex,
		"claude":            ProviderClaudeCode,
		"claude-code":       ProviderClaudeCode,
		"gemini":            ProviderGeminiCLI,
		"google-gemini-cli": ProviderGeminiCLI,
		"kiro":              ProviderKiroCLI,
		"kiro-cli":          ProviderKiroCLI,
	}
	for input, want := range cases {
		got, err := NormalizeProvider(input)
		if err != nil {
			t.Fatalf("NormalizeProvider(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizeProvider(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAgentCLIExecutorsPassConfiguredArgs(t *testing.T) {
	claudePath := writeArgPrinterTool(t, "claude")
	geminiPath := writeArgPrinterTool(t, "gemini")
	kiroPath := writeArgPrinterTool(t, "kiro-cli")

	t.Setenv("QCLOOP_CLAUDE_BIN", claudePath)
	t.Setenv("QCLOOP_CLAUDE_DANGEROUSLY_SKIP_PERMISSIONS", "true")
	t.Setenv("QCLOOP_CLAUDE_PERMISSION_MODE", "bypassPermissions")
	t.Setenv("QCLOOP_CLAUDE_MODEL", "sonnet")
	t.Setenv("QCLOOP_CLAUDE_MAX_TURNS", "6")
	t.Setenv("QCLOOP_CLAUDE_SETTINGS", "/tmp/claude-settings.json")
	t.Setenv("QCLOOP_CLAUDE_TEAMMATE_MODE", "tmux")
	t.Setenv("QCLOOP_CLAUDE_EXTRA_ARGS", `--output-format json`)
	assertProviderArgs(t, ProviderClaudeCode, []string{
		"--dangerously-skip-permissions",
		"--permission-mode", "bypassPermissions",
		"--model", "sonnet",
		"--max-turns", "6",
		"--settings", "/tmp/claude-settings.json",
		"--teammate-mode", "tmux",
		"--output-format", "json",
		"-p", "hello",
	})

	t.Setenv("QCLOOP_GEMINI_BIN", geminiPath)
	t.Setenv("QCLOOP_GEMINI_APPROVAL_MODE", "plan")
	t.Setenv("QCLOOP_GEMINI_YOLO", "true")
	t.Setenv("QCLOOP_GEMINI_SANDBOX", "docker")
	t.Setenv("QCLOOP_GEMINI_MODEL", "gemini-3-flash-preview")
	t.Setenv("QCLOOP_GEMINI_EXTRA_ARGS", `--output-format stream-json`)
	assertProviderArgs(t, ProviderGeminiCLI, []string{
		"--approval-mode", "plan",
		"--model", "gemini-3-flash-preview",
		"--yolo",
		"--sandbox", "docker",
		"--output-format", "stream-json",
		"-p", "hello",
	})

	t.Setenv("QCLOOP_KIRO_BIN", kiroPath)
	t.Setenv("QCLOOP_KIRO_TRUST_ALL_TOOLS", "true")
	t.Setenv("QCLOOP_KIRO_TRUST_TOOLS", "read,write")
	t.Setenv("QCLOOP_KIRO_REQUIRE_MCP_STARTUP", "true")
	t.Setenv("QCLOOP_KIRO_AGENT", "qa-specialist")
	t.Setenv("QCLOOP_KIRO_EXTRA_ARGS", `--debug`)
	assertProviderArgs(t, ProviderKiroCLI, []string{
		"chat", "--no-interactive",
		"--trust-all-tools",
		"--trust-tools", "read,write",
		"--require-mcp-startup",
		"--agent", "qa-specialist",
		"--debug",
		"hello",
	})
}

func TestClaudeCodeRejectsInvalidTeammateMode(t *testing.T) {
	claudePath := writeArgPrinterTool(t, "claude")
	t.Setenv("QCLOOP_CLAUDE_BIN", claudePath)
	t.Setenv("QCLOOP_CLAUDE_TEAMMATE_MODE", "invalid")

	exec, err := NewExecutorForProvider(ProviderClaudeCode)
	if err != nil {
		t.Fatalf("NewExecutorForProvider: %v", err)
	}
	_, _, exitCode, err := exec.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCode != -1 {
		t.Fatalf("exitCode = %d, want -1", exitCode)
	}
	if !strings.Contains(err.Error(), "QCLOOP_CLAUDE_TEAMMATE_MODE") {
		t.Fatalf("error = %q", err)
	}
}

func assertProviderArgs(t *testing.T, provider string, want []string) {
	t.Helper()
	exec, err := NewExecutorForProvider(provider)
	if err != nil {
		t.Fatalf("NewExecutorForProvider(%s): %v", provider, err)
	}
	stdout, stderr, exitCode, err := exec.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Execute(%s) returned error: %v", provider, err)
	}
	if exitCode != 0 {
		t.Fatalf("Execute(%s) exitCode = %d, stderr = %q", provider, exitCode, stderr)
	}
	got := strings.Split(strings.TrimSpace(stdout), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s args mismatch\ngot:\n%s\nwant:\n%s", provider, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func writeArgPrinterTool(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	writeExecutable(t, path, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "test cli"
  exit 0
fi
printf '%s\n' "$@"
`)
	return path
}
