package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	providervideo "github.com/fatballfish/pic-gallery/internal/provider/video"
	videocallback "github.com/fatballfish/pic-gallery/internal/service/videocallback"
	"github.com/fatballfish/pic-gallery/internal/storage"
	workervideo "github.com/fatballfish/pic-gallery/internal/worker/video"
	"github.com/google/uuid"
)

const (
	acceptanceFileSize = int64(1 << 30)
	acceptancePartSize = int64(16 << 20)
)

func TestMultimediaLoadAcceptance(t *testing.T) {
	requireMultimediaAcceptance(t)
	t.Run("100_and_500_estimates", testEstimateLoad)
	t.Run("1000_due_polls_release_leases", testDuePollLoad)
	t.Run("50_callback_poll_races_are_monotonic", testCallbackPollRaceLoad)
	t.Run("canvas_200_nodes_300_edges", testCanvasLoad)
}

func TestMultimediaLocalOneGiBAcceptance(t *testing.T) {
	requireMultimediaAcceptance(t)
	root := t.TempDir()
	backend := storage.NewLocalBackend(root)
	multipart := any(backend).(storage.MultipartBackend)
	before := heapInUse()
	upload, err := multipart.CreateMultipart(t.Context(), storage.MultipartCreateRequest{
		ObjectKey: "media/original/acceptance/local-one-gib.mp4", ContentType: "video/mp4",
		SizeBytes: acceptanceFileSize, PartSize: acceptancePartSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	checksum := zeroChecksum(t, acceptancePartSize)
	parts := make([]storage.CompletedPart, 0, upload.PartCount)
	for partNumber := 1; partNumber <= upload.PartCount/2; partNumber++ {
		part, putErr := multipart.PutMultipartPart(t.Context(), upload, partNumber, io.LimitReader(zeroReader{}, acceptancePartSize), acceptancePartSize, checksum)
		if putErr != nil {
			t.Fatalf("put first-half part %d: %v", partNumber, putErr)
		}
		parts = append(parts, part)
	}

	// A new backend instance must recover uploaded parts without process memory.
	restarted := storage.NewLocalBackend(root)
	multipart = any(restarted).(storage.MultipartBackend)
	status, err := multipart.MultipartStatus(t.Context(), upload)
	if err != nil || len(status.CompletedParts) != upload.PartCount/2 {
		t.Fatalf("resume status=%#v err=%v", status, err)
	}
	for partNumber := upload.PartCount/2 + 1; partNumber <= upload.PartCount; partNumber++ {
		part, putErr := multipart.PutMultipartPart(t.Context(), upload, partNumber, io.LimitReader(zeroReader{}, acceptancePartSize), acceptancePartSize, checksum)
		if putErr != nil {
			t.Fatalf("put second-half part %d: %v", partNumber, putErr)
		}
		parts = append(parts, part)
	}
	diskBeforeComplete := directoryBytes(t, root)
	completed, err := multipart.CompleteMultipart(t.Context(), upload, parts)
	if err != nil {
		t.Fatal(err)
	}
	if completed.SizeBytes != acceptanceFileSize || completed.SHA256 != zeroChecksum(t, acceptanceFileSize) {
		t.Fatalf("completed object mismatch: %#v", completed)
	}
	stream, size, err := any(restarted).(storage.StreamingBackend).OpenReader(t.Context(), completed.ObjectKey, acceptanceFileSize)
	if err != nil {
		t.Fatal(err)
	}
	if size != acceptanceFileSize {
		t.Fatalf("opened object size=%d want=%d", size, acceptanceFileSize)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, stream)
	closeErr := stream.Close()
	if copyErr != nil || closeErr != nil || written != acceptanceFileSize || hex.EncodeToString(hash.Sum(nil)) != completed.SHA256 {
		t.Fatalf("stream verify bytes=%d copy=%v close=%v", written, copyErr, closeErr)
	}

	abortUpload, err := multipart.CreateMultipart(t.Context(), storage.MultipartCreateRequest{
		ObjectKey: "media/original/acceptance/cancel.mp4", ContentType: "video/mp4", SizeBytes: acceptancePartSize, PartSize: acceptancePartSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = multipart.PutMultipartPart(t.Context(), abortUpload, 1, io.LimitReader(zeroReader{}, acceptancePartSize), acceptancePartSize, checksum); err != nil {
		t.Fatal(err)
	}
	if err = multipart.AbortMultipart(t.Context(), abortUpload); err != nil {
		t.Fatal(err)
	}
	if _, err = multipart.MultipartStatus(t.Context(), abortUpload); err == nil {
		t.Fatal("aborted upload still has recoverable state")
	}
	after := heapInUse()
	delta := int64(after) - int64(before)
	if delta > 256<<20 {
		t.Fatalf("heap grew with the 1 GiB body: before=%d after=%d delta=%d", before, after, delta)
	}
	t.Logf("local_1gib bytes=%d parts=%d heap_before=%d heap_after=%d heap_delta=%d disk_before_complete=%d", acceptanceFileSize, upload.PartCount, before, after, delta, diskBeforeComplete)
}

func TestMultimediaS3OneGiBAcceptance(t *testing.T) {
	requireMultimediaAcceptance(t)
	endpoint := os.Getenv("MULTIMEDIA_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("MULTIMEDIA_MINIO_ENDPOINT is not set")
	}
	backend, err := storage.NewS3Backend(config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{
		Endpoint: endpoint, Region: "us-east-1", Bucket: os.Getenv("MULTIMEDIA_MINIO_BUCKET"),
		AccessKeyID: os.Getenv("MULTIMEDIA_MINIO_ACCESS_KEY"), SecretAccessKey: os.Getenv("MULTIMEDIA_MINIO_SECRET_KEY"), ForcePathStyle: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	multipart := any(backend).(storage.MultipartBackend)
	upload, err := multipart.CreateMultipart(t.Context(), storage.MultipartCreateRequest{
		ObjectKey: "media/original/acceptance/s3-one-gib.mp4", ContentType: "video/mp4", SizeBytes: acceptanceFileSize, PartSize: acceptancePartSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = multipart.AbortMultipart(context.Background(), upload)
		_ = backend.Delete(context.Background(), upload.ObjectKey)
	})
	checksumHex := zeroChecksum(t, acceptancePartSize)
	parts := make([]storage.CompletedPart, 0, upload.PartCount)
	client := &http.Client{Timeout: 2 * time.Minute}
	for partNumber := 1; partNumber <= upload.PartCount; partNumber++ {
		target, signErr := multipart.SignMultipartPart(t.Context(), upload, partNumber, checksumHex, 10*time.Minute)
		if signErr != nil {
			t.Fatalf("sign part %d: %v", partNumber, signErr)
		}
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPut, target.URL, io.LimitReader(zeroReader{}, acceptancePartSize))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.ContentLength = acceptancePartSize
		for key, value := range target.Headers {
			request.Header.Set(key, value)
		}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			t.Fatalf("upload part %d: %v", partNumber, requestErr)
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("upload part %d status=%d", partNumber, response.StatusCode)
		}
		etag := response.Header.Get("ETag")
		if etag == "" {
			t.Fatalf("upload part %d did not return ETag", partNumber)
		}
		parts = append(parts, storage.CompletedPart{PartNumber: partNumber, ETag: etag, Checksum: checksumHex, SizeBytes: acceptancePartSize})
		if partNumber == upload.PartCount/2 {
			status, statusErr := multipart.MultipartStatus(t.Context(), upload)
			if statusErr != nil || len(status.CompletedParts) != partNumber {
				t.Fatalf("S3 resume status=%#v err=%v", status, statusErr)
			}
		}
	}
	completed, err := multipart.CompleteMultipart(t.Context(), upload, parts)
	if err != nil || completed.SizeBytes != acceptanceFileSize || completed.ETag == "" {
		t.Fatalf("S3 complete=%#v err=%v", completed, err)
	}
	t.Logf("s3_1gib bytes=%d parts=%d etag=%s", acceptanceFileSize, upload.PartCount, completed.ETag)
}

func testEstimateLoad(t *testing.T) {
	rule := domainvideo.SalesRule{FixedTaskPoints: "1", OutputSecondPoints: "0.75", ReferenceImagePoints: "0.5", GeneratedAudioSecondPoints: "0.2", MinimumTaskPoints: "2", ReserveMarkup: "1.15"}
	for _, count := range []int{100, 500} {
		started := time.Now()
		for index := 0; index < count; index++ {
			quote, err := domainvideo.CalculateQuote(rule, domainvideo.QuoteRequest{DurationSeconds: 10, ReferenceImageCount: index % 3, GenerateAudio: index%2 == 0, OutputCount: index%4 + 1})
			if err != nil || quote.EstimatedPoints == "" || quote.MaxReservedPoints == "" {
				t.Fatalf("estimate %d/%d quote=%#v err=%v", index, count, quote, err)
			}
		}
		t.Logf("estimate_count=%d elapsed=%s", count, time.Since(started))
	}
}

func testDuePollLoad(t *testing.T) {
	now := time.Now().UTC()
	store := newPollLoadStore(1000)
	provider := &loadProvider{pollState: providervideo.StateQueued}
	runner := workervideo.NewRunner(store, loadResolver{provider: provider}, nil, workervideo.Options{Owner: "acceptance", Now: func() time.Time { return now }})
	before := runtime.NumGoroutine()
	started := time.Now()
	for index := 0; index < 1000; index++ {
		processed, err := runner.RunOnce(t.Context())
		if err != nil || !processed {
			t.Fatalf("poll %d processed=%v err=%v", index, processed, err)
		}
	}
	after := runtime.NumGoroutine()
	if store.remainingDue(now) != 0 || store.leased() != 0 || provider.pollCalls() != 1000 || after > before+8 {
		t.Fatalf("poll load due=%d leased=%d calls=%d goroutines=%d->%d", store.remainingDue(now), store.leased(), provider.pollCalls(), before, after)
	}
	t.Logf("due_polls=1000 elapsed=%s goroutines_before=%d goroutines_after=%d", time.Since(started), before, after)
}

func testCallbackPollRaceLoad(t *testing.T) {
	provider := &loadProvider{pollState: providervideo.StateRunning, callbackState: providervideo.StateSucceeded}
	state := newRaceStore()
	callback := videocallback.NewService(loadCallbackResolver{provider: provider}, state)
	runner := workervideo.NewRunner(state, loadResolver{provider: provider}, nil, workervideo.Options{Owner: "race", Now: time.Now})
	accountID := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	var wait sync.WaitGroup
	errors := make(chan error, 50)
	for index := 0; index < 50; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			if index%2 == 0 {
				_, err := callback.Receive(t.Context(), "seedance", accountID, http.Header{}, []byte("same-provider-event"))
				errors <- err
				return
			}
			_, err := runner.RunOnce(t.Context())
			errors <- err
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.item.State != domainvideo.ItemStateArtifactPending || len(state.events) != 1 || state.regressions != 0 {
		t.Fatalf("callback/poll state=%s events=%d regressions=%d", state.item.State, len(state.events), state.regressions)
	}
	t.Logf("callback_poll_races=50 duplicate_callbacks=%d poll_conflicts=%d", state.duplicates, state.pollConflicts)
}

func testCanvasLoad(t *testing.T) {
	nodes := make([]domaincanvas.Node, 0, 200)
	for index := 0; index < 100; index++ {
		nodes = append(nodes, domaincanvas.Node{ID: fmt.Sprintf("prompt-%03d", index), Type: domaincanvas.NodeTypePrompt})
		nodes = append(nodes, domaincanvas.Node{ID: fmt.Sprintf("image-gen-%03d", index), Type: domaincanvas.NodeTypeImageGeneration})
	}
	edges := make([]domaincanvas.Edge, 0, 300)
	for index := 0; index < 100; index++ {
		for offset := 0; offset < 3; offset++ {
			edges = append(edges, domaincanvas.Edge{ID: fmt.Sprintf("edge-%03d-%d", index, offset), Source: fmt.Sprintf("prompt-%03d", index), Target: fmt.Sprintf("image-gen-%03d", (index+offset)%100), InputRole: domaincanvas.InputRolePrompt})
		}
	}
	document := domaincanvas.DocumentV1{SchemaVersion: 1, Nodes: nodes, Edges: edges}
	started := time.Now()
	for index := 0; index < 100; index++ {
		if err := domaincanvas.ValidateDocument(document, domaincanvas.DefaultLimits()); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("canvas_nodes=200 canvas_edges=300 validation_iterations=100 elapsed=%s", time.Since(started))
}

func requireMultimediaAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv("MULTIMEDIA_ACCEPTANCE") != "1" {
		t.Skip("set MULTIMEDIA_ACCEPTANCE=1 to run heavy multimedia acceptance")
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func zeroChecksum(t *testing.T, size int64) string {
	t.Helper()
	hash := sha256.New()
	if written, err := io.Copy(hash, io.LimitReader(zeroReader{}, size)); err != nil || written != size {
		t.Fatalf("hash zero stream: bytes=%d err=%v", written, err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func heapInUse() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapInuse
}

func directoryBytes(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	err := filepathWalk(root, func(size int64) { total += size })
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func filepathWalk(root string, add func(int64)) error {
	return filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			add(info.Size())
		}
		return nil
	})
}

type loadProvider struct {
	mu            sync.Mutex
	polls         int
	pollState     providervideo.State
	callbackState providervideo.State
}

func (*loadProvider) Submit(context.Context, providervideo.Request) (providervideo.Job, error) {
	return providervideo.Job{}, nil
}
func (provider *loadProvider) Get(_ context.Context, ref providervideo.JobRef) (providervideo.Status, error) {
	provider.mu.Lock()
	provider.polls++
	provider.mu.Unlock()
	return providervideo.Status{JobID: ref.ID, State: provider.pollState}, nil
}
func (*loadProvider) Cancel(context.Context, providervideo.JobRef) (providervideo.CancelResult, error) {
	return providervideo.CancelResult{}, nil
}
func (provider *loadProvider) VerifyCallback(context.Context, http.Header, []byte) (providervideo.CallbackEvent, error) {
	return providervideo.CallbackEvent{EventID: "shared-event", JobID: "job-race", Status: providervideo.Status{JobID: "job-race", State: provider.callbackState, Artifacts: []providervideo.Artifact{{URL: "https://media.example.test/result.mp4"}}}}, nil
}
func (*loadProvider) NormalizeUsage(providervideo.Status) (providervideo.Usage, error) {
	return providervideo.Usage{OutputSeconds: "5.000"}, nil
}
func (provider *loadProvider) pollCalls() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.polls
}

type loadResolver struct{ provider providervideo.Provider }

func (resolver loadResolver) Resolve(context.Context, workervideo.ProviderRef) (workervideo.ResolvedExecution, error) {
	return workervideo.ResolvedExecution{Provider: resolver.provider}, nil
}

type loadCallbackResolver struct{ provider providervideo.Provider }

func (resolver loadCallbackResolver) ResolveProvider(context.Context, string, uuid.UUID) (videocallback.ResolvedProvider, error) {
	return videocallback.ResolvedProvider{ModelAccountID: 1, Provider: resolver.provider}, nil
}

type pollLoadStore struct {
	mu    sync.Mutex
	items []workervideo.WorkItem
}

func newPollLoadStore(count int) *pollLoadStore {
	now := time.Now().Add(-time.Minute)
	items := make([]workervideo.WorkItem, count)
	for index := range items {
		items[index] = workervideo.WorkItem{ID: "poll-" + strconv.Itoa(index), TaskID: "task-" + strconv.Itoa(index), State: domainvideo.ItemStateProviderQueued, Version: 1, NextActionAt: &now, Attempt: workervideo.Attempt{ID: "attempt", JobID: "job-" + strconv.Itoa(index), ProviderCode: "fake"}}
	}
	return &pollLoadStore{items: items}
}

func (store *pollLoadStore) ClaimDue(_ context.Context, request workervideo.ClaimRequest) (workervideo.WorkItem, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for index := range store.items {
		item := &store.items[index]
		if item.LeaseOwner == "" && (item.NextActionAt == nil || !item.NextActionAt.After(request.Now)) {
			item.LeaseOwner, item.LeaseExpiresAt = request.Owner, request.Now.Add(request.LeaseTTL)
			return *item, true, nil
		}
	}
	return workervideo.WorkItem{}, false, nil
}
func (*pollLoadStore) PrepareAttempt(context.Context, workervideo.PrepareAttemptRequest) (workervideo.WorkItem, error) {
	panic("unexpected PrepareAttempt")
}
func (store *pollLoadStore) ApplyStep(_ context.Context, request workervideo.ApplyStepRequest) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for index := range store.items {
		item := &store.items[index]
		if item.ID != request.ItemID || item.Version != request.ExpectedVersion || item.LeaseOwner != request.Owner {
			continue
		}
		item.NextActionAt = request.NextActionAt
		item.LeaseOwner = ""
		item.Version++
		return true, nil
	}
	return false, nil
}
func (*pollLoadStore) CommitArtifact(context.Context, workervideo.ArtifactCommitRequest) (bool, error) {
	panic("unexpected CommitArtifact")
}
func (*pollLoadStore) LoadSettlement(context.Context, string) (workervideo.SettlementSnapshot, error) {
	panic("unexpected LoadSettlement")
}
func (*pollLoadStore) FinalizeTask(context.Context, workervideo.FinalizeRequest) (bool, error) {
	panic("unexpected FinalizeTask")
}
func (store *pollLoadStore) ReleaseLease(_ context.Context, ref workervideo.LeaseRef) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for index := range store.items {
		if store.items[index].ID == ref.ItemID && store.items[index].LeaseOwner == ref.Owner {
			store.items[index].LeaseOwner = ""
		}
	}
	return nil
}
func (store *pollLoadStore) remainingDue(now time.Time) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	for _, item := range store.items {
		if item.NextActionAt == nil || !item.NextActionAt.After(now) {
			count++
		}
	}
	return count
}
func (store *pollLoadStore) leased() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	for _, item := range store.items {
		if item.LeaseOwner != "" {
			count++
		}
	}
	return count
}

type raceStore struct {
	mu            sync.Mutex
	item          workervideo.WorkItem
	events        map[string]struct{}
	duplicates    int
	pollConflicts int
	regressions   int
}

func newRaceStore() *raceStore {
	return &raceStore{item: workervideo.WorkItem{ID: "item-race", TaskID: "task-race", State: domainvideo.ItemStateProviderQueued, Version: 1, Attempt: workervideo.Attempt{ID: "attempt-race", JobID: "job-race", ProviderCode: "fake"}}, events: map[string]struct{}{}}
}
func (store *raceStore) ResolveProvider(context.Context, string, uuid.UUID) (videocallback.ResolvedProvider, error) {
	panic("use resolver")
}
func (store *raceStore) RecordEvent(_ context.Context, record videocallback.EventRecord) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.events[record.ProviderEventID]; exists {
		store.duplicates++
		return true, nil
	}
	store.events[record.ProviderEventID] = struct{}{}
	if store.item.State == domainvideo.ItemStateProviderQueued || store.item.State == domainvideo.ItemStateProviderRunning {
		store.item.State = domainvideo.ItemStateArtifactPending
		store.item.Version++
	}
	return false, nil
}
func (store *raceStore) ClaimDue(_ context.Context, request workervideo.ClaimRequest) (workervideo.WorkItem, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.item.LeaseOwner != "" || (store.item.State != domainvideo.ItemStateProviderQueued && store.item.State != domainvideo.ItemStateProviderRunning) {
		return workervideo.WorkItem{}, false, nil
	}
	store.item.LeaseOwner = request.Owner
	return store.item, true, nil
}
func (*raceStore) PrepareAttempt(context.Context, workervideo.PrepareAttemptRequest) (workervideo.WorkItem, error) {
	panic("unexpected PrepareAttempt")
}
func (store *raceStore) ApplyStep(_ context.Context, request workervideo.ApplyStepRequest) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.item.Version != request.ExpectedVersion || store.item.LeaseOwner != request.Owner {
		store.pollConflicts++
		return false, nil
	}
	if store.item.State == domainvideo.ItemStateArtifactPending && request.Target != domainvideo.ItemStateArtifactPending {
		store.regressions++
		return false, nil
	}
	store.item.State = request.Target
	store.item.Version++
	store.item.NextActionAt = request.NextActionAt
	store.item.LeaseOwner = ""
	return true, nil
}
func (*raceStore) CommitArtifact(context.Context, workervideo.ArtifactCommitRequest) (bool, error) {
	panic("unexpected CommitArtifact")
}
func (*raceStore) LoadSettlement(context.Context, string) (workervideo.SettlementSnapshot, error) {
	panic("unexpected LoadSettlement")
}
func (*raceStore) FinalizeTask(context.Context, workervideo.FinalizeRequest) (bool, error) {
	panic("unexpected FinalizeTask")
}
func (store *raceStore) ReleaseLease(_ context.Context, ref workervideo.LeaseRef) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.item.LeaseOwner == ref.Owner {
		store.item.LeaseOwner = ""
	}
	return nil
}
