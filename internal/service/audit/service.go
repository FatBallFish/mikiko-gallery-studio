package audit

import (
	"context"
	"strings"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const RedactedValue = "[REDACTED]"

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{store: store}
}

func (s *Service) Record(ctx context.Context, req domainaudit.RecordRequest) (domainaudit.Log, error) {
	if strings.TrimSpace(req.ActorType) == "" || strings.TrimSpace(req.ActorID) == "" || strings.TrimSpace(req.Action) == "" || strings.TrimSpace(req.TargetType) == "" || strings.TrimSpace(req.TargetID) == "" {
		return domainaudit.Log{}, errs.BadRequest("actor, action, and target are required")
	}
	result := strings.TrimSpace(req.Result)
	if result == "" {
		result = "success"
	}
	return s.store.Create(ctx, domainaudit.Log{
		ActorType:  strings.TrimSpace(req.ActorType),
		ActorID:    strings.TrimSpace(req.ActorID),
		Action:     strings.TrimSpace(req.Action),
		TargetType: strings.TrimSpace(req.TargetType),
		TargetID:   strings.TrimSpace(req.TargetID),
		Result:     result,
		Metadata:   redactMetadata(req.Metadata),
		IPAddr:     req.IPAddr,
		UserAgent:  req.UserAgent,
	})
}

func (s *Service) List(ctx context.Context, req domainaudit.ListRequest) (domainaudit.ListPage, error) {
	req.Page, req.PageSize = normalizePage(req.Page, req.PageSize)
	req.ActorType = strings.TrimSpace(req.ActorType)
	req.ActorID = strings.TrimSpace(req.ActorID)
	req.Action = strings.TrimSpace(req.Action)
	req.TargetType = strings.TrimSpace(req.TargetType)
	req.TargetID = strings.TrimSpace(req.TargetID)
	req.Result = strings.TrimSpace(req.Result)
	return s.store.List(ctx, req)
}

func redactMetadata(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		if isSecretKey(key) {
			output[key] = RedactedValue
			continue
		}
		output[key] = redactValue(value)
	}
	return output
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMetadata(typed)
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactValue(item)
		}
		return redacted
	default:
		return typed
	}
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password")
}

func normalizePage(page, pageSize int) (int, int) {
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

func cloneMetadata(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneValue(value)
	}
	return output
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMetadata(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneValue(item)
		}
		return cloned
	default:
		return typed
	}
}
