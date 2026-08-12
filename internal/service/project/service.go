package project

import (
	"context"
	"errors"
	"strings"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

var (
	ErrNotFound            = repoerr.ErrNotFound
	ErrNameConflict        = errors.New("project name already exists")
	ErrDefaultImmutable    = errors.New("default project is immutable")
	ErrProjectChanged      = errors.New("project changed")
	ErrIdempotencyConflict = errors.New("idempotency key already used for another project")
	ErrInvalid             = errors.New("invalid project request")
	ErrCanvasBusy          = errors.New("project contains canvas with active generation runs")
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
	Delete(context.Context, int64, string, domainproject.DeleteRequest) (domainproject.DeleteResult, error)
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
	if _, err := s.EnsureDefault(ctx, userID); err != nil {
		return domainproject.Project{}, err
	}
	name, nameKey, err := normalizeName(req.Name)
	if err != nil {
		return domainproject.Project{}, err
	}
	idempotencyKey, err := normalizeIdempotencyKey(req.IdempotencyKey)
	if err != nil {
		return domainproject.Project{}, err
	}
	return s.store.Create(ctx, userID, name, nameKey, idempotencyKey)
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
	projectID = strings.TrimSpace(projectID)
	req.TargetProjectID = strings.TrimSpace(req.TargetProjectID)
	var err error
	req.IdempotencyKey, err = normalizeIdempotencyKey(req.IdempotencyKey)
	if err != nil {
		return domainproject.DeleteResult{}, err
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	if userID <= 0 || projectID == "" || req.ExpectedVersion <= 0 {
		return domainproject.DeleteResult{}, ErrInvalid
	}
	if req.TargetProjectID == projectID {
		return domainproject.DeleteResult{}, ErrInvalid
	}
	return s.store.Delete(ctx, userID, projectID, req)
}

func normalizeName(value string) (string, string, error) {
	name := strings.Join(strings.Fields(value), " ")
	if name == "" || len([]rune(name)) > 128 || name == domainproject.DefaultName {
		return "", "", ErrInvalid
	}
	return name, strings.ToLower(name), nil
}

func normalizeIdempotencyKey(value string) (string, error) {
	key := strings.TrimSpace(value)
	if len([]rune(key)) > domainproject.MaxIdempotencyKeyLength {
		return "", ErrInvalid
	}
	return key, nil
}
