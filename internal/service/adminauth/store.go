package adminauth

import (
	"context"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
)

type Store interface {
	ListAdmins(ctx context.Context, req domainadminauth.AdminListRequest) (domainadminauth.AdminListPage, error)
	GetAdminByEmail(ctx context.Context, email string) (domainadminauth.AdminUser, error)
	GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error)
	CreateAdmin(ctx context.Context, admin domainadminauth.AdminUser) (domainadminauth.AdminUser, error)
	UpdateAdmin(ctx context.Context, id int64, role string, status string, setRole bool, setStatus bool) (domainadminauth.AdminUser, error)
	UpdateAdminPassword(ctx context.Context, id int64, passwordHash string) error
	UpdateAdminPasswordHash(ctx context.Context, id int64, oldHash string, newHash string) error
	DeleteAdmin(ctx context.Context, id int64) error
	CountActiveSuperAdmins(ctx context.Context) (int, error)
}
