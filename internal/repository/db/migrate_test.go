package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestValidateMigrationRequestRejectsUnsafeInputs(t *testing.T) {
	valid := MigrationRequest{
		InstallationID: "installation-test",
		AppVersion:     "v1.2.3",
		ConfigVersion:  1,
	}
	tests := []struct {
		name        string
		databaseURL string
		request     MigrationRequest
	}{
		{name: "sqlite", databaseURL: "file:test.db", request: valid},
		{name: "missing host", databaseURL: "postgres:///app", request: valid},
		{name: "missing database", databaseURL: "postgres://app:secret@localhost", request: valid},
		{name: "missing installation", databaseURL: "postgres://app:secret@localhost/app", request: MigrationRequest{AppVersion: "v1", ConfigVersion: 1}},
		{name: "invalid installation", databaseURL: "postgres://app:secret@localhost/app", request: MigrationRequest{InstallationID: "contains spaces", AppVersion: "v1", ConfigVersion: 1}},
		{name: "missing app version", databaseURL: "postgres://app:secret@localhost/app", request: MigrationRequest{InstallationID: "installation-test", ConfigVersion: 1}},
		{name: "invalid config version", databaseURL: "postgres://app:secret@localhost/app", request: MigrationRequest{InstallationID: "installation-test", AppVersion: "v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMigrationRequest(tt.databaseURL, tt.request)
			if err == nil {
				t.Fatal("expected request validation to fail")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("validation error leaked database credentials: %v", err)
			}
		})
	}
	if err := validateMigrationRequest("postgres://app:secret@localhost/app?sslmode=disable", valid); err != nil {
		t.Fatalf("valid migration request rejected: %v", err)
	}
}

