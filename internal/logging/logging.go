// Package logging provides bounded log output and helpers for safe
// structured logging of generation requests.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxBytes   = 100 << 20
	DefaultMaxBackups = 5
	DefaultMaxAge     = 14 * 24 * time.Hour
	cleanupInterval   = time.Hour
)

// RotatingWriter appends to a log file and rotates it by size. Backups are
// pruned by count and age so a long-running service cannot fill the disk.
type RotatingWriter struct {
	path       string
	maxBytes   int64
	maxBackups int
	maxAge     time.Duration
	now        func() time.Time

	mu     sync.Mutex
	file   *os.File
	size   int64
	stop   chan struct{}
	closed bool
}

type Options struct {
	Path       string
	MaxBytes   int64
	MaxBackups int
	MaxAge     time.Duration
	Now        func() time.Time
}

func NewRotatingWriter(options Options) (*RotatingWriter, error) {
	path := strings.TrimSpace(options.Path)
	if path == "" {
		return nil, fmt.Errorf("log file path is empty")
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxBackups <= 0 {
		options.MaxBackups = DefaultMaxBackups
	}
	if options.MaxAge <= 0 {
		options.MaxAge = DefaultMaxAge
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	writer := &RotatingWriter{
		path: path, maxBytes: options.MaxBytes, maxBackups: options.MaxBackups,
		maxAge: options.MaxAge, now: options.Now, stop: make(chan struct{}),
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	writer.file = file
	writer.size = info.Size()
	writer.cleanupLocked()
	go writer.cleanupLoop()
	return writer, nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.file == nil {
		if err := w.openLocked(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	close(w.stop)
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingWriter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.mu.Lock()
			if !w.closed {
				w.cleanupLocked()
			}
			w.mu.Unlock()
		}
	}
}

func (w *RotatingWriter) openLocked() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) rotateLocked() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	backup := w.path + "." + w.now().UTC().Format("20060102T150405.000000000")
	if _, err := os.Stat(backup); err == nil {
		backup += fmt.Sprintf(".%d", w.now().UnixNano())
	}
	if err := os.Rename(w.path, backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := w.openLocked(); err != nil {
		return err
	}
	w.cleanupLocked()
	return nil
}

func (w *RotatingWriter) cleanupLocked() {
	entries, err := os.ReadDir(filepath.Dir(w.path))
	if err != nil {
		return
	}
	prefix := filepath.Base(w.path) + "."
	var backups []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		backups = append(backups, entry)
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name() > backups[j].Name() })
	cutoff := w.now().Add(-w.maxAge)
	for index, entry := range backups {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if index >= w.maxBackups || info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(filepath.Dir(w.path), entry.Name()))
		}
	}
}

// TruncateText bounds log values while keeping enough context to reproduce a
// generation request. The returned value is always valid UTF-8.
func TruncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + fmt.Sprintf("...(truncated %d chars)", len(runes)-limit)
}

// SanitizeURL removes credentials, query strings, and fragments. Signed
// artifact URLs must never be written to logs because they grant access.
func SanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return TruncateText(raw, 512)
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return TruncateText(parsed.String(), 2048)
}

// SanitizedURLs maps a list of URLs through SanitizeURL.
func SanitizedURLs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if sanitized := SanitizeURL(value); sanitized != "" {
			out = append(out, sanitized)
		}
	}
	return out
}

// MultiWriter returns stdout plus the rotating file when configured.
func MultiWriter(stdout io.Writer, file *RotatingWriter) io.Writer {
	if file == nil {
		return stdout
	}
	return io.MultiWriter(stdout, file)
}

// SanitizeJSON renders provider payloads for logs with URL query strings,
// credential-like keys, and oversized values removed.
func SanitizeJSON(value any, limit int) string {
	data, err := json.Marshal(sanitizeJSONValue(value, 0))
	if err != nil {
		return ""
	}
	return TruncateText(string(data), limit)
}

func sanitizeJSONValue(value any, depth int) any {
	if depth > 6 {
		return "..."
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveLogKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = sanitizeJSONValue(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeJSONValue(item, depth+1))
		}
		return out
	case string:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(typed)), "http://") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(typed)), "https://") {
			return SanitizeURL(typed)
		}
		return TruncateText(typed, 2000)
	default:
		return typed
	}
}

func sensitiveLogKey(key string) bool {
	key = strings.ToLower(key)
	for _, fragment := range []string{"api_key", "apikey", "secret", "password", "authorization", "signature", "access_token", "refresh_token"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
