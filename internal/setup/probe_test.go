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

func TestPostgresProbeExecutesSchemaReadWriteCheckAndAlwaysRollsBack(t *testing.T) {
	transaction := &recordingPostgresProbeTransaction{value: "setup-probe"}
	database := &recordingPostgresProbeDatabase{version: "16.4", transaction: transaction}
	version, err := runPostgresProbeWithDatabase(t.Context(), database, strings.NewReader(strings.Repeat("a", 12)))
	if err != nil || version != "16.4" {
		t.Fatalf("runPostgresProbeWithDatabase version=%q err=%v", version, err)
	}
	if database.pingCalls != 1 || database.versionCalls != 1 || database.beginCalls != 1 {
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
	var receivedRoot string
	service := newProbeService(probeDependencies{
		storage: func(_ context.Context, storageConfig config.StorageConfig) (string, error) {
			calls.Add(1)
			receivedRoot = storageConfig.LocalRoot
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
	result = service.ProbeStorage(t.Context(), request)
	if !result.Success || calls.Load() != 1 {
		t.Fatalf("ancestor symlink result=%#v calls=%d", result, calls.Load())
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target fixture: %v", err)
	}
	wantRoot := filepath.Join(resolvedTarget, "new-child")
	if receivedRoot != wantRoot {
		t.Fatalf("storage runner root=%q want resolved %q", receivedRoot, wantRoot)
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
	putErr      error
	deleteErr   error
	deleteCalls int
}

type recordingPostgresProbeDatabase struct {
	pingCalls    int
	versionCalls int
	beginCalls   int
	version      string
	transaction  postgresProbeTransaction
}

func (database *recordingPostgresProbeDatabase) Ping(context.Context) error {
	database.pingCalls++
	return nil
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

func (backend *recordingProbeBackend) Driver() string { return "s3" }
func (backend *recordingProbeBackend) Put(context.Context, string, string, []byte) error {
	return backend.putErr
}
func (backend *recordingProbeBackend) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected get")
}
func (backend *recordingProbeBackend) Delete(context.Context, string) error {
	backend.deleteCalls++
	return backend.deleteErr
}
