package storageconfig

import (
	"context"

	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

type Store interface {
	List(ctx context.Context) ([]domainstorageconfig.ConfigRecord, error)
	GetByID(ctx context.Context, id string) (domainstorageconfig.ConfigRecord, bool, error)
	GetByCode(ctx context.Context, code string) (domainstorageconfig.ConfigRecord, bool, error)
	GetDefaultWritable(ctx context.Context) (domainstorageconfig.ConfigRecord, bool, error)
	GetLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ConfigRecord, bool, error)
	Save(ctx context.Context, record domainstorageconfig.ConfigRecord) (domainstorageconfig.ConfigRecord, error)
	ClearDefault(ctx context.Context) error
}
