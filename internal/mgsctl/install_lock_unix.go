//go:build !windows

package mgsctl

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openInstallLockFile(path string) (*os.File, error) {
	flags := unix.O_CREAT | unix.O_EXCL | unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Open(path, flags, 0o600)
	created := err == nil
	if errors.Is(err, syscall.EEXIST) {
		fd, err = unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = unix.Close(fd)
		}
	}()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("install lock must be an owner-controlled regular file")
	}
	if created {
		if err := unix.Fchmod(fd, 0o600); err != nil {
			return nil, err
		}
	} else if stat.Mode&0o777 != 0o600 {
		return nil, fmt.Errorf("existing install lock permissions must be 0600")
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		return nil, fmt.Errorf("open install lock")
	}
	closeOnError = false
	return file, nil
}

func tryLockInstallFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockInstallFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
