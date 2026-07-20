package textmodel

import (
	"context"
	"sync"
	"time"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	ListAccounts(ctx context.Context) ([]domaintextmodel.AccountRecord, error)
	GetAccount(ctx context.Context, accountID int64) (domaintextmodel.AccountRecord, error)
	CreateAccount(ctx context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error)
	UpdateAccount(ctx context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error)
	DeleteAccount(ctx context.Context, accountID int64) error
	ListModels(ctx context.Context, accountID int64) ([]domaintextmodel.Model, error)
	GetModel(ctx context.Context, modelID int64) (domaintextmodel.Model, error)
	CreateModel(ctx context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error)
	UpdateModel(ctx context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error)
	DeleteModel(ctx context.Context, modelID int64) error
	SetDefaultModel(ctx context.Context, modelID int64) (domaintextmodel.Model, error)
	GetDefaultModel(ctx context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, error)
	SaveOptimizationRun(ctx context.Context, run domaintextmodel.OptimizationRun) (domaintextmodel.OptimizationRun, error)
}

type MemoryStore struct {
	mu       sync.Mutex
	nextID   int64
	accounts map[int64]domaintextmodel.AccountRecord
	models   map[int64]domaintextmodel.Model
	runs     map[string]domaintextmodel.OptimizationRun
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1, accounts: map[int64]domaintextmodel.AccountRecord{}, models: map[int64]domaintextmodel.Model{}, runs: map[string]domaintextmodel.OptimizationRun{}}
}

func (s *MemoryStore) ListAccounts(context.Context) ([]domaintextmodel.AccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domaintextmodel.AccountRecord, 0, len(s.accounts))
	for _, item := range s.accounts {
		if item.DeletedAt == nil {
			result = append(result, cloneAccountRecord(item))
		}
	}
	return result, nil
}

func (s *MemoryStore) GetAccount(_ context.Context, accountID int64) (domaintextmodel.AccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.accounts[accountID]
	if !ok || item.DeletedAt != nil {
		return domaintextmodel.AccountRecord{}, repoerr.ErrNotFound
	}
	return cloneAccountRecord(item), nil
}

func (s *MemoryStore) CreateAccount(_ context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	record.ID = s.nextID
	s.nextID++
	record.CreatedAt, record.UpdatedAt = now, now
	s.accounts[record.ID] = cloneAccountRecord(record)
	return cloneAccountRecord(record), nil
}

func (s *MemoryStore) UpdateAccount(_ context.Context, record domaintextmodel.AccountRecord) (domaintextmodel.AccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.accounts[record.ID]
	if !ok || current.DeletedAt != nil {
		return domaintextmodel.AccountRecord{}, repoerr.ErrNotFound
	}
	record.CreatedAt = current.CreatedAt
	record.UpdatedAt = time.Now().UTC()
	s.accounts[record.ID] = cloneAccountRecord(record)
	return cloneAccountRecord(record), nil
}

func (s *MemoryStore) DeleteAccount(_ context.Context, accountID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.accounts[accountID]
	if !ok || item.DeletedAt != nil {
		return repoerr.ErrNotFound
	}
	for _, model := range s.models {
		if model.AccountID == accountID && model.DeletedAt == nil {
			return repoerr.ErrConflict
		}
	}
	now := time.Now().UTC()
	item.DeletedAt = &now
	s.accounts[accountID] = item
	return nil
}

func (s *MemoryStore) ListModels(_ context.Context, accountID int64) ([]domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []domaintextmodel.Model{}
	for _, item := range s.models {
		if item.DeletedAt == nil && (accountID == 0 || item.AccountID == accountID) {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *MemoryStore) GetModel(_ context.Context, modelID int64) (domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.models[modelID]
	if !ok || item.DeletedAt != nil {
		return domaintextmodel.Model{}, repoerr.ErrNotFound
	}
	return item, nil
}

func (s *MemoryStore) CreateModel(_ context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	model.ID = s.nextID
	s.nextID++
	model.CreatedAt, model.UpdatedAt = now, now
	s.models[model.ID] = model
	return model, nil
}

func (s *MemoryStore) UpdateModel(_ context.Context, model domaintextmodel.Model) (domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.models[model.ID]
	if !ok || current.DeletedAt != nil {
		return domaintextmodel.Model{}, repoerr.ErrNotFound
	}
	model.CreatedAt = current.CreatedAt
	model.UpdatedAt = time.Now().UTC()
	s.models[model.ID] = model
	return model, nil
}

func (s *MemoryStore) DeleteModel(_ context.Context, modelID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.models[modelID]
	if !ok || item.DeletedAt != nil {
		return repoerr.ErrNotFound
	}
	now := time.Now().UTC()
	item.DeletedAt, item.IsDefault = &now, false
	s.models[modelID] = item
	return nil
}

func (s *MemoryStore) SetDefaultModel(_ context.Context, modelID int64) (domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.models[modelID]
	if !ok || target.DeletedAt != nil || !target.Enabled {
		return domaintextmodel.Model{}, repoerr.ErrNotFound
	}
	account, ok := s.accounts[target.AccountID]
	if !ok || account.DeletedAt != nil || !account.Enabled {
		return domaintextmodel.Model{}, repoerr.ErrConflict
	}
	for id, item := range s.models {
		item.IsDefault = id == modelID
		s.models[id] = item
	}
	target = s.models[modelID]
	target.Version++
	target.UpdatedAt = time.Now().UTC()
	s.models[modelID] = target
	return target, nil
}

func (s *MemoryStore) GetDefaultModel(_ context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, model := range s.models {
		account, ok := s.accounts[model.AccountID]
		if model.IsDefault && model.Enabled && model.DeletedAt == nil && ok && account.Enabled && account.DeletedAt == nil {
			return cloneAccountRecord(account), model, nil
		}
	}
	return domaintextmodel.AccountRecord{}, domaintextmodel.Model{}, repoerr.ErrNotFound
}

func (s *MemoryStore) SaveOptimizationRun(_ context.Context, run domaintextmodel.OptimizationRun) (domaintextmodel.OptimizationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	s.runs[run.ID] = run
	return run, nil
}

func (s *MemoryStore) GetOptimizationRun(_ context.Context, runID string) (domaintextmodel.OptimizationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return domaintextmodel.OptimizationRun{}, repoerr.ErrNotFound
	}
	return run, nil
}

func cloneAccountRecord(value domaintextmodel.AccountRecord) domaintextmodel.AccountRecord {
	value.SecretEncrypted = cloneMap(value.SecretEncrypted)
	return value
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
