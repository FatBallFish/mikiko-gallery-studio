package audit

import (
	"context"
	"time"
)

type Event struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	Metadata   map[string]any
	IPAddr     string
	UserAgent  string
	CreatedAt  time.Time
}

type Store interface {
	Write(ctx context.Context, event Event) error
	List(ctx context.Context, filter Filter) ([]Event, int, error)
}

type Filter struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	Page       int
	PageSize   int
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Write(ctx context.Context, event Event) error {
	if s == nil || s.store == nil {
		return nil
	}
	if event.Result == "" {
		event.Result = "success"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.store.Write(ctx, event)
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Event, int, error) {
	if s == nil || s.store == nil {
		return []Event{}, 0, nil
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	return s.store.List(ctx, filter)
}
