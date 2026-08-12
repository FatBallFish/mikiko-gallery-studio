//go:build !windows

package storage

import "golang.org/x/sys/unix"

func localAvailableDiskBytes(path string) (int64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(path, &stats); err != nil {
		return 0, err
	}
	return int64(uint64(stats.Bavail) * uint64(stats.Bsize)), nil
}
