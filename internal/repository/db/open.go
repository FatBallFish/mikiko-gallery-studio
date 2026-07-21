package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/lib/pq"
)

func Open(url string) (*repoent.Client, error) {
	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		return repoent.Open(dialect.Postgres, url)
	case strings.HasPrefix(url, "file:"), strings.HasPrefix(url, "sqlite://"), strings.HasSuffix(url, ".db"), url == ":memory:":
		return openSQLite(url)
	default:
		return repoent.Open(dialect.Postgres, url)
	}
}

// OpenContext applies the caller deadline to PostgreSQL connection establishment.
// The returned client continues to use normal per-operation contexts after startup.
func OpenContext(ctx context.Context, databaseURL string) (*repoent.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return Open(databaseURL)
	}
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		return Open(databaseURL)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, context.DeadlineExceeded
	}
	connectorConfig, err := pq.NewConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if connectorConfig.ConnectTimeout <= 0 || remaining < connectorConfig.ConnectTimeout {
		connectorConfig.ConnectTimeout = remaining
	}
	connector, err := pq.NewConnectorConfig(connectorConfig)
	if err != nil {
		return nil, err
	}
	database := sql.OpenDB(connector)
	return repoent.NewClient(repoent.Driver(entsql.OpenDB(dialect.Postgres, database))), nil
}

func sqliteDSN(url string) string {
	return strings.TrimPrefix(url, "sqlite://")
}

func sqliteUnavailableError() error {
	return fmt.Errorf("sqlite database URLs require a cgo-enabled build")
}
