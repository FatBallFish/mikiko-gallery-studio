package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/lib/pq"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestProbeResultJSONContract(t *testing.T) {
	result := ProbeResult{
		Kind: "database", Success: true, Code: ProbeCodeOK,
		Message: "PostgreSQL connection and schema privileges verified.", LatencyMS: 12, Version: "16.4",
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal ProbeResult: %v", err)
	}
	want := `{"kind":"database","success":true,"code":"OK","message":"PostgreSQL connection and schema privileges verified.","latency_ms":12,"version":"16.4"}`
	if string(encoded) != want {
		t.Fatalf("ProbeResult JSON = %s", encoded)
	}
}

func TestProbeValidationRejectsDraftsBeforeDial(t *testing.T) {
	var calls atomic.Int32
	service := newProbeService(probeDependencies{
		postgres: func(context.Context, string) (string, error) { calls.Add(1); return "", nil },
		redis:    func(context.Context, RedisProbeRequest) (string, error) { calls.Add(1); return "", nil },
		storage:  func(context.Context, config.StorageConfig) (string, error) { calls.Add(1); return "", nil },
	})

	cases := []struct {
		name   string
		result ProbeResult
	}{
		{name: "database scheme", result: service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: "mysql://user:secret@db/app"})},
		{name: "database target", result: service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: "postgres://user:secret@db"})},
		{name: "redis scheme", result: service.ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: "http://user:secret@redis", KeyPrefix: "app"})},
		{name: "redis prefix", result: service.ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: "redis://redis:6379/0", KeyPrefix: "../shared"})},
		{name: "local root", result: service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "local"}})},
		{name: "s3 credentials", result: service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{Endpoint: "http://minio:9000", Region: "us-east-1", Bucket: "assets"}}})},
		{name: "s3 endpoint path", result: service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{Endpoint: "http://minio:9000/ignored", Region: "us-east-1", Bucket: "assets", AccessKeyID: "access", SecretAccessKey: "secret"}}})},
		{name: "s3 prefix traversal", result: service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "s3", S3: config.StorageS3Config{Endpoint: "http://minio:9000", Region: "us-east-1", Bucket: "assets", AccessKeyID: "access", SecretAccessKey: "secret", Prefix: "safe/../other"}}})},
	}
	for _, testCase := range cases {
		if testCase.result.Success || testCase.result.Code != ProbeCodeInvalidConfiguration {
			t.Errorf("%s result = %#v", testCase.name, testCase.result)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid drafts reached a connector %d times", calls.Load())
	}
}

func TestProbeTimeoutCancellationAndErrorsAreSanitized(t *testing.T) {
	secret := "postgres://operator:super-secret@db.internal/app"
	blocked := newProbeService(probeDependencies{
		timeout: 20 * time.Millisecond,
		postgres: func(ctx context.Context, _ string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	result := blocked.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: secret})
	if result.Code != ProbeCodeTimeout || result.Success {
		t.Fatalf("timeout result = %#v", result)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result = blocked.ProbePostgres(ctx, PostgresProbeRequest{DatabaseURL: secret})
	if result.Code != ProbeCodeCancelled || result.Success {
		t.Fatalf("cancelled result = %#v", result)
	}

	ignoresCancellation := newProbeService(probeDependencies{
		timeout: 20 * time.Millisecond,
		postgres: func(ctx context.Context, _ string) (string, error) {
			<-ctx.Done()
			return "16.4", nil
		},
	})
	result = ignoresCancellation.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: secret})
	if result.Code != ProbeCodeTimeout || result.Success {
		t.Fatalf("runner that ignored cancellation returned %#v", result)
	}

	failing := newProbeService(probeDependencies{
		postgres: func(context.Context, string) (string, error) {
			return "", fmt.Errorf("dial failed for %s", secret)
		},
	})
	result = failing.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: secret})
	encoded, _ := json.Marshal(result)
	if result.Code != ProbeCodeConnectionFailed || strings.Contains(string(encoded), "super-secret") || strings.Contains(result.Message, secret) {
		t.Fatalf("failure was unstable or leaked credentials: result=%#v json=%s", result, encoded)
	}
}

