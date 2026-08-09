//go:build js || plan9 || wasip1 || windows

package storage

import (
	"errors"
	"io"
	"os"
)

const localDirectoryFallbackChunkSize = 16

func readLocalDirectoryChunk(directory string, offset int64) ([]string, int64, bool, error) {
	file, err := os.Open(directory)
	if err != nil {
		return nil, offset, false, err
	}
	defer file.Close()
	remaining := offset
	for remaining > 0 {
		step := int64(localDirectoryFallbackChunkSize)
		if remaining < step {
			step = remaining
		}
		entries, err := file.ReadDir(int(step))
		remaining -= int64(len(entries))
		if err != nil {
			if errors.Is(err, io.EOF) && remaining == 0 {
				break
			}
			return nil, offset, false, err
		}
	}
	entries, err := file.ReadDir(localDirectoryFallbackChunkSize)
	eof := errors.Is(err, io.EOF)
	if err != nil && !eof {
		return nil, offset, false, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, offset + int64(len(names)), eof, nil
}
