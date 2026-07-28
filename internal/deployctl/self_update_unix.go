//go:build !windows

package deployctl

import "os"

func replaceDeployctlExecutable(current, staged string) (bool, error) {
	return false, os.Rename(staged, current)
}
