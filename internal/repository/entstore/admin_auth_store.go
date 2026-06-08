package entstore

import (
	"context"
	"strings"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type AdminAuthStore struct {
	client *repoent.Client
}

func NewAdminAuthStore(client *repoent.Client) *AdminAuthStore {
	return &AdminAuthStore{client: client}
}

func (s *AdminAuthStore) GetAdminByEmail(ctx context.Context, email string) (domainadminauth.AdminUser, error) {
	entity, err := s.client.AdminUser.Query().Where(adminuser.EmailEQ(normalizeAdminEmail(email))).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminauth.AdminUser{}, repoerr.ErrNotFound
		}
		return domainadminauth.AdminUser{}, err
	}
	return mapAdminUserEntity(entity), nil
}

func (s *AdminAuthStore) GetAdminByID(ctx context.Context, id int64) (domainadminauth.AdminUser, error) {
	entity, err := s.client.AdminUser.Get(ctx, int(id))
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminauth.AdminUser{}, repoerr.ErrNotFound
		}
		return domainadminauth.AdminUser{}, err
	}
	return mapAdminUserEntity(entity), nil
}

func (s *AdminAuthStore) CreateAdmin(ctx context.Context, admin domainadminauth.AdminUser) (domainadminauth.AdminUser, error) {
	role := admin.Role
	if role == "" {
		role = domainadminauth.RoleAdmin
	}
	status := admin.Status
	if status == "" {
		status = "active"
	}
	entity, err := s.client.AdminUser.Create().
		SetEmail(normalizeAdminEmail(admin.Email)).
		SetPasswordHash(admin.PasswordHash).
		SetRole(role).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return domainadminauth.AdminUser{}, err
	}
	return mapAdminUserEntity(entity), nil
}

func (s *AdminAuthStore) UpdateAdminPasswordHash(ctx context.Context, id int64, oldHash string, newHash string) error {
	updated, err := s.client.AdminUser.Update().
		Where(adminuser.IDEQ(int(id)), adminuser.PasswordHashEQ(oldHash)).
		SetPasswordHash(newHash).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func mapAdminUserEntity(entity *repoent.AdminUser) domainadminauth.AdminUser {
	if entity == nil {
		return domainadminauth.AdminUser{}
	}
	return domainadminauth.AdminUser{
		ID:           int64(entity.ID),
		Email:        entity.Email,
		PasswordHash: entity.PasswordHash,
		Role:         entity.Role,
		Status:       entity.Status,
		CreatedAt:    entity.CreatedAt,
		UpdatedAt:    entity.UpdatedAt,
	}
}

func normalizeAdminEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
