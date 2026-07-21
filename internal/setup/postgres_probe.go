package setup

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/lib/pq"
)

type postgresProbeDatabase interface {
	Ping(context.Context) error
	IsSuperuser(context.Context) (bool, error)
	ServerVersion(context.Context) (string, error)
	Begin(context.Context) (postgresProbeTransaction, error)
}

type postgresProbeTransaction interface {
	Exec(context.Context, string) error
	QueryValue(context.Context, string) (string, error)
	Rollback() error
}

type sqlPostgresProbeDatabase struct{ database *sql.DB }
type sqlPostgresProbeTransaction struct{ transaction *sql.Tx }

func (database sqlPostgresProbeDatabase) Ping(ctx context.Context) error {
	return database.database.PingContext(ctx)
}

func (database sqlPostgresProbeDatabase) ServerVersion(ctx context.Context) (string, error) {
	var version string
	err := database.database.QueryRowContext(ctx, "SHOW server_version").Scan(&version)
	return version, err
}

func (database sqlPostgresProbeDatabase) IsSuperuser(ctx context.Context) (bool, error) {
	var superuser bool
	err := database.database.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_catalog.pg_roles
			WHERE rolname IN (session_user, current_user) AND rolsuper
		)`).Scan(&superuser)
	return superuser, err
}

func (database sqlPostgresProbeDatabase) Begin(ctx context.Context) (postgresProbeTransaction, error) {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlPostgresProbeTransaction{transaction: transaction}, nil
}

func (transaction sqlPostgresProbeTransaction) Exec(ctx context.Context, statement string) error {
	_, err := transaction.transaction.ExecContext(ctx, statement)
	return err
}

func (transaction sqlPostgresProbeTransaction) QueryValue(ctx context.Context, query string) (string, error) {
	var value string
	err := transaction.transaction.QueryRowContext(ctx, query).Scan(&value)
	return value, err
}

func (transaction sqlPostgresProbeTransaction) Rollback() error {
	return transaction.transaction.Rollback()
}

func validatePostgresProbeRequest(request PostgresProbeRequest) error {
	parsed, err := url.Parse(strings.TrimSpace(request.DatabaseURL))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return errors.New("invalid PostgreSQL URL")
	}
	if strings.Trim(parsed.EscapedPath(), "/") == "" {
		return errors.New("PostgreSQL target database is required")
	}
	return nil
}

func runPostgresProbe(ctx context.Context, databaseURL string) (version string, resultErr error) {
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return "", probeFailureError(ProbeCodeConnectionFailed, err)
	}
	defer database.Close()
	return runPostgresProbeWithDatabase(ctx, sqlPostgresProbeDatabase{database: database}, cryptorand.Reader)
}

func runPostgresProbeWithDatabase(ctx context.Context, database postgresProbeDatabase, random io.Reader) (version string, resultErr error) {
	if err := database.Ping(ctx); err != nil {
		return "", postgresProbeFailure(err, ProbeCodeConnectionFailed)
	}
	superuser, err := database.IsSuperuser(ctx)
	if err != nil {
		return "", postgresProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	if superuser {
		return "", probeFailureError(ProbeCodeUnsafePrivileges, errors.New("PostgreSQL probe account is a server superuser"))
	}
	version, err = database.ServerVersion(ctx)
	if err != nil {
		return "", postgresProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}

	transaction, err := database.Begin(ctx)
	if err != nil {
		return "", postgresProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	defer func() {
		rollbackErr := transaction.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			resultErr = probeFailureError(ProbeCodeCleanupFailed, errors.Join(resultErr, rollbackErr))
		}
	}()

	identifier, err := randomProbeIdentifier(random, "setup_probe_")
	if err != nil {
		return "", probeFailureError(ProbeCodeInternalError, err)
	}
	quotedTable := pq.QuoteIdentifier(identifier)
	quotedIndex := pq.QuoteIdentifier(identifier + "_value_idx")
	statements := []string{
		"CREATE TABLE " + quotedTable + " (probe_value text NOT NULL)",
		"CREATE UNIQUE INDEX " + quotedIndex + " ON " + quotedTable + " (probe_value)",
		"INSERT INTO " + quotedTable + " (probe_value) VALUES ('setup-probe')",
	}
	for _, statement := range statements {
		if err := transaction.Exec(ctx, statement); err != nil {
			return "", postgresProbeFailure(err, ProbeCodeReadWriteCheckFailed)
		}
	}
	value, err := transaction.QueryValue(ctx, "SELECT probe_value FROM "+quotedTable+" LIMIT 1")
	if err != nil || value != "setup-probe" {
		if err == nil {
			err = errors.New("PostgreSQL probe value mismatch")
		}
		return "", postgresProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	return version, nil
}

func postgresProbeFailure(err error, fallback string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	var postgresError *pq.Error
	if errors.As(err, &postgresError) {
		switch string(postgresError.Code) {
		case "28P01", "28000":
			return probeFailureError(ProbeCodeAuthenticationFailed, err)
		case "42501":
			return probeFailureError(ProbeCodeInsufficientPrivileges, err)
		}
	}
	return probeFailureError(fallback, err)
}

func randomProbeIdentifier(randomSource io.Reader, prefix string) (string, error) {
	random := make([]byte, 12)
	if _, err := io.ReadFull(randomSource, random); err != nil {
		return "", fmt.Errorf("generate probe identifier: %w", err)
	}
	identifier := prefix + hex.EncodeToString(random)
	clear(random)
	return identifier, nil
}
