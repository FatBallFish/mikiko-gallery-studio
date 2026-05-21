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

	stored, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	metadata := stored[0].Metadata
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
