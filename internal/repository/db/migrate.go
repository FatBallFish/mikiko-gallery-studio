package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/fatballfish/pic-gallery/internal/config"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/lib/pq"
)

const (
	// CurrentDatabaseSchemaVersion is advanced whenever an application release
	// requires a database migration before ordinary nodes may start.
	CurrentDatabaseSchemaVersion = 5

	// A fixed signed 64-bit key coordinates every explicit migrator for one
	// PostgreSQL database. Session locks are scoped by database, so installations
	// in different databases do not block one another.
	migrationAdvisoryLockKey int64 = 0x5047434D49475231

	acquireMigrationLockSQL              = `SELECT pg_advisory_lock($1)`
	releaseMigrationLockSQL              = `SELECT pg_advisory_unlock($1)`
	installationSingletonKey             = "installation"
	projectBackfillBatchSize             = 100
	projectBackfillMaxBatches            = 100
	projectBackfillBatchPause            = 10 * time.Millisecond
	referenceAssetNameBackfillBatchSize  = 100
	referenceAssetNameBackfillMaxBatches = 100
	referenceAssetNameBackfillBatchPause = 10 * time.Millisecond
)

var migrationIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type MigrationRequest struct {
	InstallationID string
	AppVersion     string
	ConfigVersion  int
}

type SchemaVersion struct {
	InstallationID        string
	AppVersion            string
	ConfigVersion         int
	DatabaseSchemaVersion int
}

// CompatibilityExpectation names SchemaVersion's role at a node startup
// boundary while preserving the plan's public SchemaVersion API.
type CompatibilityExpectation = SchemaVersion

type MigrationResult struct {
	Previous       *SchemaVersion
	Current        SchemaVersion
	Changed        bool
	BackfilledRows int
	MigratedAt     time.Time
}

type ProjectBackfillIncompleteError struct {
	Progress ProjectBackfillProgress
}

func (e *ProjectBackfillIncompleteError) Error() string {
	return fmt.Sprintf("project ownership backfill paused in phase %q after %d batches and %d processed rows; rerun migration to resume", e.Progress.Phase, e.Progress.Batches, e.Progress.ProcessedRows)
}

type CompatibilityErrorKind string

const (
	CompatibilityMissing                CompatibilityErrorKind = "missing"
	CompatibilityDuplicate              CompatibilityErrorKind = "duplicate"
	CompatibilitySingletonMismatch      CompatibilityErrorKind = "singleton_mismatch"
	CompatibilityInstallationMismatch   CompatibilityErrorKind = "installation_mismatch"
	CompatibilityAppVersionMismatch     CompatibilityErrorKind = "app_version_mismatch"
	CompatibilityConfigSchemaMismatch   CompatibilityErrorKind = "config_schema_mismatch"
	CompatibilityDatabaseSchemaMismatch CompatibilityErrorKind = "database_schema_mismatch"
)

type CompatibilityError struct {
	Kind     CompatibilityErrorKind
	Expected string
	Actual   string
	Cause    error
}

func (e *CompatibilityError) Error() string {
	if e == nil {
		return "schema compatibility error"
	}
	message := "database schema is incompatible: " + string(e.Kind)
	if e.Expected != "" || e.Actual != "" {
		message += fmt.Sprintf(" (expected %q, got %q)", e.Expected, e.Actual)
	}
	return message
}

func (e *CompatibilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type migrationLock interface {
	Lock(context.Context) error
	Unlock(context.Context) error
}

type sqlRowScanner interface {
	Scan(...any) error
}

type migrationSQLSession interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) sqlRowScanner
}

type sqlConnSession struct{ connection *sql.Conn }

func (s *sqlConnSession) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.connection.ExecContext(ctx, query, args...)
}

func (s *sqlConnSession) QueryRowContext(ctx context.Context, query string, args ...any) sqlRowScanner {
	return s.connection.QueryRowContext(ctx, query, args...)
}

type postgresAdvisoryLocker struct{ session migrationSQLSession }

func newPostgresAdvisoryLocker(session migrationSQLSession) migrationLock {
	return &postgresAdvisoryLocker{session: session}
}

func (l *postgresAdvisoryLocker) Lock(ctx context.Context) error {
	if _, err := l.session.ExecContext(ctx, acquireMigrationLockSQL, migrationAdvisoryLockKey); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("acquire database migration lock: %w", contextErr)
		}
		return fmt.Errorf("acquire database migration lock: %w", err)
	}
	return nil
}

func (l *postgresAdvisoryLocker) Unlock(ctx context.Context) error {
	var unlocked bool
	if err := l.session.QueryRowContext(ctx, releaseMigrationLockSQL, migrationAdvisoryLockKey).Scan(&unlocked); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("release database migration lock: %w", contextErr)
		}
		return fmt.Errorf("release database migration lock: %w", err)
	}
	if !unlocked {
		return fmt.Errorf("release database migration lock: lock was not held by this session")
	}
	return nil
}

