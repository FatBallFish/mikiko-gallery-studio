package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
)

func TestStreamGalleryArchiveAppliesResponseDeadlineAndRemovesTempFile(t *testing.T) {
	path := t.TempDir() + "/direct.zip"
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Millisecond)
	archive := &galleryexportservice.Archive{Path: path, Size: 7, ResponseDeadline: deadline}
	writer := newDeadlineBlockingWriter()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/download", nil)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	started, err := streamGalleryArchive(writer, request, archive)
	if !started || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("stream result started=%v err=%v", started, err)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("deadline stream took %s", elapsed)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary archive leaked: %v", err)
	}
	if !writer.deadlineCleared() {
		t.Fatal("response write deadline was not cleared")
	}
}

type deadlineBlockingWriter struct {
	mu       sync.Mutex
	header   http.Header
	deadline time.Time
	cleared  bool
}

func newDeadlineBlockingWriter() *deadlineBlockingWriter {
	return &deadlineBlockingWriter{header: make(http.Header)}
}
func (w *deadlineBlockingWriter) Header() http.Header { return w.header }
func (*deadlineBlockingWriter) WriteHeader(int)       {}
func (w *deadlineBlockingWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadline = deadline
	if deadline.IsZero() {
		w.cleared = true
	}
	return nil
}
func (w *deadlineBlockingWriter) Write([]byte) (int, error) {
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	if wait := time.Until(deadline); wait > 0 {
		timer := time.NewTimer(wait)
		<-timer.C
	}
	return 0, os.ErrDeadlineExceeded
}
func (w *deadlineBlockingWriter) deadlineCleared() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cleared
}
