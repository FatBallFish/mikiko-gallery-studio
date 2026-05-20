package db

import (
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	_ "github.com/lib/pq"
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

func sqliteDSN(url string) string {
	return strings.TrimPrefix(url, "sqlite://")
}

func sqliteUnavailableError() error {
	return fmt.Errorf("sqlite database URLs require a cgo-enabled build")
}
