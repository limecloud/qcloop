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

type executableProbeConfig struct {
	DisplayName string
	BinaryName  string
	BinaryEnv   string
	CachedPath  string
	VersionArgs []string
}

func runCLICommand(ctx context.Context, path string, args []string, dir string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command(path, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = strings.TrimSpace(dir)
	}
	prepareCommandForContext(cmd)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err = cmd.Start(); err != nil {
		return "", "", -1, fmt.Errorf("执行失败: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	timedOut := false
	select {
	case err = <-done:
	case <-ctx.Done():
		timedOut = true
		_ = terminateCommandForContext(cmd)
		err = <-done
	}

	stdout = outBuf.String()
	stderr = errBuf.String()
	if timedOut {
		if stderr != "" {
			stderr += "\n"
		}
		stderr += fmt.Sprintf("执行超时或已取消: %v", ctx.Err())
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdout, stderr, exitErr.ExitCode(), fmt.Errorf("执行超时或已取消: %w", ctx.Err())
		}
		return stdout, stderr, -1, fmt.Errorf("执行超时或已取消: %w", ctx.Err())
	}
	if err == nil {
		return stdout, stderr, 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, exitErr.ExitCode(), nil
	}
	return stdout, stderr, -1, fmt.Errorf("执行失败: %w", err)
}

func resolveExecutablePath(ctx context.Context, cfg executableProbeConfig) (path string, probeError string, err error) {
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.BinaryName
	}
	if len(cfg.VersionArgs) == 0 {
		cfg.VersionArgs = []string{"--version"}
	}

	if cfg.CachedPath != "" {
		if err := probeExecutable(ctx, cfg.CachedPath, cfg.VersionArgs); err == nil {
			return cfg.CachedPath, "", nil
		} else if strings.TrimSpace(os.Getenv(cfg.BinaryEnv)) != "" {
			return "", "", fmt.Errorf("%s 不可用: %s: %w", cfg.BinaryEnv, cfg.CachedPath, err)
		}
	}

	if explicit := strings.TrimSpace(os.Getenv(cfg.BinaryEnv)); explicit != "" {
		if err := probeExecutable(ctx, explicit, cfg.VersionArgs); err == nil {
			return explicit, "", nil
		} else {
			return "", "", fmt.Errorf("%s 不可用: %s: %w", cfg.BinaryEnv, explicit, err)
		}
	}

	var probeErrors []string
	for _, candidate := range executableCandidates(cfg.BinaryName) {
		if err := probeExecutable(ctx, candidate, cfg.VersionArgs); err == nil {
			return candidate, "", nil
		} else {
			probeErrors = append(probeErrors, fmt.Sprintf("%s: %v", candidate, err))
		}
	}

	probeError = strings.Join(probeErrors, "; ")
	if probeError == "" {
		probeError = "PATH 中没有可执行候选"
	}
	return "", probeError, fmt.Errorf("没有找到可用 %s；请修复 PATH 或设置 %s。探测结果: %s", cfg.DisplayName, cfg.BinaryEnv, probeError)
}

func executableCandidates(binaryName string) []string {
	seen := map[string]bool{}
	var candidates []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, binaryName)
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

func probeExecutable(ctx context.Context, path string, args []string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, path, args...)
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