func TestProbePostgresClassifiesAuthenticationAndSchemaPrivileges(t *testing.T) {
	cases := []struct {
		code string
		err  error
	}{
		{code: ProbeCodeAuthenticationFailed, err: &pq.Error{Code: "28P01", Message: "password authentication failed for secret"}},
		{code: ProbeCodeInsufficientPrivileges, err: &pq.Error{Code: "42501", Message: "permission denied for schema private"}},
		{code: ProbeCodeReadWriteCheckFailed, err: probeFailureError(ProbeCodeReadWriteCheckFailed, errors.New("unexpected query failure containing a-password"))},
	}
	for _, testCase := range cases {
		service := newProbeService(probeDependencies{
			postgres: func(context.Context, string) (string, error) { return "", testCase.err },
		})
		result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: "postgres://user:a-password@db/app"})
		if result.Code != testCase.code || strings.Contains(result.Message, "a-password") || strings.Contains(result.Message, "private") {
			t.Errorf("error %T classified as %#v", testCase.err, result)
		}
	}
}

func TestPostgresProbeRejectsServerSuperuserBeforeDDL(t *testing.T) {
	database := &recordingPostgresProbeDatabase{
		version: "16.4", superuser: true,
		transaction: &recordingPostgresProbeTransaction{value: "setup-probe"},
	}
	_, err := runPostgresProbeWithDatabase(t.Context(), database, strings.NewReader(strings.Repeat("z", 12)))
	if code := classifyProbeError("database", err); code != ProbeCodeUnsafePrivileges {
		t.Fatalf("superuser result code=%s err=%v", code, err)
	}
	if database.superuserCalls != 1 || database.versionCalls != 0 || database.beginCalls != 0 {
		t.Fatalf("superuser reached version/DDL checks: %#v", database)
	}
	result := failedProbeResult("database", ProbeCodeUnsafePrivileges, time.Now())
	if strings.Contains(strings.ToLower(result.Message), "secret") || result.Message == probeMessage("database", ProbeCodeInsufficientPrivileges) {
		t.Fatalf("unsafe privilege message is not explicit and sanitized: %#v", result)
	}

	downscoped := &recordingPostgresProbeDatabase{
		version: "16.4", sessionSuperuser: true,
		transaction: &recordingPostgresProbeTransaction{value: "setup-probe"},
	}
	_, err = runPostgresProbeWithDatabase(t.Context(), downscoped, strings.NewReader(strings.Repeat("y", 12)))
	if code := classifyProbeError("database", err); code != ProbeCodeUnsafePrivileges {
		t.Fatalf("down-scoped superuser result code=%s err=%v", code, err)
	}
	if downscoped.superuserCalls != 1 || downscoped.versionCalls != 0 || downscoped.beginCalls != 0 {
		t.Fatalf("down-scoped superuser reached version/DDL checks: %#v", downscoped)
	}
}

func TestPostgresProbeExecutesSchemaReadWriteCheckAndAlwaysRollsBack(t *testing.T) {
	transaction := &recordingPostgresProbeTransaction{value: "setup-probe"}
	database := &recordingPostgresProbeDatabase{version: "16.4", transaction: transaction}
	version, err := runPostgresProbeWithDatabase(t.Context(), database, strings.NewReader(strings.Repeat("a", 12)))
	if err != nil || version != "16.4" {
		t.Fatalf("runPostgresProbeWithDatabase version=%q err=%v", version, err)
	}
	if database.pingCalls != 1 || database.superuserCalls != 1 || database.versionCalls != 1 || database.beginCalls != 1 {
		t.Fatalf("database calls: %#v", database)
	}
	if len(transaction.statements) != 3 ||
		!strings.HasPrefix(transaction.statements[0], `CREATE TABLE "setup_probe_`) ||
		!strings.HasPrefix(transaction.statements[1], `CREATE UNIQUE INDEX "setup_probe_`) ||
		!strings.HasPrefix(transaction.statements[2], `INSERT INTO "setup_probe_`) ||
		!strings.HasPrefix(transaction.query, `SELECT probe_value FROM "setup_probe_`) {
		t.Fatalf("unexpected PostgreSQL probe statements: statements=%q query=%q", transaction.statements, transaction.query)
	}
	if transaction.rollbackCalls != 1 {
		t.Fatalf("transaction rollback calls = %d", transaction.rollbackCalls)
	}
}

