package adminauth

import (
	"context"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
)

type Store interface {
	GetAdminByEmail(ctx context.Context, email string) (domainadminauth.AdminUser, error)
	GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error)
	CreateAdmin(ctx context.Context, admin domainadminauth.AdminUser) (domainadminauth.AdminUser, error)
	UpdateAdminPasswordHash(ctx context.Context, id int64, oldHash string, newHash string) error
}
