package audit

import (
	"context"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
)

type Store interface {
	Create(ctx context.Context, log domainaudit.Log) (domainaudit.Log, error)
	List(ctx context.Context) ([]domainaudit.Log, error)
}