func TestProbeRunEnforcesHardDeadlineForContextIgnoringRunner(t *testing.T) {
	finished := make(chan struct{})
	service := newProbeService(probeDependencies{
		timeout: 20 * time.Millisecond,
		postgres: func(context.Context, string) (string, error) {
			time.Sleep(150 * time.Millisecond)
			close(finished)
			return "16.4", nil
		},
	})
	started := time.Now()
	result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: "postgres://user:password@db/app"})
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("hard deadline response took %s", elapsed)
	}
	if result.Code != ProbeCodeTimeout || result.Success {
		t.Fatalf("context-ignoring runner result = %#v", result)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("late runner did not finish and release its slot")
	}
}

func TestProbeRunDeadlineAndCancellationWinOverConcurrentRunnerError(t *testing.T) {
	const attempts = 100
	for range attempts {
		service := newProbeService(probeDependencies{
			timeout: time.Millisecond,
			postgres: func(ctx context.Context, _ string) (string, error) {
				<-ctx.Done()
				return "", errors.New("provider error after deadline")
			},
		})
		result := service.ProbePostgres(context.Background(), PostgresProbeRequest{DatabaseURL: "postgres://user:password@db/app"})
		if result.Code != ProbeCodeTimeout {
			t.Fatalf("deadline attempt returned %#v", result)
		}
	}

	for range attempts {
		started := make(chan struct{})
		service := newProbeService(probeDependencies{
			timeout: time.Second,
			postgres: func(ctx context.Context, _ string) (string, error) {
				close(started)
				<-ctx.Done()
				return "", errors.New("provider error after cancellation")
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		resultChannel := make(chan ProbeResult, 1)
		go func() {
			resultChannel <- service.ProbePostgres(ctx, PostgresProbeRequest{DatabaseURL: "postgres://user:password@db/app"})
		}()
		<-started
		cancel()
		if result := <-resultChannel; result.Code != ProbeCodeCancelled {
			t.Fatalf("cancellation attempt returned %#v", result)
		}
	}
}

func TestProbeRunDoesNotStartCancelledOrUnboundedOperations(t *testing.T) {
	var cancelledStarts atomic.Int32
	service := newProbeService(probeDependencies{
		timeout:       25 * time.Millisecond,
		maxConcurrent: 2,
		postgres: func(context.Context, string) (string, error) {
			cancelledStarts.Add(1)
			return "16", nil
		},
	})
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	result := service.ProbePostgres(cancelledCtx, PostgresProbeRequest{DatabaseURL: "postgres://user:password@db/app"})
	if result.Code != ProbeCodeCancelled || cancelledStarts.Load() != 0 {
		t.Fatalf("cancelled request result=%#v starts=%d", result, cancelledStarts.Load())
	}

	release := make(chan struct{})
	var starts atomic.Int32
	bounded := newProbeService(probeDependencies{
		timeout:       30 * time.Millisecond,
		maxConcurrent: 2,
		postgres: func(context.Context, string) (string, error) {
			starts.Add(1)
			<-release
			return "16", nil
		},
	})
	const callers = 24
	results := make(chan ProbeResult, callers)
	for range callers {
		go func() {
			results <- bounded.ProbePostgres(context.Background(), PostgresProbeRequest{DatabaseURL: "postgres://user:password@db/app"})
		}()
	}
	for range callers {
		result := <-results
		if result.Code != ProbeCodeTimeout {
			t.Errorf("bounded runner result = %#v", result)
		}
	}
	if starts.Load() != 2 {
		t.Fatalf("bounded pool started %d operations, want 2", starts.Load())
	}
	close(release)
}

func TestProbeRunRecoversPanicsWithoutLeakingDetails(t *testing.T) {
	service := newProbeService(probeDependencies{
		postgres: func(context.Context, string) (string, error) {
			panic("panic includes submitted-super-secret")
		},
	})
	result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: "postgres://user:submitted-super-secret@db/app"})
	encoded, _ := json.Marshal(result)
	if result.Code != ProbeCodeInternalError || result.Success || strings.Contains(string(encoded), "submitted-super-secret") {
		t.Fatalf("panic result leaked or was misclassified: %#v", result)
	}
}

func TestPostgresProbeMapsPrivilegeAndCleanupFailures(t *testing.T) {
	privilegeTx := &recordingPostgresProbeTransaction{execErr: &pq.Error{Code: "42501", Message: "private schema secret"}}
	_, err := runPostgresProbeWithDatabase(t.Context(), &recordingPostgresProbeDatabase{version: "16", transaction: privilegeTx}, strings.NewReader(strings.Repeat("b", 12)))
	if code := classifyProbeError("database", err); code != ProbeCodeInsufficientPrivileges || privilegeTx.rollbackCalls != 1 {
		t.Fatalf("privilege result code=%s rollback=%d err=%v", code, privilegeTx.rollbackCalls, err)
	}

	cleanupTx := &recordingPostgresProbeTransaction{value: "setup-probe", rollbackErr: errors.New("rollback failed with db-secret")}
	_, err = runPostgresProbeWithDatabase(t.Context(), &recordingPostgresProbeDatabase{version: "16", transaction: cleanupTx}, strings.NewReader(strings.Repeat("c", 12)))
	if code := classifyProbeError("database", err); code != ProbeCodeCleanupFailed {
		t.Fatalf("cleanup result code=%s err=%v", code, err)
	}

	combinedTx := &recordingPostgresProbeTransaction{
		execErr:     &pq.Error{Code: "42501", Message: "permission denied"},
		rollbackErr: errors.New("rollback also failed"),
	}
	_, err = runPostgresProbeWithDatabase(t.Context(), &recordingPostgresProbeDatabase{version: "16", transaction: combinedTx}, strings.NewReader(strings.Repeat("d", 12)))
	if code := classifyProbeError("database", err); code != ProbeCodeCleanupFailed {
		t.Fatalf("combined failure code=%s err=%v", code, err)
	}
}

func TestProbeRedisCleansUniqueKeyAndAllowsUnavailableOptionalVersion(t *testing.T) {
	server := miniredis.RunT(t)
	server.Set("app:setup-probe:existing", "keep")
	service := NewProbeService()
	result := service.ProbeRedis(t.Context(), RedisProbeRequest{
		RedisURL: "redis://" + server.Addr() + "/0", KeyPrefix: "app",
	})
	if !result.Success || result.Code != ProbeCodeOK || result.Version != "" {
		t.Fatalf("Redis probe result = %#v", result)
	}
	if value, err := server.Get("app:setup-probe:existing"); err != nil || value != "keep" {
		t.Fatalf("existing Redis key changed: value=%q err=%v", value, err)
	}
	keys := server.Keys()
	if len(keys) != 1 || keys[0] != "app:setup-probe:existing" {
		t.Fatalf("probe keys were not cleaned: %v", keys)
	}
}

func TestProbeAllowsSuccessfulRunnerWithoutOptionalVersion(t *testing.T) {
	service := newProbeService(probeDependencies{
		redis: func(context.Context, RedisProbeRequest) (string, error) { return "", nil },
	})
	result := service.ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: "redis://localhost:6379/0", KeyPrefix: "app"})
	if !result.Success || result.Code != ProbeCodeOK || result.Version != "" {
		t.Fatalf("versionless Redis result = %#v", result)
	}
}

