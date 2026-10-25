package imagetask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	artifactRangeBytes      int64 = 512 << 10
	artifactRequestTimeout        = 2 * time.Minute
	artifactTransferTimeout       = 15 * time.Minute
	artifactMaxRetries            = 4
	artifactMaxRequests           = 136 // 128 chunks at the size limit, plus bounded recovery.
)

type artifactProgressKey struct{}
type artifactDownloadProgress struct {
	mu      sync.Mutex
	last    time.Time
	service *Service
	task    domainimagetask.Task
	count   int
}

func withArtifactProgress(ctx context.Context, s *Service, task domainimagetask.Task, count int) context.Context {
	return context.WithValue(ctx, artifactProgressKey{}, &artifactDownloadProgress{service: s, task: task, count: count})
}

// Only the owned progress columns are updated; download telemetry must never save
// a stale task snapshot over the heartbeat or encrypted recovery checkpoint.
func reportArtifactDownload(ctx context.Context, index int, bytes, total int64, retry bool) {
	p, _ := ctx.Value(artifactProgressKey{}).(*artifactDownloadProgress)
	if p == nil || p.task.LeaseOwner == "" || ctx.Err() != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.last) < 2*time.Second {
		return
	}
	p.last = time.Now()
	task := p.task
	task.ProgressStage = domainimagetask.ProgressStagePersisting
	task.ProgressMessage = fmt.Sprintf("正在下载结果 %d/%d：已接收 %d 字节", index+1, p.count, bytes)
	if total >= 0 {
		task.ProgressMessage += fmt.Sprintf(" / %d 字节", total)
	}
	if retry {
		task.ProgressMessage += "，正在重试下载"
	}
	progressCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = p.service.updateOwnedProgress(progressCtx, task, task.LeaseOwner)
}

func artifactValidator(h http.Header) (string, string) {
	if etag := strings.TrimSpace(h.Get("ETag")); strings.HasPrefix(etag, "\"") && strings.HasSuffix(etag, "\"") && len(etag) > 1 {
		return "ETag", etag
	}
	if modified := h.Get("Last-Modified"); modified != "" {
		if _, err := http.ParseTime(modified); err == nil {
			return "Last-Modified", modified
		}
	}
	return "", ""
}

