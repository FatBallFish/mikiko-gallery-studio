package db

import (
	"strings"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func Open(url string) (*repoent.Client, error) {
	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		return repoent.Open(dialect.Postgres, url)
	case strings.HasPrefix(url, "file:"), strings.HasPrefix(url, "sqlite://"), strings.HasSuffix(url, ".db"), url == ":memory:":
		dsn := strings.TrimPrefix(url, "sqlite://")
		return repoent.Open(dialect.SQLite, dsn)
	default:
		return repoent.Open(dialect.Postgres, url)
	}
}
