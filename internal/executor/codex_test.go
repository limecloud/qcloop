package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexExecutorSkipsBrokenCodexOnPath(t *testing.T) {
	badDir := t.TempDir()
	goodDir := t.TempDir()
	badCodex := filepath.Join(badDir, "codex")
	goodCodex := filepath.Join(goodDir, "codex")

	writeExecutable(t, badCodex, `#!/bin/sh
echo broken codex >&2
exit 1
`)
	writeExecutable(t, goodCodex, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "codex-cli test"
  exit 0
fi
if [ "$1" = "exec" ]; then
  printf 'exec:%s' "$2"
  exit 0
fi
exit 2
`)

	t.Setenv("QCLOOP_CODEX_BIN", "")
	t.Setenv("PATH", badDir+string(os.PathListSeparator)+goodDir)

	exec := NewCodexExecutor()
	stdout, stderr, exitCode, err := exec.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %q", exitCode, stderr)
	}
	if stdout != "exec:hello" {
		t.Fatalf("stdout = %q", stdout)
	}
	if exec.codexPath != goodCodex {
		t.Fatalf("resolved codexPath = %q, want %q", exec.codexPath, goodCodex)
	}
}

func TestCodexExecutorReportsInvalidExplicitCodexBin(t *testing.T) {
	badDir := t.TempDir()
	badCodex := filepath.Join(badDir, "codex")
	writeExecutable(t, badCodex, `#!/bin/sh
echo explicitly broken >&2
exit 1
`)

	t.Setenv("QCLOOP_CODEX_BIN", badCodex)

	exec := NewCodexExecutor()
	_, _, exitCode, err := exec.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if exitCode != -1 {
		t.Fatalf("exitCode = %d, want -1", exitCode)
	}
	if !strings.Contains(err.Error(), "QCLOOP_CODEX_BIN 不可用") {
		t.Fatalf("error = %q", err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
