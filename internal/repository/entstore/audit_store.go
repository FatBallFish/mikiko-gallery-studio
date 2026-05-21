package entstore

import (
	"context"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
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

func (s *AuditStore) List(ctx context.Context) ([]domainaudit.Log, error) {
	entities, err := s.client.AuditLog.Query().Order(repoent.Desc("created_at")).Limit(200).All(ctx)
	if err != nil {
		return nil, err
	}
	logs := make([]domainaudit.Log, 0, len(entities))
	for _, entity := range entities {
		logs = append(logs, mapAuditLogEntity(entity))
	}
	return logs, nil
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
