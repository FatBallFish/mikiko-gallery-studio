package assetgroup

import (
	"context"
	"strings"

	domain "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Store interface {
	List(context.Context, int64, string) ([]domain.Group, error)
	Get(context.Context, int64, string, string) (domain.Group, error)
	Create(context.Context, int64, string, domain.CreateRequest) (domain.Group, error)
	Update(context.Context, int64, string, string, domain.UpdateRequest) (domain.Group, error)
	Delete(context.Context, int64, string, string, domain.DeleteRequest) error
}
type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }
func NormalizeName(name string) (string, string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len(name) > 64 {
		return "", "", errs.BadRequest("group name must contain 1 to 64 bytes")
	}
	return name, strings.ToLower(name), nil
}
func (s *Service) List(ctx context.Context, userID int64, projectID string) ([]domain.Group, error) {
	return s.store.List(ctx, userID, projectID)
}
func (s *Service) Get(ctx context.Context, userID int64, projectID, id string) (domain.Group, error) {
	return s.store.Get(ctx, userID, projectID, id)
}
func (s *Service) Create(ctx context.Context, userID int64, projectID string, req domain.CreateRequest) (domain.Group, error) {
	name, _, err := NormalizeName(req.Name)
	if err != nil {
		return domain.Group{}, err
	}
	req.Name = name
	return s.store.Create(ctx, userID, projectID, req)
}
func (s *Service) Update(ctx context.Context, userID int64, projectID, id string, req domain.UpdateRequest) (domain.Group, error) {
	name, _, err := NormalizeName(req.Name)
	if err != nil {
		return domain.Group{}, err
	}
	req.Name = name
	if req.ExpectedVersion <= 0 {
		return domain.Group{}, errs.BadRequest("expected_version is required")
	}
	return s.store.Update(ctx, userID, projectID, id, req)
}
func (s *Service) Delete(ctx context.Context, userID int64, projectID, id string, req domain.DeleteRequest) error {
	if req.ExpectedVersion <= 0 {
		return errs.BadRequest("expected_version is required")
	}
	return s.store.Delete(ctx, userID, projectID, id, req)
}