func TestProbeRedisClassifiesAuthenticationFailureWithoutPassword(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireAuth("correct-password")
	result := NewProbeService().ProbeRedis(t.Context(), RedisProbeRequest{
		RedisURL: "redis://:wrong-password@" + server.Addr() + "/0", KeyPrefix: "app",
	})
	encoded, _ := json.Marshal(result)
	if result.Code != ProbeCodeAuthenticationFailed || strings.Contains(string(encoded), "wrong-password") || strings.Contains(string(encoded), "correct-password") {
		t.Fatalf("Redis auth result leaked or was misclassified: %#v", result)
	}
}

func TestProbeLocalStorageRoundTripCleanupAndSymlinkSafety(t *testing.T) {
	root := t.TempDir()
	service := NewProbeService()
	result := service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "local", LocalRoot: root}})
	if !result.Success || result.Code != ProbeCodeOK || result.Version != "local" {
		t.Fatalf("local storage result = %#v", result)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("local storage probe left data: entries=%v err=%v", entries, err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	symlink := filepath.Join(t.TempDir(), "storage-link")
	if err := os.Symlink(root, symlink); err != nil {
		t.Fatalf("create symlink fixture: %v", err)
	}
	result = service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "local", LocalRoot: symlink}})
	if result.Code != ProbeCodeInvalidConfiguration {
		t.Fatalf("symlink root result = %#v", result)
	}
}

