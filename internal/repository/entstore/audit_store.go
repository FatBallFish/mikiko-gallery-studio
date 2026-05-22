package entstore

import (
	"context"
	"strings"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/auditlog"
)

type AuditStore struct {
	client *repoent.Client
}

func NewAuditStore(client *repoent.Client) *AuditStore {
	return &AuditStore{client: client}
}

func (s *AuditStore) Create(ctx context.Context, log domainaudit.Log) (domainaudit.Log, error) {
	result := log.Result
	if result == "" {
		result = "success"
	}
	entity, err := s.client.AuditLog.Create().
		SetActorType(log.ActorType).
		SetActorID(log.ActorID).
		SetAction(log.Action).
		SetTargetType(log.TargetType).
		SetTargetID(log.TargetID).
		SetResult(result).
		SetMetadata(cloneAuditMetadata(log.Metadata)).
		SetIPAddr(log.IPAddr).
		SetUserAgent(log.UserAgent).
		Save(ctx)
	if err != nil {
		return domainaudit.Log{}, err
	}
	return mapAuditLogEntity(entity), nil
}

func (s *AuditStore) List(ctx context.Context, req domainaudit.ListRequest) (domainaudit.ListPage, error) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	query := s.client.AuditLog.Query()
	if actorType := strings.TrimSpace(req.ActorType); actorType != "" {
		query.Where(auditlog.ActorTypeEqualFold(actorType))
	}
	if actorID := strings.TrimSpace(req.ActorID); actorID != "" {
		query.Where(auditlog.ActorIDEQ(actorID))
	}
	if action := strings.TrimSpace(req.Action); action != "" {
		query.Where(auditlog.ActionEqualFold(action))
	}
	if targetType := strings.TrimSpace(req.TargetType); targetType != "" {
		query.Where(auditlog.TargetTypeEqualFold(targetType))
	}
	if targetID := strings.TrimSpace(req.TargetID); targetID != "" {
		query.Where(auditlog.TargetIDEQ(targetID))
	}
	if result := strings.TrimSpace(req.Result); result != "" {
		query.Where(auditlog.ResultEqualFold(result))
	}
	if !req.CreatedFrom.IsZero() {
		query.Where(auditlog.CreatedAtGTE(req.CreatedFrom))
	}
	if !req.CreatedTo.IsZero() {
		query.Where(auditlog.CreatedAtLTE(req.CreatedTo))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return domainaudit.ListPage{}, err
	}
	entities, err := query.Order(repoent.Desc(auditlog.FieldCreatedAt), repoent.Desc(auditlog.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return domainaudit.ListPage{}, err
	}
	logs := make([]domainaudit.Log, 0, len(entities))
	for _, entity := range entities {
		logs = append(logs, mapAuditLogEntity(entity))
	}
	return domainaudit.ListPage{
		Items:    logs,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func mapAuditLogEntity(entity *repoent.AuditLog) domainaudit.Log {
	if entity == nil {
		return domainaudit.Log{}
	}
	return domainaudit.Log{
		ID:         int64(entity.ID),
		ActorType:  entity.ActorType,
		ActorID:    entity.ActorID,
		Action:     entity.Action,
		TargetType: entity.TargetType,
		TargetID:   entity.TargetID,
		Result:     entity.Result,
		Metadata:   cloneAuditMetadata(entity.Metadata),
		IPAddr:     entity.IPAddr,
		UserAgent:  entity.UserAgent,
		CreatedAt:  entity.CreatedAt,
		UpdatedAt:  entity.UpdatedAt,
	}
}

func cloneAuditMetadata(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneAuditValue(value)
	}
	return output
}

func cloneAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAuditMetadata(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneAuditValue(item)
		}
		return cloned
	default:
		return typed
	}
}
