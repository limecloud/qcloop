//go:build windows

package executor

import "os/exec"

func prepareCommandForContext(cmd *exec.Cmd) {}

func terminateCommandForContext(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
