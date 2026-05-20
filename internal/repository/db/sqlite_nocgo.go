//go:build !cgo

package db

import repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"

func openSQLite(string) (*repoent.Client, error) {
	return nil, sqliteUnavailableError()
}
