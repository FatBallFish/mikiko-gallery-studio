package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var ErrInvalidRange = errors.New("invalid storage byte range")

// RangeBackend opens only the requested bytes. Implementations must never
// satisfy a range by reading and discarding an object's prefix. MetadataOnly
// requests return no reader and transfer no object body.
type RangeBackend interface {
	OpenRange(context.Context, string, string, int64, bool) (RangeStream, error)
}
type RangeStream struct {
	Reader           io.ReadCloser
	Size, Start, End int64
	Partial          bool
}

func ParseByteRange(value string, size int64) (start, end int64, partial bool, err error) {
	value = strings.TrimSpace(value)
	if size <= 0 {
		return 0, 0, false, ErrInvalidRange
	}
	if value == "" {
		return 0, size - 1, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, false, ErrInvalidRange
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, false, ErrInvalidRange
	}
	if parts[0] == "" {
		n, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || n <= 0 {
			return 0, 0, false, ErrInvalidRange
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true, nil
	}
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, ErrInvalidRange
	}
	end = size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false, ErrInvalidRange
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true, nil
}
func rangeMetadata(value string, size, maxBytes int64) (RangeStream, error) {
	result := RangeStream{Size: size}
	if size < 0 {
		return result, fmt.Errorf("unknown storage content length")
	}
	if size > maxBytes {
		return result, ErrObjectTooLarge
	}
	var err error
	result.Start, result.End, result.Partial, err = ParseByteRange(value, size)
	return result, err
}
func (b *LocalBackend) OpenRange(ctx context.Context, key, value string, maxBytes int64, metadataOnly bool) (RangeStream, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return RangeStream{}, err
	}
	if err := contextError(ctx); err != nil {
		return RangeStream{}, err
	}
	fullPath, ok := b.resolvePath(key)
	if !ok {
		return RangeStream{}, ErrNotFound
	}
	file, err := os.Open(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return RangeStream{}, ErrNotFound
	}
	if err != nil {
		return RangeStream{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return RangeStream{}, err
	}
	result, err := rangeMetadata(value, info.Size(), maxBytes)
	if err != nil || metadataOnly {
		_ = file.Close()
		return result, err
	}
	if _, err = file.Seek(result.Start, io.SeekStart); err != nil {
		_ = file.Close()
		return RangeStream{}, err
	}
	result.Reader = &rangeReadCloser{Reader: io.LimitReader(newBoundedStreamReader(ctx, file, result.End-result.Start+1), result.End-result.Start+1), Closer: file}
	return result, nil
}

type rangeReadCloser struct {
	io.Reader
	io.Closer
}

func (b *S3Backend) OpenRange(ctx context.Context, key, value string, maxBytes int64, metadataOnly bool) (RangeStream, error) {
	if err := validateBoundedReadLimit(maxBytes); err != nil {
		return RangeStream{}, err
	}
	if err := contextError(ctx); err != nil {
		return RangeStream{}, err
	}
	if strings.TrimSpace(value) == "" && !metadataOnly {
		reader, size, err := b.OpenReader(ctx, key, maxBytes)
		if err != nil {
			return RangeStream{}, err
		}
		result, err := rangeMetadata("", size, maxBytes)
		if err != nil {
			_ = reader.Close()
			return result, err
		}
		result.Reader = reader
		return result, nil
	}
	// HEAD obtains authoritative length without beginning an unbounded GET.
	req, err := b.newSignedRequest(ctx, http.MethodHead, key, "", nil)
	if err != nil {
		return RangeStream{}, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return RangeStream{}, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return RangeStream{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return RangeStream{}, fmt.Errorf("head s3 range object: status=%d", resp.StatusCode)
	}
	result, err := rangeMetadata(value, resp.ContentLength, maxBytes)
	if err != nil || metadataOnly {
		return result, err
	}
	req, err = b.newSignedRequest(ctx, http.MethodGet, key, "", nil)
	if err != nil {
		return RangeStream{}, err
	}
	if result.Partial {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", result.Start, result.End))
	}
	// Prevent serving a different object if storage changed between HEAD and GET.
	if etag := resp.Header.Get("ETag"); etag != "" {
		req.Header.Set("If-Match", etag)
	}
	resp, err = b.client.Do(req)
	if err != nil {
		return RangeStream{}, err
	}
	expectedStatus := http.StatusOK
	if result.Partial {
		expectedStatus = http.StatusPartialContent
	}
	length := result.End - result.Start + 1
	if resp.StatusCode != expectedStatus || resp.ContentLength != length || result.Partial && resp.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", result.Start, result.End, result.Size) {
		_ = resp.Body.Close()
		return RangeStream{}, fmt.Errorf("s3 range response does not match requested bytes")
	}
	result.Reader = newBoundedStreamReader(ctx, resp.Body, length)
	return result, nil
}
