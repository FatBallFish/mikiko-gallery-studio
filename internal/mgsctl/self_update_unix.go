//go:build !windows

package mgsctl

import "os"

func replaceMGSCTLExecutable(current, staged string) (bool, error) {
	return false, os.Rename(staged, current)
}
