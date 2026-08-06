package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	hmacBodyInitialCapacityBytes int64 = 32 * 1024
	hmacBodyMemoryThresholdBytes       = 1024 * 1024
)

type spooledHMACBody struct {
	memory []byte
	reader *bytes.Reader
	file   *os.File
	path   string
	closed bool
}

func spoolBoundedHMACBody(source io.Reader, limit int64, tempDir string) (*spooledHMACBody, string, *errs.Error) {
	if source == nil {
		source = bytes.NewReader(nil)
	}
	if limit < 0 {
		return nil, "", errs.BadRequest("invalid request body limit")
	}
	body := &spooledHMACBody{memory: make([]byte, 0, minInt64(limit, hmacBodyInitialCapacityBytes))}
	hasher := sha256.New()
	limited := io.LimitReader(source, limit+1)
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		count, readErr := limited.Read(buffer)
		if count > 0 {
			total += int64(count)
			if total > limit {
				_ = body.Close()
				return nil, "", errs.New(http.StatusRequestEntityTooLarge, errs.CodeValidationFailed, "request body too large")
			}
			_, _ = hasher.Write(buffer[:count])
			if err := body.write(buffer[:count], tempDir); err != nil {
				_ = body.Close()
				return nil, "", errs.Internal("failed to buffer request body")
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = body.Close()
			return nil, "", errs.BadRequest("failed to read request body")
		}
	}
	if err := body.rewind(); err != nil {
		_ = body.Close()
		return nil, "", errs.Internal("failed to buffer request body")
	}
	return body, base64.RawURLEncoding.EncodeToString(hasher.Sum(nil)), nil
}

func (b *spooledHMACBody) write(content []byte, tempDir string) error {
	if b.file == nil && int64(len(b.memory)+len(content)) <= hmacBodyMemoryThresholdBytes {
		b.memory = append(b.memory, content...)
		return nil
	}
	if b.file == nil {
		file, err := os.CreateTemp(tempDir, "mgs-openapi-body-*")
		if err != nil {
			return err
		}
		b.file = file
		b.path = file.Name()
		if len(b.memory) > 0 {
			if _, err := b.file.Write(b.memory); err != nil {
				return err
			}
			b.memory = nil
		}
	}
	_, err := b.file.Write(content)
	return err
}

func (b *spooledHMACBody) rewind() error {
	if b.file != nil {
		_, err := b.file.Seek(0, io.SeekStart)
		return err
	}
	b.reader = bytes.NewReader(b.memory)
	return nil
}

func (b *spooledHMACBody) Read(target []byte) (int, error) {
	if b.closed {
		return 0, os.ErrClosed
	}
	if b.file != nil {
		return b.file.Read(target)
	}
	return b.reader.Read(target)
}

func (b *spooledHMACBody) Close() error {
	if b == nil || b.closed {
		return nil
	}
	b.closed = true
	b.memory = nil
	b.reader = nil
	var closeErr error
	if b.file != nil {
		closeErr = b.file.Close()
		b.file = nil
	}
	var removeErr error
	if b.path != "" {
		removeErr = os.Remove(b.path)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		b.path = ""
	}
	return errors.Join(closeErr, removeErr)
}

func minInt64(left, right int64) int {
	if left < right {
		return int(left)
	}
	return int(right)
}