func TestValidateSchemaVersionRejectsInvalidDatabaseVersion(t *testing.T) {
	err := validateSchemaVersion(SchemaVersion{
		InstallationID: "installation-test",
		AppVersion:     "v1",
		ConfigVersion:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "database schema version") {
		t.Fatalf("validateSchemaVersion error = %v, want database schema version diagnostic", err)
	}
}

func TestPostgresAdvisoryLockerUsesFixedSessionLockKey(t *testing.T) {
	session := &recordingSQLSession{}
	locker := newPostgresAdvisoryLocker(session)
	if err := locker.Lock(context.Background()); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := locker.Unlock(context.Background()); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	if len(session.calls) != 2 {
		t.Fatalf("session calls = %d, want 2", len(session.calls))
	}
	if session.calls[0].query != acquireMigrationLockSQL || session.calls[1].query != releaseMigrationLockSQL {
		t.Fatalf("unexpected advisory lock SQL sequence: %#v", session.calls)
	}
	for _, call := range session.calls {
		if len(call.args) != 1 || call.args[0] != migrationAdvisoryLockKey {
			t.Fatalf("advisory lock must use fixed key %d: %#v", migrationAdvisoryLockKey, call.args)
		}
	}
}

func TestMigrationLifecycleHoldsLockAcrossEveryMutation(t *testing.T) {
	locker := &statefulMigrationLocker{}
	steps := []string{}
	err := runMigrationLifecycle(context.Background(), locker, func(ctx context.Context) error {
		if !locker.locked.Load() {
			t.Fatal("migration mutation ran without lock")
		}
		steps = append(steps, "legacy", "schema", "backfill", "installation")
		return nil
	})
	if err != nil {
		t.Fatalf("runMigrationLifecycle: %v", err)
	}
	if locker.locked.Load() {
		t.Fatal("migration lock remained held after lifecycle")
	}
	if got := strings.Join(steps, ","); got != "legacy,schema,backfill,installation" {
		t.Fatalf("migration steps = %q", got)
	}
	if got := strings.Join(locker.events, ","); got != "lock,unlock" {
		t.Fatalf("lock events = %q", got)
	}
}

func TestMigrationLifecycleJoinsMutationAndUnlockErrors(t *testing.T) {
	mutationErr := errors.New("mutation failed")
	unlockErr := errors.New("unlock failed")
	locker := &statefulMigrationLocker{unlockErr: unlockErr}
	err := runMigrationLifecycle(context.Background(), locker, func(context.Context) error {
		return mutationErr
	})
	if !errors.Is(err, mutationErr) || !errors.Is(err, unlockErr) {
		t.Fatalf("migration lifecycle error = %v, want joined mutation and unlock errors", err)
	}
}

func TestConcurrentMigrationLifecyclesSerialize(t *testing.T) {
	locker := &mutexMigrationLocker{}
	start := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := runMigrationLifecycle(context.Background(), locker, func(context.Context) error {
				current := active.Add(1)
				for {
					maximum := maxActive.Load()
					if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				active.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("runMigrationLifecycle: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("maximum concurrent migration mutations = %d, want 1", got)
	}
}

func TestCheckSchemaCompatibilityIsReadOnlyAndTyped(t *testing.T) {
	client := openMigrationSQLite(t, "compatibility")
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	expected := SchemaVersion{
		InstallationID:        "installation-a",
		AppVersion:            "v1.2.3",
		ConfigVersion:         1,
		DatabaseSchemaVersion: CurrentDatabaseSchemaVersion,
	}

	err := CheckSchemaCompatibility(ctx, client, expected)
	assertCompatibilityErrorKind(t, err, CompatibilityMissing)

	if _, err := client.Installation.Create().
		SetSingletonKey("installation").
		SetInstallationID(expected.InstallationID).
		SetConfigSchemaVersion(expected.ConfigVersion).
		SetDatabaseSchemaVersion(expected.DatabaseSchemaVersion).
		SetAppVersion(expected.AppVersion).
		SetInitializedAt(time.Now().UTC()).
		SetMigratedAt(time.Now().UTC()).
		Save(ctx); err != nil {
		t.Fatalf("create installation: %v", err)
	}
	if err := CheckSchemaCompatibility(ctx, client, expected); err != nil {
		t.Fatalf("compatible schema rejected: %v", err)
	}

	mismatches := []struct {
		name     string
		mutate   func(*SchemaVersion)
		wantKind CompatibilityErrorKind
	}{
		{name: "installation", mutate: func(v *SchemaVersion) { v.InstallationID = "installation-b" }, wantKind: CompatibilityInstallationMismatch},
		{name: "app", mutate: func(v *SchemaVersion) { v.AppVersion = "v2.0.0" }, wantKind: CompatibilityAppVersionMismatch},
		{name: "config", mutate: func(v *SchemaVersion) { v.ConfigVersion++ }, wantKind: CompatibilityConfigSchemaMismatch},
		{name: "database", mutate: func(v *SchemaVersion) { v.DatabaseSchemaVersion++ }, wantKind: CompatibilityDatabaseSchemaMismatch},
	}
	for _, tt := range mismatches {
		t.Run(tt.name, func(t *testing.T) {
			want := expected
			tt.mutate(&want)
			assertCompatibilityErrorKind(t, CheckSchemaCompatibility(ctx, client, want), tt.wantKind)
		})
	}
}

func TestValidateInstallationSnapshotsRejectsDuplicateRows(t *testing.T) {
	now := time.Now().UTC()
	rows := []installationSnapshot{
		{SingletonKey: installationSingletonKey, InstallationID: "one", ConfigVersion: 1, DatabaseVersion: 1, AppVersion: "v1", MigratedAt: now},
		{SingletonKey: installationSingletonKey, InstallationID: "two", ConfigVersion: 1, DatabaseVersion: 1, AppVersion: "v1", MigratedAt: now},
	}
	_, err := validateInstallationSnapshots(rows)
	assertCompatibilityErrorKind(t, err, CompatibilityDuplicate)
}

func TestValidateInstallationSnapshotsRejectsInvalidSingletonKey(t *testing.T) {
	_, err := validateInstallationSnapshots([]installationSnapshot{{
		SingletonKey:    "another-key",
		InstallationID:  "installation-test",
		ConfigVersion:   1,
		DatabaseVersion: 1,
		AppVersion:      "v1",
	}})
	assertCompatibilityErrorKind(t, err, CompatibilitySingletonMismatch)
}

func TestMigrateSerializesAndIsIdempotentOnPostgres(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL migration integration")
	}

	database, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	defer database.Close()
	schemaName := fmt.Sprintf("migration_%d", time.Now().UnixNano())
	if _, err := database.Exec(`CREATE SCHEMA ` + schemaName); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
	databaseURL := databaseURLWithSearchPath(t, adminURL, schemaName)
	request := MigrationRequest{InstallationID: "integration-installation", AppVersion: "integration-v1", ConfigVersion: 1}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := Migrate(context.Background(), databaseURL, request)
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent migration failed: %v", err)
		}
	}

	client, err := Open(databaseURL)
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer client.Close()
	if count, err := client.Installation.Query().Count(context.Background()); err != nil || count != 1 {
		t.Fatalf("installation count = %d, err=%v, want 1", count, err)
	}
	if _, err := database.Exec(`INSERT INTO ` + schemaName + `.installations
		(singleton_key, installation_id, config_schema_version, database_schema_version, app_version, initialized_at, migrated_at, created_at, updated_at)
		VALUES ('other', 'other-installation-row', 1, 1, 'v1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("PostgreSQL accepted a second installation row with a different singleton key")
	}
	expected := SchemaVersion{InstallationID: request.InstallationID, AppVersion: request.AppVersion, ConfigVersion: request.ConfigVersion, DatabaseSchemaVersion: CurrentDatabaseSchemaVersion}
	if err := CheckSchemaCompatibility(context.Background(), client, expected); err != nil {
		t.Fatalf("migrated schema compatibility: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO ` + schemaName + `.system_configs
		(config_category, config_key, config_value, scope, version, updated_by, updated_at)
		VALUES ('migration_test', 'wrong_identity_guard', '{"value":{"reference_generate":true,"text_to_image":true}}', 'global', 1, 0, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert wrong-identity mutation sentinel: %v", err)
	}

	_, err = Migrate(context.Background(), databaseURL, MigrationRequest{InstallationID: "other-installation", AppVersion: request.AppVersion, ConfigVersion: request.ConfigVersion})
	assertCompatibilityErrorKind(t, err, CompatibilityInstallationMismatch)
	var sentinel string
	if err := database.QueryRow(`SELECT config_value::text FROM ` + schemaName + `.system_configs WHERE config_key = 'wrong_identity_guard'`).Scan(&sentinel); err != nil {
		t.Fatalf("query wrong-identity mutation sentinel: %v", err)
	}
	if !strings.Contains(sentinel, "reference_generate") {
		t.Fatalf("wrong-installation migration mutated legacy data before rejecting identity: %s", sentinel)
	}

	upgradeRequest := MigrationRequest{InstallationID: request.InstallationID, AppVersion: "integration-v2", ConfigVersion: 2}
	if _, err := Migrate(context.Background(), databaseURL, upgradeRequest); err != nil {
		t.Fatalf("upgrade migration: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO ` + schemaName + `.system_configs
		(config_category, config_key, config_value, scope, version, updated_by, updated_at)
		VALUES ('migration_test', 'downgrade_guard', '{"value":{"reference_generate":true,"text_to_image":true}}', 'global', 1, 0, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert downgrade mutation sentinel: %v", err)
	}
	_, err = Migrate(context.Background(), databaseURL, request)
	assertCompatibilityErrorKind(t, err, CompatibilityConfigSchemaMismatch)
	if err := database.QueryRow(`SELECT config_value::text FROM ` + schemaName + `.system_configs WHERE config_key = 'downgrade_guard'`).Scan(&sentinel); err != nil {
		t.Fatalf("query downgrade mutation sentinel: %v", err)
	}
	if !strings.Contains(sentinel, "reference_generate") {
		t.Fatalf("config downgrade mutated legacy data before rejection: %s", sentinel)
	}
	if _, err := database.Exec(`UPDATE `+schemaName+`.installations SET database_schema_version = $1`, CurrentDatabaseSchemaVersion+1); err != nil {
		t.Fatalf("simulate newer database schema: %v", err)
	}
	_, err = Migrate(context.Background(), databaseURL, upgradeRequest)
	assertCompatibilityErrorKind(t, err, CompatibilityDatabaseSchemaMismatch)
	if err := database.QueryRow(`SELECT config_value::text FROM ` + schemaName + `.system_configs WHERE config_key = 'downgrade_guard'`).Scan(&sentinel); err != nil {
		t.Fatalf("query database downgrade mutation sentinel: %v", err)
	}
	if !strings.Contains(sentinel, "reference_generate") {
		t.Fatalf("database downgrade mutated legacy data before rejection: %s", sentinel)
	}
}

func TestMigrateLockWaitHonorsContextCancellationOnPostgres(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL migration integration")
	}
	database, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	defer database.Close()
	schemaName := fmt.Sprintf("migration_cancel_%d", time.Now().UnixNano())
	if _, err := database.Exec(`CREATE SCHEMA ` + schemaName); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
	databaseURL := databaseURLWithSearchPath(t, adminURL, schemaName)
	lockConnection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve blocking lock connection: %v", err)
	}
	defer lockConnection.Close()
	if _, err := lockConnection.ExecContext(context.Background(), acquireMigrationLockSQL, migrationAdvisoryLockKey); err != nil {
		t.Fatalf("acquire blocking migration lock: %v", err)
	}
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), releaseMigrationLockSQL, migrationAdvisoryLockKey)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = Migrate(ctx, databaseURL, MigrationRequest{InstallationID: "cancel-installation", AppVersion: "v1", ConfigVersion: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait error = %T %v, want context deadline", err, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled lock wait took %v", elapsed)
	}
	var tableCount int
	if err := database.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname = $1`, schemaName).Scan(&tableCount); err != nil {
		t.Fatalf("inspect canceled migration schema: %v", err)
	}
	if tableCount != 0 {
		t.Fatalf("canceled migration created %d tables before acquiring lock", tableCount)
	}
}

func openMigrationSQLite(t *testing.T, name string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:"+name+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func assertCompatibilityErrorKind(t *testing.T, err error, want CompatibilityErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected compatibility error %q", want)
	}
	var compatibilityErr *CompatibilityError
	if !errors.As(err, &compatibilityErr) {
		t.Fatalf("error %T %v is not a CompatibilityError", err, err)
	}
	if compatibilityErr.Kind != want {
		t.Fatalf("compatibility error kind = %q, want %q", compatibilityErr.Kind, want)
	}
}

func databaseURLWithSearchPath(t *testing.T, rawURL, searchPath string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", searchPath)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

type sqlCall struct {
	query string
	args  []any
}

type recordingSQLSession struct {
	calls []sqlCall
}

func (s *recordingSQLSession) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	s.calls = append(s.calls, sqlCall{query: query, args: args})
	return nil, nil
}

type statefulMigrationLocker struct {
	locked    atomic.Bool
	events    []string
	unlockErr error
}

func (l *statefulMigrationLocker) Lock(context.Context) error {
	l.events = append(l.events, "lock")
	l.locked.Store(true)
	return nil
}

func (l *statefulMigrationLocker) Unlock(context.Context) error {
	l.events = append(l.events, "unlock")
	l.locked.Store(false)
	return l.unlockErr
}

type mutexMigrationLocker struct{ mu sync.Mutex }

func (l *mutexMigrationLocker) Lock(context.Context) error {
	l.mu.Lock()
	return nil
}

func (l *mutexMigrationLocker) Unlock(context.Context) error {
	l.mu.Unlock()
	return nil
}
