//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package storage

import (
	"io"
	"os"
	"syscall"
)

const localDirectoryReadBufferSize = 512

func readLocalDirectoryChunk(directory string, offset int64) ([]string, int64, bool, error) {
	file, err := os.Open(directory)
	if err != nil {
		return nil, offset, false, err
	}
	defer file.Close()
	fd := int(file.Fd())
	if offset > 0 {
		if _, err := syscall.Seek(fd, offset, io.SeekStart); err != nil {
			return nil, offset, false, err
		}
	}
	buffer := make([]byte, localDirectoryReadBufferSize)
	read, err := syscall.ReadDirent(fd, buffer)
	if err != nil {
		return nil, offset, false, err
	}
	nextOffset, err := syscall.Seek(fd, 0, io.SeekCurrent)
	if err != nil {
		return nil, offset, false, err
	}
	if read == 0 {
		return nil, nextOffset, true, nil
	}
	_, _, names := syscall.ParseDirent(buffer[:read], -1, nil)
	return names, nextOffset, false, nil
}