func TestProbeLocalStorageRejectsTraversalAndResolvesAncestorSymlink(t *testing.T) {
	var calls atomic.Int32
	service := newProbeService(probeDependencies{
		storage: func(_ context.Context, _ config.StorageConfig) (string, error) {
			calls.Add(1)
			return "local", nil
		},
	})
	traversal := filepath.Join(t.TempDir(), "child") + string(filepath.Separator) + ".." + string(filepath.Separator) + "storage"
	result := service.ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{Driver: "local", LocalRoot: traversal}})
	if result.Code != ProbeCodeInvalidConfiguration || calls.Load() != 0 {
		t.Fatalf("traversal root result=%#v calls=%d", result, calls.Load())
	}
	if runtime.GOOS == "windows" {
		return
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "parent-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create ancestor symlink fixture: %v", err)
	}
	requested := filepath.Join(link, "new-child")
	request := StorageProbeRequest{Config: config.StorageConfig{Driver: "local", LocalRoot: requested}}
	normalized, err := normalizeStorageProbeConfig(t.Context(), request.Config)
	if err != nil {
		t.Fatalf("normalize ancestor symlink target: %v", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target fixture: %v", err)
	}
	wantRoot := filepath.Join(resolvedTarget, "new-child")
	if normalized.LocalRoot != wantRoot {
		t.Fatalf("normalized root=%q want resolved %q", normalized.LocalRoot, wantRoot)
	}
	if request.Config.LocalRoot != requested {
		t.Fatalf("point-in-time path resolution mutated draft: got=%q want=%q", request.Config.LocalRoot, requested)
	}
}

func TestLocalProbeParentTraversalClassificationIsPortable(t *testing.T) {
	unsafePaths := []string{
		"../probe", "child/../probe", `child\..\probe`,
		`C:..\probe`, "C:../probe", `C:\safe\..\probe`,
		`\\server\share\..\probe`,
	}
	for _, candidate := range unsafePaths {
		if !localProbePathHasParentTraversal(candidate) {
			t.Errorf("parent traversal was not detected in %q", candidate)
		}
	}
	for _, candidate := range []string{"./probe", "data/storage", "/var/lib/storage", `C:\storage`} {
		if localProbePathHasParentTraversal(candidate) {
			t.Errorf("safe path was rejected as traversal: %q", candidate)
		}
	}
}

func TestProbeStorageUsesBackendAndCleansAfterPartialFailure(t *testing.T) {
	backend := &recordingProbeBackend{
		putErr:    errors.New("partial write containing secret-access-key"),
		deleteErr: errors.New("cleanup failed containing secret-access-key"),
	}
	_, err := runStorageProbeWithFactory(t.Context(), config.StorageConfig{
		Driver: "s3",
		S3:     config.StorageS3Config{Endpoint: "http://s3.internal", Region: "us-east-1", Bucket: "assets", AccessKeyID: "access", SecretAccessKey: "secret-access-key"},
	}, strings.NewReader(strings.Repeat("a", 64)), func(config.StorageConfig) (storage.Backend, error) {
		return backend, nil
	})
	if err == nil || classifyProbeError("storage", err) != ProbeCodeCleanupFailed || backend.deleteCalls != 1 {
		t.Fatalf("partial failure cleanup: err=%v delete_calls=%d", err, backend.deleteCalls)
	}
}

func TestProbeStorageRejectsOversizedReadAndCleansObject(t *testing.T) {
	backend := &recordingProbeBackend{boundedGetErr: storage.ErrObjectTooLarge}
	_, err := runStorageProbeWithFactory(t.Context(), config.StorageConfig{
		Driver: "s3",
		S3:     config.StorageS3Config{Endpoint: "http://s3.internal", Region: "us-east-1", Bucket: "assets", AccessKeyID: "access", SecretAccessKey: "secret"},
	}, strings.NewReader(strings.Repeat("q", 32)), func(config.StorageConfig) (storage.Backend, error) {
		return backend, nil
	})
	if code := classifyProbeError("storage", err); code != ProbeCodeReadWriteCheckFailed {
		t.Fatalf("oversized storage probe code=%s err=%v", code, err)
	}
	if backend.boundedGetMax != 16 || backend.getCalls != 0 || backend.deleteCalls != 1 {
		t.Fatalf("oversized storage probe backend=%#v", backend)
	}
}

