package entstore

import (
	"context"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/auditlog"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
)

type AuditStore struct{ client *repoent.Client }

func NewAuditStore(client *repoent.Client) *AuditStore { return &AuditStore{client: client} }

func (s *AuditStore) Write(ctx context.Context, event auditservice.Event) error {
	return s.client.AuditLog.Create().
		SetActorType(event.ActorType).
		SetActorID(event.ActorID).
		SetAction(event.Action).
		SetTargetType(event.TargetType).
		SetTargetID(event.TargetID).
		SetResult(defaultString(event.Result, "success")).
		SetMetadata(event.Metadata).
		SetIPAddr(event.IPAddr).
		SetUserAgent(event.UserAgent).
		Exec(ctx)
}

func (s *AuditStore) List(ctx context.Context, filter auditservice.Filter) ([]auditservice.Event, int, error) {
	query := s.client.AuditLog.Query()
	if filter.ActorType != "" {
		query = query.Where(auditlog.ActorTypeEQ(filter.ActorType))
	}
	if filter.ActorID != "" {
		query = query.Where(auditlog.ActorIDEQ(filter.ActorID))
	}
	if filter.Action != "" {
		query = query.Where(auditlog.ActionEQ(filter.Action))
	}
	if filter.TargetType != "" {
		query = query.Where(auditlog.TargetTypeEQ(filter.TargetType))
	}
	if filter.TargetID != "" {
		query = query.Where(auditlog.TargetIDEQ(filter.TargetID))
	}
	if filter.Result != "" {
		query = query.Where(auditlog.ResultEQ(filter.Result))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	items, err := query.Order(repoent.Desc(auditlog.FieldCreatedAt)).Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	events := make([]auditservice.Event, 0, len(items))
	for _, item := range items {
		events = append(events, auditservice.Event{ActorType: item.ActorType, ActorID: item.ActorID, Action: item.Action, TargetType: item.TargetType, TargetID: item.TargetID, Result: item.Result, Metadata: item.Metadata, IPAddr: item.IPAddr, UserAgent: item.UserAgent, CreatedAt: item.CreatedAt})
	}
	return events, total, nil
}
