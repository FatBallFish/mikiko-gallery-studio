package imagetask

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type interruptedArtifactBody struct{ data *bytes.Reader }

func (b interruptedArtifactBody) Read(p []byte) (int, error) {
	n, err := b.data.Read(p)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}
func (b interruptedArtifactBody) Close() error { return nil }

func transferTestService(t *testing.T, transport artifactRoundTripFunc) *Service {
	t.Helper()
	backend := &countingArtifactBackend{objects: map[string][]byte{}}
	s := NewServiceWithProvidersStoreAssetsBillingAndRouter(artifactRecoveryTestConfig(), nil, NewMemoryStore(), nil, nil, storage.NewStaticRouter(backend))
	s.SetHTTPClient(&http.Client{Transport: transport})
	return s
}
func transferResponse(status int, body io.ReadCloser, length int64, contentRange, etag string) *http.Response {
	h := http.Header{}
	if contentRange != "" {
		h.Set("Content-Range", contentRange)
	}
	if etag != "" {
		h.Set("ETag", etag)
	}
	return &http.Response{StatusCode: status, Header: h, ContentLength: length, Body: body}
}

func TestArtifactDownloadRetainsInterruptedRange(t *testing.T) {
	payload := bytes.Repeat([]byte("abcdef"), 200000)
	calls := 0
	s := transferTestService(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var start, end int64
		fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
		if end >= int64(len(payload)) {
			end = int64(len(payload)) - 1
		}
		if calls == 2 && (start != 100000 || r.Header.Get("If-Range") != "\"stable\"") {
			t.Errorf("resume offset/validator: %s %s", r.Header.Get("Range"), r.Header.Get("If-Range"))
		}
		var body io.ReadCloser = io.NopCloser(bytes.NewReader(payload[start : end+1]))
		if calls == 1 {
			body = interruptedArtifactBody{bytes.NewReader(payload[:100000])}
		}
		return transferResponse(206, body, end-start+1, fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)), "\"stable\""), nil
	})
	got, _, _, err := s.downloadArtifact(context.Background(), domainimagetask.Task{}, 0, "https://artifact.test/image")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download bytes=%d calls=%d err=%v", len(got), calls, err)
	}
	if calls != 4 {
		t.Fatalf("calls=%d want 4", calls)
	}
}

func TestArtifactDownloadRangeSafety(t *testing.T) {
	for _, mode := range []string{"ignored", "changed-full", "changed-etag", "bad-offset", "changed-total", "oversized-range", "overlong-body", "last-modified"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			s := transferTestService(t, func(r *http.Request) (*http.Response, error) {
				calls++
				etag := "\"one\""
				if calls == 1 {
					resp := transferResponse(206, interruptedArtifactBody{bytes.NewReader([]byte("abc"))}, 6, "bytes 0-5/6", etag)
					if mode == "last-modified" {
						resp.Header.Del("ETag")
						resp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
					}
					return resp, nil
				}
				if mode == "ignored" || mode == "changed-full" {
					return transferResponse(200, io.NopCloser(strings.NewReader("NEWNEW")), 6, "", "\"two\""), nil
				}
				cr := "bytes 3-5/6"
				body := "def"
				switch mode {
				case "changed-etag":
					etag = "\"two\""
				case "bad-offset":
					cr = "bytes 2-4/6"
				case "changed-total":
					cr = "bytes 3-5/7"
				case "oversized-range":
					cr = "bytes 3-999999/1000000"
				case "overlong-body":
					body = "def!"
				}
				resp := transferResponse(206, io.NopCloser(strings.NewReader(body)), 3, cr, etag)
				if mode == "last-modified" {
					resp.Header.Del("ETag")
					resp.Header.Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
					if r.Header.Get("If-Range") != resp.Header.Get("Last-Modified") {
						t.Error("missing date If-Range")
					}
				}
				return resp, nil
			})
			got, _, _, err := s.downloadArtifact(context.Background(), domainimagetask.Task{}, 0, "https://artifact.test/image")
			switch mode {
			case "ignored", "changed-full":
				if err != nil || string(got) != "NEWNEW" {
					t.Fatalf("got %q err=%v", got, err)
				}
			case "last-modified":
				if err != nil || string(got) != "abcdef" {
					t.Fatalf("got %q err=%v", got, err)
				}
			default:
				if err == nil {
					t.Fatalf("unsafe range accepted: %q", got)
				}
			}
		})
	}
}

