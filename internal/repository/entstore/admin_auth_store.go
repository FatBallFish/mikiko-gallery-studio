package entstore

import (
	"context"
	"strings"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type AdminAuthStore struct {
	client *repoent.Client
}

func NewAdminAuthStore(client *repoent.Client) *AdminAuthStore {
	return &AdminAuthStore{client: client}
}

func (s *AdminAuthStore) ListAdmins(ctx context.Context, req domainadminauth.AdminListRequest) (domainadminauth.AdminListPage, error) {
	page, pageSize := normalizeAdminPage(req.Page, req.PageSize)
	query := s.client.AdminUser.Query()
	where := make([]predicate.AdminUser, 0, 3)
	if q := normalizeAdminEmail(req.Query); q != "" {
		where = append(where, adminuser.EmailContains(q))
	}
	if role := strings.TrimSpace(req.Role); role != "" {
		where = append(where, adminuser.RoleEQ(role))
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		where = append(where, adminuser.StatusEQ(status))
	}
	if len(where) > 0 {
		query = query.Where(where...)
	}
	total, err := query.Count(ctx)
	if err != nil {
		return domainadminauth.AdminListPage{}, err
	}
	entities, err := query.Order(repoent.Asc(adminuser.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainadminauth.AdminListPage{}, err
	}
	items := make([]domainadminauth.AdminUser, 0, len(entities))
	for _, entity := range entities {
		items = append(items, mapAdminUserEntity(entity))
	}
	return domainadminauth.AdminListPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
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
		if repoent.IsConstraintError(err) {
			return domainadminauth.AdminUser{}, repoerr.ErrConflict
		}
		return domainadminauth.AdminUser{}, err
	}
	return mapAdminUserEntity(entity), nil
}

func (s *AdminAuthStore) UpdateAdmin(ctx context.Context, id int64, role string, status string, setRole bool, setStatus bool) (domainadminauth.AdminUser, error) {
	update := s.client.AdminUser.UpdateOneID(int(id))
	if setRole {
		update.SetRole(role)
	}
	if setStatus {
		update.SetStatus(status)
	}
	entity, err := update.Save(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainadminauth.AdminUser{}, repoerr.ErrNotFound
		}
		return domainadminauth.AdminUser{}, err
	}
	return mapAdminUserEntity(entity), nil
}

func (s *AdminAuthStore) UpdateAdminPassword(ctx context.Context, id int64, passwordHash string) error {
	err := s.client.AdminUser.UpdateOneID(int(id)).SetPasswordHash(passwordHash).Exec(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	return nil
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

func (s *AdminAuthStore) DeleteAdmin(ctx context.Context, id int64) error {
	err := s.client.AdminUser.DeleteOneID(int(id)).Exec(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *AdminAuthStore) CountActiveSuperAdmins(ctx context.Context) (int, error) {
	return s.client.AdminUser.Query().Where(adminuser.RoleEQ(domainadminauth.RoleSuperAdmin), adminuser.StatusEQ("active")).Count(ctx)
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

func normalizeAdminPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