func runMigrationLifecycle(ctx context.Context, locker migrationLock, migrate func(context.Context) error) (err error) {
	if locker == nil {
		return fmt.Errorf("database migration lock is required")
	}
	if migrate == nil {
		return fmt.Errorf("database migration operation is required")
	}
	if err := locker.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if unlockErr := locker.Unlock(unlockCtx); unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()
	return migrate(ctx)
}

func Migrate(ctx context.Context, databaseURL string, req MigrationRequest) (result MigrationResult, err error) {
	if err := validateMigrationRequest(databaseURL, req); err != nil {
		return MigrationResult{}, err
	}
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("open PostgreSQL migration connection: %w", err)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close PostgreSQL migration connection: %w", closeErr))
		}
	}()
	connection, err := database.Conn(ctx)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("reserve PostgreSQL migration lock connection: %w", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close PostgreSQL migration lock connection: %w", closeErr))
		}
	}()

	err = runMigrationLifecycle(ctx, newPostgresAdvisoryLocker(&sqlConnSession{connection: connection}), func(ctx context.Context) error {
		result, err = migrateLocked(ctx, database, req)
		return err
	})
	return result, err
}

func migrateLocked(ctx context.Context, database *sql.DB, req MigrationRequest) (MigrationResult, error) {
	if err := preflightInstallationMigration(ctx, database, req); err != nil {
		return MigrationResult{}, err
	}
	if err := prepareLegacyDataWithExecutor(ctx, database); err != nil {
		return MigrationResult{}, fmt.Errorf("prepare legacy database data: %w", err)
	}
	driver := entsql.OpenDB(dialect.Postgres, database)
	client := repoent.NewClient(repoent.Driver(driver))
	if err := client.Schema.Create(ctx); err != nil {
		return MigrationResult{}, fmt.Errorf("create database schema: %w", err)
	}
	backfilled, err := BackfillLegacyModelAccountCapabilities(ctx, client)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("backfill legacy model account capabilities: %w", err)
	}
	sizeBoundsBackfilled, err := BackfillLegacyModelAccountSizeBounds(ctx, client)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("backfill legacy model account size bounds: %w", err)
	}
	projectProgress, err := RunProjectOwnershipBackfill(ctx, client, ProjectBackfillOptions{
		BatchSize: projectBackfillBatchSize, MaxBatches: projectBackfillMaxBatches, BatchPause: projectBackfillBatchPause,
	})
	if err != nil {
		return MigrationResult{}, fmt.Errorf("backfill legacy project ownership: %w", err)
	}
	if err := requireCompletedProjectBackfill(projectProgress); err != nil {
		return MigrationResult{}, err
	}
	referenceAssetNameProgress, err := RunReferenceAssetNameBackfill(ctx, client, ReferenceAssetNameBackfillOptions{
		BatchSize: referenceAssetNameBackfillBatchSize, MaxBatches: referenceAssetNameBackfillMaxBatches, BatchPause: referenceAssetNameBackfillBatchPause,
	})
	if err != nil {
		return MigrationResult{}, fmt.Errorf("backfill historical reference asset names: %w", err)
	}
	if err := requireCompletedReferenceAssetNameBackfill(referenceAssetNameProgress); err != nil {
		return MigrationResult{}, err
	}
	result, err := recordInstallationMigration(ctx, client, req)
	if err != nil {
		return MigrationResult{}, err
	}
	result.BackfilledRows = backfilled + sizeBoundsBackfilled + projectProgress.UpdatedRows + referenceAssetNameProgress.UpdatedRows
	return result, nil
}

func requireCompletedProjectBackfill(progress ProjectBackfillProgress) error {
	if progress.Completed {
		return nil
	}
	return &ProjectBackfillIncompleteError{Progress: progress}
}