func TestArtifactDownloadCancellationSizeAndBudget(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		s := transferTestService(t, func(r *http.Request) (*http.Response, error) { cancel(); return nil, r.Context().Err() })
		_, _, _, err := s.downloadArtifact(ctx, domainimagetask.Task{}, 0, "https://artifact.test/image")
		if err == nil {
			t.Fatal("expected cancellation")
		}
	})
	t.Run("size", func(t *testing.T) {
		s := transferTestService(t, func(r *http.Request) (*http.Response, error) {
			return transferResponse(200, io.NopCloser(strings.NewReader("x")), maxGeneratedArtifactBytes+1, "", ""), nil
		})
		_, _, _, err := s.downloadArtifact(context.Background(), domainimagetask.Task{}, 0, "https://artifact.test/image")
		var failure *artifactPersistenceFailure
		if !errors.As(err, &failure) || failure.diagnostic.Code != "ARTIFACT_SIZE_LIMIT_EXCEEDED" {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("retry-budget", func(t *testing.T) {
		calls := 0
		s := transferTestService(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
		_, _, _, err := s.downloadArtifact(context.Background(), domainimagetask.Task{}, 0, "https://artifact.test/image")
		if err == nil || calls != artifactMaxRetries+1 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
}

func TestArtifactPersistenceConcurrencyPreservesOrder(t *testing.T) {
	var active, peak atomic.Int32
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	s := transferTestService(t, func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		// Existing fixture is a decodable PNG; remote persistence validates and verifies storage.
		return transferResponse(200, io.NopCloser(bytes.NewReader(artifactTestPNG(t))), int64(len(artifactTestPNG(t))), "", ""), nil
	})
	task := domainimagetask.Task{ID: "ordered-task", UserID: 1}
	results := make([]provider.ImageResult, 4)
	for i := range results {
		results[i] = provider.ImageResult{URL: "https://artifact.test/" + strconv.Itoa(i), RevisedPrompt: strconv.Itoa(i)}
	}
	done := make(chan error, 1)
	go func() {
		got, err := s.persistImageResults(context.Background(), task, results)
		if err == nil {
			for i, item := range got {
				if item.ID != deterministicImageResultID(task.ID, i) || item.RevisedPrompt != strconv.Itoa(i) {
					err = fmt.Errorf("out of order result %d", i)
				}
			}
		}
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("persistence did not overlap")
		}
	}
	if len(entered) != 0 {
		t.Fatal("more than two requests started")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak=%d", peak.Load())
	}
}

func artifactTestPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestArtifactDownloadNoValidatorFallsBackWithoutConcatenating(t *testing.T) {
	calls := 0
	s := transferTestService(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return transferResponse(206, io.NopCloser(strings.NewReader("abc")), 3, "bytes 0-2/6", ""), nil
		}
		if r.Header.Get("Range") != "" || r.Header.Get("If-Range") != "" {
			t.Error("unsafe range without validator")
		}
		return transferResponse(200, io.NopCloser(strings.NewReader("NEWNEW")), 6, "", ""), nil
	})
	got, _, _, err := s.downloadArtifact(context.Background(), domainimagetask.Task{}, 0, "https://artifact.test/image")
	if err != nil || string(got) != "NEWNEW" || calls != 2 {
		t.Fatalf("got=%q calls=%d err=%v", got, calls, err)
	}
}

type artifactProgressSpy struct {
	Store
	calls          int
	owner, message string
	at             time.Time
}

func (s *artifactProgressSpy) UpdateProgressIfOwned(_ context.Context, _ string, owner, stage, message string, at time.Time) error {
	s.calls++
	s.owner = owner
	s.message = message
	s.at = at
	if stage != domainimagetask.ProgressStagePersisting {
		return errors.New("unexpected progress stage")
	}
	return errors.New("telemetry unavailable")
}
func TestArtifactDownloadProgressOwnedThrottledAndNonfatal(t *testing.T) {
	s := transferTestService(t, nil)
	spy := &artifactProgressSpy{Store: s.store}
	s.store = spy
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	task := domainimagetask.Task{ID: "task", LeaseOwner: "worker"}
	ctx := withArtifactProgress(context.Background(), s, task, 3)
	reportArtifactDownload(ctx, 1, 1024, 4096, false)
	reportArtifactDownload(ctx, 1, 2048, 4096, true)
	if spy.calls != 1 || spy.owner != "worker" || !spy.at.Equal(now) || !strings.Contains(spy.message, "2/3") || !strings.Contains(spy.message, "1024") {
		t.Fatalf("progress=%+v", spy)
	}
	p := ctx.Value(artifactProgressKey{}).(*artifactDownloadProgress)
	p.last = time.Now().Add(-3 * time.Second)
	reportArtifactDownload(ctx, 1, 2048, 4096, true)
	if spy.calls != 2 || !strings.Contains(spy.message, "重试") {
		t.Fatalf("retry progress=%+v", spy)
	}
	reportArtifactDownload(withArtifactProgress(context.Background(), s, domainimagetask.Task{}, 1), 0, 10, -1, false)
	if spy.calls != 2 {
		t.Fatal("unowned progress updated")
	}
}
