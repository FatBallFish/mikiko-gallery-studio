//go:build windows

package main

func restoreLocalFileOwnership(_, _ string) error {
	return nil
}
