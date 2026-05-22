package audit

import (
	"context"
	"testing"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
)

func TestServiceWritesAuditLogWithRedactedMetadata(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)

	log, err := svc.Record(ctx, domainaudit.RecordRequest{
		ActorType:  "admin",
		ActorID:    "42",
		Action:     "admin.login",
		TargetType: "admin_user",
		TargetID:   "42",
		Result:     "success",
		Metadata: map[string]any{
			"api_token": "should-not-persist",
			"password":  "should-not-persist",
			"nested": map[string]any{
				"clientSecret": "should-not-persist",
				"safe":         "kept",
			},
		},
		IPAddr:    "127.0.0.1",
		UserAgent: "agent-test",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if log.ID == 0 || log.Action != "admin.login" {
		t.Fatalf("unexpected log %#v", log)
	}

	stored, err := store.List(ctx, domainaudit.ListRequest{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	metadata := stored.Items[0].Metadata
	if metadata["api_token"] != RedactedValue || metadata["password"] != RedactedValue {
		t.Fatalf("expected top-level secrets redacted, got %#v", metadata)
	}
	nested, ok := metadata["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map, got %#v", metadata["nested"])
	}
	if nested["clientSecret"] != RedactedValue || nested["safe"] != "kept" {
		t.Fatalf("unexpected nested metadata %#v", nested)
	}
}

func TestServiceListsAuditLogsWithFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)

	first, err := svc.Record(ctx, domainaudit.RecordRequest{
		ActorType:  "admin",
		ActorID:    "1",
		Action:     "config.update",
		TargetType: "config_tab",
		TargetID:   "billing",
		Result:     "success",
	})
	if err != nil {
		t.Fatalf("Record first: %v", err)
	}
	second, err := svc.Record(ctx, domainaudit.RecordRequest{
		ActorType:  "admin",
		ActorID:    "2",
		Action:     "config.publish",
		TargetType: "config_tab",
		TargetID:   "billing",
		Result:     "failed",
	})
	if err != nil {
		t.Fatalf("Record second: %v", err)
	}
	page, err := svc.List(ctx, domainaudit.ListRequest{
		Page:      1,
		PageSize:  1,
		ActorType: "admin",
		Result:    "failed",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page %+v", page)
	}
	if page.Items[0].ID != second.ID {
		t.Fatalf("expected second record, got %+v", page.Items[0])
	}
	if page.Items[0].ID == first.ID {
		t.Fatalf("expected filtered result, got first record")
	}
}
