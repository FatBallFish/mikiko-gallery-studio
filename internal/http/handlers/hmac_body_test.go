package handlers

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
)

func TestSpoolBoundedHMACBodySpillsToDiskAndRemovesOnClose(t *testing.T) {
	tempDir := t.TempDir()
	content := bytes.Repeat([]byte("x"), int(hmacBodyMemoryThresholdBytes+1024))
	body, bodyHash, appErr := spoolBoundedHMACBody(bytes.NewReader(content), int64(len(content)), tempDir)
	if appErr != nil {
		t.Fatalf("spoolBoundedHMACBody: %v", appErr)
	}
	if body.file == nil || len(body.memory) != 0 {
		t.Fatalf("expected file-backed spool above memory threshold: %#v", body)
	}
	path := body.file.Name()
	if bodyHash != apikeyservice.BodySHA256(content) {
		t.Fatalf("streamed body hash = %q, want %q", bodyHash, apikeyservice.BodySHA256(content))
	}
	got, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("read spooled body: bytes=%d err=%v", len(got), err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close spooled body: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary body file still exists after close: %v", err)
	}
}

func TestSpoolBoundedHMACBodyRejectsOverflowAndCleansTemporaryFile(t *testing.T) {
	tempDir := t.TempDir()
	content := bytes.Repeat([]byte("x"), int(hmacBodyMemoryThresholdBytes+96*1024))
	body, _, appErr := spoolBoundedHMACBody(bytes.NewReader(content), hmacBodyMemoryThresholdBytes+64*1024, tempDir)
	if body != nil || appErr == nil || appErr.StatusCode != 413 {
		t.Fatalf("expected bounded overflow error, body=%#v err=%#v", body, appErr)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files leaked after overflow: %#v", entries)
	}
}
