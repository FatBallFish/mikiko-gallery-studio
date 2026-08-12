package router

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
	videocallbackservice "github.com/fatballfish/pic-gallery/internal/service/videocallback"
)

func TestVideoProviderCallbackAPIHandlesChallengeAndAcceptedEvent(t *testing.T) {
	accountID := uuid.New()
	provider := &routerCallbackProvider{event: videoprovider.CallbackEvent{Challenge: "verify-me"}}
	store := &routerCallbackStore{provider: provider}
	service := videocallbackservice.NewService(store, store)
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), nil, nil, nil, nil, nil)
	api.SetVideoCallbackService(service)
	handler := NewWithAPI(api)

	challenge := callbackRequest(t, handler, "/api/open/video/v1/provider-callbacks/minimax/"+accountID.String(), `{"challenge":"verify-me"}`)
	if challenge.Code != http.StatusOK || challenge.Body.String() != `{"challenge":"verify-me"}`+"\n" {
		t.Fatalf("challenge=%d %s", challenge.Code, challenge.Body.String())
	}

	provider.event = videoprovider.CallbackEvent{EventID: "event-1", JobID: "job-1", Status: videoprovider.Status{JobID: "job-1", State: videoprovider.StateRunning}}
	accepted := callbackRequest(t, handler, "/api/open/video/v1/provider-callbacks/minimax/"+accountID.String(), `{"task":{"id":"job-1","status":"running"}}`)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("accepted=%d %s", accepted.Code, accepted.Body.String())
	}
	duplicate := callbackRequest(t, handler, "/api/open/video/v1/provider-callbacks/minimax/"+accountID.String(), `{"task":{"id":"job-1","status":"running"}}`)
	if duplicate.Code != http.StatusAccepted || !bytes.Contains(duplicate.Body.Bytes(), []byte(`"duplicate":true`)) {
		t.Fatalf("duplicate=%d %s", duplicate.Code, duplicate.Body.String())
	}
}

func callbackRequest(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

type routerCallbackProvider struct{ event videoprovider.CallbackEvent }

func (p *routerCallbackProvider) Submit(context.Context, videoprovider.Request) (videoprovider.Job, error) {
	return videoprovider.Job{}, nil
}
func (p *routerCallbackProvider) Get(context.Context, videoprovider.JobRef) (videoprovider.Status, error) {
	return videoprovider.Status{}, nil
}
func (p *routerCallbackProvider) Cancel(context.Context, videoprovider.JobRef) (videoprovider.CancelResult, error) {
	return videoprovider.CancelResult{}, nil
}
func (p *routerCallbackProvider) VerifyCallback(context.Context, http.Header, []byte) (videoprovider.CallbackEvent, error) {
	return p.event, nil
}
func (p *routerCallbackProvider) NormalizeUsage(videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{}, nil
}

type routerCallbackStore struct {
	provider videoprovider.Provider
	seen     map[string]struct{}
}

func (s *routerCallbackStore) ResolveProvider(context.Context, string, uuid.UUID) (videocallbackservice.ResolvedProvider, error) {
	return videocallbackservice.ResolvedProvider{ModelAccountID: 7, Provider: s.provider}, nil
}
func (s *routerCallbackStore) RecordEvent(_ context.Context, record videocallbackservice.EventRecord) (bool, error) {
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if _, ok := s.seen[record.ProviderEventID]; ok {
		return true, nil
	}
	s.seen[record.ProviderEventID] = struct{}{}
	return false, nil
}
