package videocallback

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

func TestServiceVerifiesChallengeAndPersistsNormalEventsIdempotently(t *testing.T) {
	accountID := uuid.New()
	provider := &callbackProvider{event: videoprovider.CallbackEvent{Challenge: "verify-me"}}
	store := &callbackStore{provider: provider}
	service := NewService(store, store)

	challenge, err := service.Receive(t.Context(), "minimax", accountID, http.Header{}, []byte(`{"challenge":"verify-me"}`))
	if err != nil || challenge.Challenge != "verify-me" || challenge.Accepted || len(store.records) != 0 {
		t.Fatalf("challenge = %#v records=%#v err=%v", challenge, store.records, err)
	}

	provider.event = videoprovider.CallbackEvent{
		JobID:  "job-1",
		Status: videoprovider.Status{JobID: "job-1", State: videoprovider.StateRunning, Raw: map[string]any{"status": "running"}},
	}
	body := []byte(`{"task":{"id":"job-1","status":"running"}}`)
	first, err := service.Receive(t.Context(), "minimax", accountID, http.Header{}, body)
	if err != nil || !first.Accepted || first.Duplicate || len(store.records) != 1 {
		t.Fatalf("first = %#v records=%#v err=%v", first, store.records, err)
	}
	second, err := service.Receive(t.Context(), "minimax", accountID, http.Header{}, body)
	if err != nil || !second.Accepted || !second.Duplicate || len(store.records) != 1 {
		t.Fatalf("second = %#v records=%#v err=%v", second, store.records, err)
	}
	if store.records[0].ProviderEventID == "" || store.records[0].ProviderJobID != "job-1" || store.records[0].PayloadSnapshot["state"] != "running" {
		t.Fatalf("record = %#v", store.records[0])
	}
	normalized, _ := store.records[0].PayloadSnapshot["usage_normalized"].(map[string]any)
	if normalized["output_seconds"] != "5.000" || normalized["provider_tokens"] != "1200" {
		t.Fatalf("normalized callback usage = %#v", normalized)
	}
}

type callbackProvider struct{ event videoprovider.CallbackEvent }

func (p *callbackProvider) Submit(context.Context, videoprovider.Request) (videoprovider.Job, error) {
	return videoprovider.Job{}, nil
}
func (p *callbackProvider) Get(context.Context, videoprovider.JobRef) (videoprovider.Status, error) {
	return videoprovider.Status{}, nil
}
func (p *callbackProvider) Cancel(context.Context, videoprovider.JobRef) (videoprovider.CancelResult, error) {
	return videoprovider.CancelResult{}, nil
}
func (p *callbackProvider) VerifyCallback(context.Context, http.Header, []byte) (videoprovider.CallbackEvent, error) {
	return p.event, nil
}
func (p *callbackProvider) NormalizeUsage(videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{OutputSeconds: "5.000", ProviderTokens: "1200"}, nil
}

type callbackStore struct {
	provider videoprovider.Provider
	records  []EventRecord
	seen     map[string]struct{}
}

func (s *callbackStore) ResolveProvider(context.Context, string, uuid.UUID) (ResolvedProvider, error) {
	return ResolvedProvider{ModelAccountID: 7, Provider: s.provider}, nil
}
func (s *callbackStore) RecordEvent(_ context.Context, record EventRecord) (bool, error) {
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if _, ok := s.seen[record.ProviderEventID]; ok {
		return true, nil
	}
	s.seen[record.ProviderEventID] = struct{}{}
	s.records = append(s.records, record)
	return false, nil
}
