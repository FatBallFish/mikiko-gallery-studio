package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTruncateTextKeepsValidUTF8(t *testing.T) {
	value := strings.Repeat("图片", 5)
	got := TruncateText(value, 3)
	if !strings.HasPrefix(got, "图片图") || !strings.Contains(got, "truncated 7") {
		t.Fatalf("unexpected truncated value %q", got)
	}
}

func TestSanitizeURLDropsCredentialsAndQuery(t *testing.T) {
	got := SanitizeURL("https://user:pass@cdn.example.test/path/result.png?Expires=1&Signature=secret#frag")
	if got != "https://cdn.example.test/path/result.png" {
		t.Fatalf("unexpected sanitized url %q", got)
	}
}

func TestSanitizeJSONRedactsSecretsAndSignedURLs(t *testing.T) {
	got := SanitizeJSON(map[string]any{
		"api_key": "sk-secret",
		"artifacts": []any{
			map[string]any{"url": "https://cdn.example.test/a.mp4?Signature=secret", "size": 12},
		},
		"message": "ok",
	}, 2000)
	for _, forbidden := range []string{"sk-secret", "Signature=secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized json leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "[redacted]") || !strings.Contains(got, "https://cdn.example.test/a.mp4") {
		t.Fatalf("unexpected sanitized json %s", got)
	}
}

func TestRotatingWriterCapsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	writer, err := NewRotatingWriter(Options{Path: path, MaxBytes: 16, MaxBackups: 2, MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	defer writer.Close()
	for index := 0; index < 10; index++ {
		if _, err := writer.Write([]byte("0123456789abcdef\n")); err != nil {
			t.Fatalf("write %d: %v", index, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	backups := 0
	for _, entry := range entries {
		if entry.Name() != "app.log" {
			backups++
		}
	}
	if backups > 2 {
		t.Fatalf("expected at most 2 backups, got %d", backups)
	}
}

func TestRotatingWriterRemovesExpiredBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	expired := path + ".20260101T000000.000000000"
	if err := os.WriteFile(expired, []byte("old"), 0o640); err != nil {
		t.Fatalf("write expired backup: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	writer, err := NewRotatingWriter(Options{Path: path, MaxBytes: 1024, MaxBackups: 5, MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	defer writer.Close()
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired backup still exists: %v", err)
	}
}