func preflightInstallationMigration(ctx context.Context, database *sql.DB, req MigrationRequest) error {
	var tableName sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT to_regclass(current_schema() || '.installations')::text`).Scan(&tableName); err != nil {
		return fmt.Errorf("inspect installation migration table: %w", err)
	}
	if !tableName.Valid || tableName.String == "" {
		return nil
	}
	rows, err := database.QueryContext(ctx, `
		SELECT id, singleton_key, installation_id, config_schema_version,
		       database_schema_version, app_version, migrated_at
		FROM installations
		ORDER BY id
		LIMIT 2`)
	if err != nil {
		return fmt.Errorf("query installation migration preflight: %w", err)
	}
	defer rows.Close()
	snapshots := make([]installationSnapshot, 0, 2)
	for rows.Next() {
		var snapshot installationSnapshot
		if err := rows.Scan(
			&snapshot.ID,
			&snapshot.SingletonKey,
			&snapshot.InstallationID,
			&snapshot.ConfigVersion,
			&snapshot.DatabaseVersion,
			&snapshot.AppVersion,
			&snapshot.MigratedAt,
		); err != nil {
			return fmt.Errorf("scan installation migration preflight: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate installation migration preflight: %w", err)
	}
	existing, err := validateInstallationSnapshots(snapshots)
	if errors.Is(err, errInstallationMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.InstallationID != req.InstallationID {
		return compatibilityMismatch(CompatibilityInstallationMismatch, req.InstallationID, existing.InstallationID)
	}
	if existing.DatabaseVersion > CurrentDatabaseSchemaVersion {
		return compatibilityMismatch(CompatibilityDatabaseSchemaMismatch, fmt.Sprint(CurrentDatabaseSchemaVersion), fmt.Sprint(existing.DatabaseVersion))
	}
	if existing.ConfigVersion > req.ConfigVersion {
		return compatibilityMismatch(CompatibilityConfigSchemaMismatch, fmt.Sprint(req.ConfigVersion), fmt.Sprint(existing.ConfigVersion))
	}
	return nil
}

func recordInstallationMigration(ctx context.Context, client *repoent.Client, req MigrationRequest) (MigrationResult, error) {
	installations, err := client.Installation.Query().Limit(2).All(ctx)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("query installation migration state: %w", err)
	}
	snapshots := make([]installationSnapshot, 0, len(installations))
	for _, installation := range installations {
		snapshots = append(snapshots, installationSnapshotFromEntity(installation))
	}
	existing, err := validateInstallationSnapshots(snapshots)
	if err != nil && !errors.Is(err, errInstallationMissing) {
		return MigrationResult{}, err
	}
	now := time.Now().UTC()
	current := SchemaVersion{
		InstallationID:        req.InstallationID,
		AppVersion:            req.AppVersion,
		ConfigVersion:         req.ConfigVersion,
		DatabaseSchemaVersion: CurrentDatabaseSchemaVersion,
	}
	if errors.Is(err, errInstallationMissing) {
		if _, createErr := client.Installation.Create().
			SetSingletonKey(installationSingletonKey).
			SetInstallationID(req.InstallationID).
			SetConfigSchemaVersion(req.ConfigVersion).
			SetDatabaseSchemaVersion(CurrentDatabaseSchemaVersion).
			SetAppVersion(req.AppVersion).
			SetInitializedAt(now).
			SetMigratedAt(now).
			Save(ctx); createErr != nil {
			return MigrationResult{}, fmt.Errorf("create installation migration state: %w", createErr)
		}
		return MigrationResult{Current: current, Changed: true, MigratedAt: now}, nil
	}
	previous := existing.schemaVersion()
	if existing.InstallationID != req.InstallationID {
		return MigrationResult{}, compatibilityMismatch(CompatibilityInstallationMismatch, req.InstallationID, existing.InstallationID)
	}
	if existing.DatabaseVersion > CurrentDatabaseSchemaVersion {
		return MigrationResult{}, compatibilityMismatch(CompatibilityDatabaseSchemaMismatch, fmt.Sprint(CurrentDatabaseSchemaVersion), fmt.Sprint(existing.DatabaseVersion))
	}
	if existing.ConfigVersion > req.ConfigVersion {
		return MigrationResult{}, compatibilityMismatch(CompatibilityConfigSchemaMismatch, fmt.Sprint(req.ConfigVersion), fmt.Sprint(existing.ConfigVersion))
	}
	if previous == current {
		return MigrationResult{Previous: &previous, Current: current, Changed: false, MigratedAt: existing.MigratedAt}, nil
	}
	if _, err := client.Installation.UpdateOneID(existing.ID).
		SetConfigSchemaVersion(req.ConfigVersion).
		SetDatabaseSchemaVersion(CurrentDatabaseSchemaVersion).
		SetAppVersion(req.AppVersion).
		SetMigratedAt(now).
		Save(ctx); err != nil {
		return MigrationResult{}, fmt.Errorf("update installation migration state: %w", err)
	}
	return MigrationResult{Previous: &previous, Current: current, Changed: true, MigratedAt: now}, nil
}

func CheckSchemaCompatibility(ctx context.Context, client *repoent.Client, expected SchemaVersion) error {
	if client == nil {
		return fmt.Errorf("schema compatibility client is required")
	}
	if err := validateSchemaVersion(expected); err != nil {
		return err
	}
	installations, err := client.Installation.Query().Limit(2).All(ctx)
	if err != nil {
		if isMissingInstallationTable(err) {
			return &CompatibilityError{Kind: CompatibilityMissing, Cause: err}
		}
		return fmt.Errorf("query installation schema compatibility: %w", err)
	}
	snapshots := make([]installationSnapshot, 0, len(installations))
	for _, installation := range installations {
		snapshots = append(snapshots, installationSnapshotFromEntity(installation))
	}
	actual, err := validateInstallationSnapshots(snapshots)
	if err != nil {
		return err
	}
	if actual.InstallationID != expected.InstallationID {
		return compatibilityMismatch(CompatibilityInstallationMismatch, expected.InstallationID, actual.InstallationID)
	}
	if actual.AppVersion != expected.AppVersion {
		return compatibilityMismatch(CompatibilityAppVersionMismatch, expected.AppVersion, actual.AppVersion)
	}
	if actual.ConfigVersion != expected.ConfigVersion {
		return compatibilityMismatch(CompatibilityConfigSchemaMismatch, fmt.Sprint(expected.ConfigVersion), fmt.Sprint(actual.ConfigVersion))
	}
	if actual.DatabaseVersion != expected.DatabaseSchemaVersion {
		return compatibilityMismatch(CompatibilityDatabaseSchemaMismatch, fmt.Sprint(expected.DatabaseSchemaVersion), fmt.Sprint(actual.DatabaseVersion))
	}
	return nil
}

func validateMigrationRequest(databaseURL string, req MigrationRequest) error {
	parsed, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil {
		return fmt.Errorf("database URL is invalid")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("database URL must use PostgreSQL")
	}
	if parsed.Hostname() == "" || strings.Trim(parsed.EscapedPath(), "/") == "" || parsed.Fragment != "" {
		return fmt.Errorf("database URL must include a host and database name without a fragment")
	}
	if !migrationIdentifierPattern.MatchString(req.InstallationID) {
		return fmt.Errorf("installation ID must be a stable identifier of at most 128 characters")
	}
	if err := config.ValidateApplicationVersion(req.AppVersion); err != nil {
		return err
	}
	if req.ConfigVersion != config.CurrentRuntimeSchemaVersion {
		return fmt.Errorf("configuration schema version must equal current runtime schema version %d", config.CurrentRuntimeSchemaVersion)
	}
	return nil
}

func validateSchemaVersion(version SchemaVersion) error {
	if err := validateMigrationRequest("postgres://validation.invalid/database", MigrationRequest{
		InstallationID: version.InstallationID,
		AppVersion:     version.AppVersion,
		ConfigVersion:  version.ConfigVersion,
	}); err != nil {
		return err
	}
	if version.DatabaseSchemaVersion <= 0 {
		return fmt.Errorf("database schema version must be positive")
	}
	return nil
}

var errInstallationMissing = errors.New("installation migration state is missing")

type installationSnapshot struct {
	ID              int
	SingletonKey    string
	InstallationID  string
	ConfigVersion   int
	DatabaseVersion int
	AppVersion      string
	MigratedAt      time.Time
}

func installationSnapshotFromEntity(installation *repoent.Installation) installationSnapshot {
	return installationSnapshot{
		ID:              installation.ID,
		SingletonKey:    installation.SingletonKey,
		InstallationID:  installation.InstallationID,
		ConfigVersion:   installation.ConfigSchemaVersion,
		DatabaseVersion: installation.DatabaseSchemaVersion,
		AppVersion:      installation.AppVersion,
		MigratedAt:      installation.MigratedAt,
	}
}

func (s installationSnapshot) schemaVersion() SchemaVersion {
	return SchemaVersion{
		InstallationID:        s.InstallationID,
		AppVersion:            s.AppVersion,
		ConfigVersion:         s.ConfigVersion,
		DatabaseSchemaVersion: s.DatabaseVersion,
	}
}

func validateInstallationSnapshots(rows []installationSnapshot) (installationSnapshot, error) {
	switch len(rows) {
	case 0:
		return installationSnapshot{}, &CompatibilityError{Kind: CompatibilityMissing, Cause: errInstallationMissing}
	case 1:
		if rows[0].SingletonKey != installationSingletonKey {
			return installationSnapshot{}, compatibilityMismatch(CompatibilitySingletonMismatch, installationSingletonKey, rows[0].SingletonKey)
		}
		return rows[0], nil
	default:
		return installationSnapshot{}, &CompatibilityError{Kind: CompatibilityDuplicate, Actual: fmt.Sprint(len(rows))}
	}
}

func compatibilityMismatch(kind CompatibilityErrorKind, expected, actual string) error {
	return &CompatibilityError{Kind: kind, Expected: expected, Actual: actual}
}

func isMissingInstallationTable(err error) bool {
	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) && postgresErr.Code == "42P01" {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") && strings.Contains(message, "installations")
}
