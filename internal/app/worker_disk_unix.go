//go:build !windows

package app

import "golang.org/x/sys/unix"

func diskUsage(path string) (int, int64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(path, &stats); err != nil {
		return 0, 0, err
	}
	total := uint64(stats.Blocks)
	available := uint64(stats.Bavail)
	if total == 0 {
		return 100, 0, nil
	}
	used := total - available
	return int((used*100 + total - 1) / total), int64(available * uint64(stats.Bsize)), nil
}
