//go:build cgo

package db

import (
	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	_ "github.com/mattn/go-sqlite3"
)

func openSQLite(url string) (*repoent.Client, error) {
	return repoent.Open(dialect.SQLite, sqliteDSN(url))
}