func TestProbeStorageRequiresBoundedReadCapability(t *testing.T) {
	backend := &unboundedProbeBackend{}
	_, err := runStorageProbeWithFactory(t.Context(), config.StorageConfig{
		Driver: "s3",
		S3:     config.StorageS3Config{Endpoint: "http://s3.internal", Region: "us-east-1", Bucket: "assets", AccessKeyID: "access", SecretAccessKey: "secret"},
	}, strings.NewReader(strings.Repeat("u", 32)), func(config.StorageConfig) (storage.Backend, error) {
		return backend, nil
	})
	if code := classifyProbeError("storage", err); code != ProbeCodeInternalError {
		t.Fatalf("unbounded storage backend code=%s err=%v", code, err)
	}
	if backend.getCalls != 0 || backend.deleteCalls != 1 {
		t.Fatalf("unbounded storage backend=%#v", backend)
	}
}

func TestProbeStorageReturnsByDeadlineAndCleansAfterLatePut(t *testing.T) {
	backend := &latePutProbeBackend{
		putStarted: make(chan struct{}), putRelease: make(chan struct{}), deleted: make(chan struct{}),
	}
	service := newProbeService(probeDependencies{
		timeout:       25 * time.Millisecond,
		maxConcurrent: 1,
		storage: func(ctx context.Context, storageConfig config.StorageConfig) (string, error) {
			return runStorageProbeWithFactory(ctx, storageConfig, strings.NewReader(strings.Repeat("e", 32)), func(config.StorageConfig) (storage.Backend, error) {
				return backend, nil
			})
		},
	})
	root := t.TempDir()
	resultChannel := make(chan ProbeResult, 1)
	go func() {
		resultChannel <- service.ProbeStorage(context.Background(), StorageProbeRequest{Config: config.StorageConfig{Driver: "local", LocalRoot: root}})
	}()
	select {
	case <-backend.putStarted:
	case <-time.After(time.Second):
		t.Fatal("storage probe did not start Put")
	}
	returnedBeforeRelease := false
	var result ProbeResult
	select {
	case result = <-resultChannel:
		returnedBeforeRelease = true
	case <-time.After(100 * time.Millisecond):
	}
	close(backend.putRelease)
	if !returnedBeforeRelease {
		result = <-resultChannel
		t.Error("storage probe did not honor hard response deadline")
	}
	if result.Code != ProbeCodeTimeout {
		t.Errorf("late Put result = %#v", result)
	}
	select {
	case <-backend.deleted:
	case <-time.After(time.Second):
		t.Fatal("late Put object was not cleaned")
	}
}

func TestProbeS3CompatibleStorageRoundTripAndCleanup(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		methods = append(methods, request.Method)
		if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		switch request.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(request.Body)
			objects[request.URL.Path] = body
			writer.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, exists := objects[request.URL.Path]
			if !exists {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(body)
		case http.MethodDelete:
			delete(objects, request.URL.Path)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	result := NewProbeService().ProbeStorage(t.Context(), StorageProbeRequest{Config: config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: server.URL, Region: "us-east-1", Bucket: "assets",
			AccessKeyID: "access", SecretAccessKey: "secret", ForcePathStyle: true, Prefix: "installation",
		},
	}})
	if !result.Success || result.Code != ProbeCodeOK || result.Version != "s3-compatible" {
		t.Fatalf("S3 probe result = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(objects) != 0 || strings.Join(methods, ",") != "PUT,GET,DELETE" {
		t.Fatalf("S3 cleanup methods=%v remaining=%v", methods, objects)
	}
}

