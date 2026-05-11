//go:build !windows

package executor

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCLICommandTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	_, stderr, code, err := runCLICommand(ctx, "/bin/sh", []string{
		"-c",
		"(sleep 30) & echo $! > " + strconv.Quote(pidFile) + "; wait",
	}, "")

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(stderr, "执行超时或已取消") {
		t.Fatalf("stderr should mention timeout, got %q", stderr)
	}
	if code == 0 {
		t.Fatalf("exit code should be non-zero on timeout")
	}

	rawPID, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("read child pid: %v", readErr)
	}
	childPID, parseErr := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if parseErr != nil {
		t.Fatalf("parse child pid %q: %v", rawPID, parseErr)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(childPID) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after parent timeout", childPID)
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || !errors.Is(err, syscall.ESRCH)
}