func parseArtifactRange(value string) (start, end, total int64, err error) {
	start, end, total = -1, -1, -1
	if !strings.HasPrefix(value, "bytes ") {
		return start, end, total, errors.New("invalid artifact Content-Range unit")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return start, end, total, errors.New("invalid artifact Content-Range")
	}
	bounds := strings.Split(parts[0], "-")
	if len(bounds) != 2 {
		return start, end, total, errors.New("invalid artifact Content-Range bounds")
	}
	start, e1 := strconv.ParseInt(bounds[0], 10, 64)
	end, e2 := strconv.ParseInt(bounds[1], 10, 64)
	total, e3 := strconv.ParseInt(parts[1], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || start < 0 || end < start || total <= end {
		return start, end, total, errors.New("invalid artifact Content-Range bounds")
	}
	return start, end, total, nil
}

// Partial bytes live only within this operation. Every append requires a range
// starting exactly at len(content), the same total, and the same validator.
func (s *Service) downloadArtifact(parent context.Context, task domainimagetask.Task, index int, source string) ([]byte, *http.Request, *http.Response, error) {
	ctx, cancel := context.WithTimeout(parent, artifactTransferTimeout)
	defer cancel()
	if ctx.Value(artifactProgressKey{}) == nil {
		ctx = withArtifactProgress(ctx, s, task, index+1)
	}
	var content []byte
	var req *http.Request
	var resp *http.Response
	var lastErr error
	total := int64(-1)
	validatorName, validator := "", ""
	retries := 0
	rangeEnabled := true
	started := s.nowUTC()
	fail := func(code string, retryable bool, cause error) ([]byte, *http.Request, *http.Response, error) {
		failure := newArtifactFailure(s, code, "read", retryable, cause)
		decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, int64(len(content)), started)
		return nil, req, resp, failure
	}
	for request := 0; request < artifactMaxRequests; request++ {
		if request > 0 && lastErr != nil {
			timer := time.NewTimer(time.Duration(retries) * 100 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return fail(errs.CodeArtifactFetchTimeout, true, ctx.Err())
			}
			lastErr = nil
		}
		if err := ctx.Err(); err != nil {
			return fail(errs.CodeArtifactFetchTimeout, true, err)
		}
		offset := int64(len(content))
		if offset > 0 && validator == "" {
			content = nil
			offset = 0
			total = -1
		}
		end := offset + artifactRangeBytes - 1
		if total >= 0 && end >= total {
			end = total - 1
		}
		requestCtx, requestCancel := context.WithTimeout(ctx, artifactRequestTimeout)
		var err error
		req, err = http.NewRequestWithContext(requestCtx, http.MethodGet, source, nil)
		if err != nil {
			requestCancel()
			return fail(errs.CodeArtifactSourceURLInvalid, false, err)
		}
		req.Header.Set("Accept-Encoding", "identity")
		if rangeEnabled {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
		}
		if offset > 0 {
			req.Header.Set("If-Range", validator)
		}
		resp, err = s.artifactHTTPClient().Do(req)
		if err != nil {
			requestCancel()
			if ctx.Err() != nil {
				return fail(errs.CodeArtifactFetchTimeout, true, ctx.Err())
			}
			lastErr = err
			if retries >= artifactMaxRetries {
				failure := classifyFetchError(s, err)
				decorateArtifactHTTPDiagnostic(s, &failure.diagnostic, req, resp, int64(len(content)), started)
				return nil, req, resp, failure
			}
			retries++
			reportArtifactDownload(ctx, index, int64(len(content)), total, true)
			continue
		}
		expected := int64(-1)
		if resp.StatusCode == http.StatusPartialContent {
			start, rangeEnd, rangeTotal, rangeErr := parseArtifactRange(resp.Header.Get("Content-Range"))
			name, value := artifactValidator(resp.Header)
			if rangeErr != nil || start != offset || rangeEnd > end || (offset > 0 && (rangeTotal != total || name != validatorName || value != validator)) || (resp.ContentLength > 0 && resp.ContentLength != rangeEnd-start+1) || resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity" {
				resp.Body.Close()
				requestCancel()
				return fail(errs.CodeArtifactFetchReadFailed, true, errors.New("artifact range or representation mismatch"))
			}
			total = rangeTotal
			validatorName, validator = name, value
			expected = rangeEnd - start + 1
			// Without a validator, fetch the whole representation next time instead of
			// joining unrelated partial responses.
			if validator == "" && rangeEnd+1 < total {
				resp.Body.Close()
				requestCancel()
				if !rangeEnabled {
					return fail(errs.CodeArtifactFetchReadFailed, false, errors.New("unsolicited partial artifact response"))
				}
				rangeEnabled = false
				content = nil
				total = -1
				continue
			}
		} else if resp.StatusCode == http.StatusOK {
			// If-Range permits a full response when the representation changed. Replace
			// rather than append, including when the server simply ignores Range.
			content = nil
			offset = 0
			total = resp.ContentLength
			if total == 0 && resp.Header.Get("Content-Length") == "" {
				total = -1
			}
			validatorName, validator = artifactValidator(resp.Header)
			expected = total
		} else {
			resp.Body.Close()
			requestCancel()
			retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
			lastErr = fmt.Errorf("upstream artifact returned status %d", resp.StatusCode)
			if !retryable || retries >= artifactMaxRetries {
				return fail(errs.CodeArtifactFetchHTTPStatus, retryable, lastErr)
			}
			retries++
			reportArtifactDownload(ctx, index, int64(len(content)), total, true)
			continue
		}
		if total > maxGeneratedArtifactBytes || expected > maxGeneratedArtifactBytes-offset {
			resp.Body.Close()
			requestCancel()
			return fail(errs.CodeArtifactSizeLimitExceeded, false, errors.New("generated artifact exceeds size limit"))
		}
		reportArtifactDownload(ctx, index, offset, total, false)
		reader := io.LimitReader(resp.Body, maxGeneratedArtifactBytes-offset+1)
		if resp.StatusCode == http.StatusPartialContent {
			reader = io.LimitReader(resp.Body, expected+1)
		}
		before := len(content)
		buf := make([]byte, 32<<10)
		for {
			n, readErr := reader.Read(buf)
			content = append(content, buf[:n]...)
			reportArtifactDownload(ctx, index, int64(len(content)), total, false)
			if int64(len(content)) > maxGeneratedArtifactBytes {
				err = errors.New("generated artifact exceeds size limit")
				break
			}
			if expected >= 0 && int64(len(content)-before) > expected {
				err = errors.New("artifact response exceeds declared range")
				break
			}
			if readErr != nil {
				if readErr != io.EOF {
					err = readErr
				}
				break
			}
			if ctx.Err() != nil {
				err = ctx.Err()
				break
			}
		}
		resp.Body.Close()
		requestCancel()
		if int64(len(content)) > maxGeneratedArtifactBytes {
			return fail(errs.CodeArtifactSizeLimitExceeded, false, err)
		}
		received := int64(len(content) - before)
		if expected >= 0 && received > expected {
			return fail(errs.CodeArtifactFetchReadFailed, false, err)
		}
		if err == nil && expected >= 0 && received != expected {
			err = io.ErrUnexpectedEOF
		}
		if ctx.Err() != nil {
			return fail(errs.CodeArtifactFetchTimeout, true, ctx.Err())
		}
		if err != nil && total >= 0 && int64(len(content)) == total {
			return fail(errs.CodeArtifactFetchReadFailed, true, err)
		}
		if err == nil && (resp.StatusCode == http.StatusOK || int64(len(content)) == total) {
			return content, req, resp, nil
		}
		if err == nil {
			continue
		}
		lastErr = err
		if retries >= artifactMaxRetries {
			return fail(errs.CodeArtifactFetchReadFailed, true, err)
		}
		retries++
		reportArtifactDownload(ctx, index, int64(len(content)), total, true)
	}
	if lastErr == nil {
		lastErr = errors.New("artifact request budget exhausted")
	}
	return fail(errs.CodeArtifactFetchReadFailed, true, lastErr)
}