func TestProbeServiceIsConcurrentAndDoesNotMutateDrafts(t *testing.T) {
	draft := RedisProbeRequest{RedisURL: "redis://localhost:6379/0", KeyPrefix: "installation"}
	service := newProbeService(probeDependencies{
		redis: func(context.Context, RedisProbeRequest) (string, error) { return "7.4", nil },
	})
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result := service.ProbeRedis(context.Background(), draft)
			if !result.Success {
				t.Errorf("concurrent result = %#v", result)
			}
		}()
	}
	wait.Wait()
	if draft.RedisURL != "redis://localhost:6379/0" || draft.KeyPrefix != "installation" {
		t.Fatalf("probe mutated draft: %#v", draft)
	}
}

type recordingProbeBackend struct {
	putErr        error
	boundedGetErr error
	boundedGetMax int64
	getCalls      int
	deleteErr     error
	deleteCalls   int
}

type unboundedProbeBackend struct {
	getCalls    int
	deleteCalls int
}

type recordingPostgresProbeDatabase struct {
	pingCalls        int
	superuserCalls   int
	versionCalls     int
	beginCalls       int
	superuser        bool
	sessionSuperuser bool
	version          string
	transaction      postgresProbeTransaction
}

func (database *recordingPostgresProbeDatabase) Ping(context.Context) error {
	database.pingCalls++
	return nil
}
func (database *recordingPostgresProbeDatabase) IsSuperuser(context.Context) (bool, error) {
	database.superuserCalls++
	return database.superuser || database.sessionSuperuser, nil
}
func (database *recordingPostgresProbeDatabase) ServerVersion(context.Context) (string, error) {
	database.versionCalls++
	return database.version, nil
}
func (database *recordingPostgresProbeDatabase) Begin(context.Context) (postgresProbeTransaction, error) {
	database.beginCalls++
	return database.transaction, nil
}

type recordingPostgresProbeTransaction struct {
	statements    []string
	query         string
	value         string
	execErr       error
	queryErr      error
	rollbackErr   error
	rollbackCalls int
}

func (transaction *recordingPostgresProbeTransaction) Exec(_ context.Context, statement string) error {
	transaction.statements = append(transaction.statements, statement)
	return transaction.execErr
}
func (transaction *recordingPostgresProbeTransaction) QueryValue(_ context.Context, query string) (string, error) {
	transaction.query = query
	return transaction.value, transaction.queryErr
}
func (transaction *recordingPostgresProbeTransaction) Rollback() error {
	transaction.rollbackCalls++
	return transaction.rollbackErr
}

type latePutProbeBackend struct {
	putStarted chan struct{}
	putRelease chan struct{}
	deleted    chan struct{}
	deleteOnce sync.Once
}

func (*latePutProbeBackend) Driver() string { return "local" }
func (backend *latePutProbeBackend) Put(context.Context, string, string, []byte) error {
	close(backend.putStarted)
	<-backend.putRelease
	return nil
}
func (*latePutProbeBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("Get must not run after late Put")
}
func (*latePutProbeBackend) GetBounded(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("GetBounded must not run after late Put")
}
func (backend *latePutProbeBackend) Delete(context.Context, string) error {
	backend.deleteOnce.Do(func() { close(backend.deleted) })
	return nil
}

func (backend *recordingProbeBackend) Driver() string { return "s3" }
func (backend *recordingProbeBackend) Put(context.Context, string, string, []byte) error {
	return backend.putErr
}
func (backend *recordingProbeBackend) Get(context.Context, string) ([]byte, error) {
	backend.getCalls++
	return nil, errors.New("unexpected get")
}
func (backend *recordingProbeBackend) GetBounded(_ context.Context, _ string, maxBytes int64) ([]byte, error) {
	backend.boundedGetMax = maxBytes
	return nil, backend.boundedGetErr
}
func (backend *recordingProbeBackend) Delete(context.Context, string) error {
	backend.deleteCalls++
	return backend.deleteErr
}

func (*unboundedProbeBackend) Driver() string { return "test-unbounded" }
func (*unboundedProbeBackend) Put(context.Context, string, string, []byte) error {
	return nil
}
func (backend *unboundedProbeBackend) Get(context.Context, string) ([]byte, error) {
	backend.getCalls++
	return []byte("must not be called"), nil
}
func (backend *unboundedProbeBackend) Delete(context.Context, string) error {
	backend.deleteCalls++
	return nil
}
