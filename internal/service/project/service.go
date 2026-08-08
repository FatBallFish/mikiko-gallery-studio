package project

import (
	"context"
	"errors"
	"strings"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

var (
	ErrNotFound         = repoerr.ErrNotFound
	ErrNameConflict     = errors.New("project name already exists")
	ErrDefaultImmutable = errors.New("default project is immutable")
	ErrProjectChanged   = errors.New("project changed")
	ErrInvalid          = errors.New("invalid project request")
)

type NonEmptyError struct {
	Counts domainproject.OwnershipCounts
}

func (e *NonEmptyError) Error() string { return "project contains owned records" }

type Store interface {
	EnsureDefault(context.Context, int64) (domainproject.Project, error)
	List(context.Context, int64) ([]domainproject.Project, error)
	Get(context.Context, int64, string) (domainproject.Project, error)
	Create(context.Context, int64, string, string, string) (domainproject.Project, error)
	Rename(context.Context, int64, string, string, string, int64) (domainproject.Project, error)
	Delete(context.Context, int64, string, string, int64) (domainproject.DeleteResult, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

func NewServiceWithStore(store Store) *Service { return NewService(store) }

func (s *Service) EnsureDefault(ctx context.Context, userID int64) (domainproject.Project, error) {
	if userID <= 0 {
		return domainproject.Project{}, ErrInvalid
	}
	return s.store.EnsureDefault(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID int64) ([]domainproject.Project, error) {
	if _, err := s.EnsureDefault(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.List(ctx, userID)
}

func (s *Service) ResolveOwned(ctx context.Context, userID int64, projectID string) (domainproject.Project, error) {
	projectID = strings.TrimSpace(projectID)
	if userID <= 0 || projectID == "" {
		return domainproject.Project{}, ErrNotFound
	}
	return s.store.Get(ctx, userID, projectID)
}

func (s *Service) ResolveForWrite(ctx context.Context, userID int64, projectID string) (domainproject.Project, error) {
	if strings.TrimSpace(projectID) == "" {
		return s.EnsureDefault(ctx, userID)
	}
	return s.ResolveOwned(ctx, userID, projectID)
}

func (s *Service) Create(ctx context.Context, userID int64, req domainproject.CreateRequest) (domainproject.Project, error) {
	if userID <= 0 {
		return domainproject.Project{}, ErrInvalid
	}
	name, nameKey, err := normalizeName(req.Name)
	if err != nil {
		return domainproject.Project{}, err
	}
	return s.store.Create(ctx, userID, name, nameKey, strings.TrimSpace(req.IdempotencyKey))
}

func (s *Service) Rename(ctx context.Context, userID int64, projectID string, req domainproject.RenameRequest) (domainproject.Project, error) {
	if req.ExpectedVersion <= 0 {
		return domainproject.Project{}, ErrInvalid
	}
	current, err := s.ResolveOwned(ctx, userID, projectID)
	if err != nil {
		return domainproject.Project{}, err
	}
	if current.IsDefault {
		return domainproject.Project{}, ErrDefaultImmutable
	}
	name, nameKey, err := normalizeName(req.Name)
	if err != nil {
		return domainproject.Project{}, err
	}
	return s.store.Rename(ctx, userID, current.ID, name, nameKey, req.ExpectedVersion)
}

func (s *Service) Delete(ctx context.Context, userID int64, projectID string, req domainproject.DeleteRequest) (domainproject.DeleteResult, error) {
	if req.ExpectedVersion <= 0 {
		return domainproject.DeleteResult{}, ErrInvalid
	}
	current, err := s.ResolveOwned(ctx, userID, projectID)
	if err != nil {
		return domainproject.DeleteResult{}, err
	}
	if current.IsDefault {
		return domainproject.DeleteResult{}, ErrDefaultImmutable
	}
	targetID := strings.TrimSpace(req.TargetProjectID)
	if targetID == current.ID {
		return domainproject.DeleteResult{}, ErrInvalid
	}
	if targetID != "" {
		if _, err := s.ResolveOwned(ctx, userID, targetID); err != nil {
			return domainproject.DeleteResult{}, err
		}
	}
	return s.store.Delete(ctx, userID, current.ID, targetID, req.ExpectedVersion)
}

func normalizeName(value string) (string, string, error) {
	name := strings.Join(strings.Fields(value), " ")
	if name == "" || len([]rune(name)) > 128 || name == domainproject.DefaultName {
		return "", "", ErrInvalid
	}
	return name, strings.ToLower(name), nil
}
