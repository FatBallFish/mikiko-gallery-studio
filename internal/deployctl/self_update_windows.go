//go:build windows

package deployctl

import (
	"os"
	"os/exec"
)

func replaceDeployctlExecutable(current, staged string) (bool, error) {
	script := windowsSelfUpdateScript(current, staged, os.Getpid())
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err := command.Start(); err != nil {
		return false, err
	}
	return true, command.Process.Release()
}
