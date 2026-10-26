package openplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"testing"
	"time"
)

type testUsers struct{ user domainauth.User }

func (u *testUsers) GetUserByID(id int64) (domainauth.User, bool) { return u.user, u.user.ID == id }
func TestProjectOperationErrorsPreserveLifecycleSemantics(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{&projectservice.NonEmptyError{Counts: domainproject.OwnershipCounts{Assets: 1}}, 409, errs.CodeProjectNotEmpty},
		{projectservice.ErrDefaultImmutable, 403, errs.CodeDefaultProjectImmutable},
		{projectservice.ErrProjectChanged, 409, errs.CodeProjectChanged},
		{projectservice.ErrCanvasBusy, 409, "CANVAS_BUSY"},
		{projectservice.ErrNameConflict, 409, errs.CodeConflict},
		{projectservice.ErrIdempotencyConflict, 409, errs.CodeConflict},
		{projectservice.ErrNotFound, 404, errs.CodeNotFound},
		{projectservice.ErrInvalid, 400, errs.CodeBadRequest},
	} {
		t.Run(tc.code+tc.err.Error(), func(t *testing.T) {
			var app *errs.Error
			if err := projectOperationError(fmt.Errorf("wrapped: %w", tc.err)); !errors.As(err, &app) || app.StatusCode != tc.status || app.Code != tc.code {
				t.Fatalf("mapped error = %#v, want %d %s", err, tc.status, tc.code)
			}
			if tc.code == errs.CodeProjectNotEmpty && app.Details["counts"] != (domainproject.OwnershipCounts{Assets: 1}) {
				t.Fatalf("missing ownership counts: %#v", app.Details)
			}
		})
	}
	unexpected := errors.New("database unavailable")
	if projectOperationError(unexpected) != unexpected {
		t.Fatal("unexpected failure changed")
	}
	app := errs.BadRequest("invalid upload field")
	if projectOperationError(app) != app {
		t.Fatal("existing media/group typed error changed")
	}
}

func TestOpenPlatformScopesAndCurrentAccount(t *testing.T) {
	users := &testUsers{domainauth.User{ID: 7, Status: "active", Email: "test@example.com"}}
	s := New(Dependencies{Auth: users})
	p := Principal{UserID: 7}
	if _, err := s.Execute(context.Background(), p, "get_current_user", nil); err == nil {
		t.Fatal("missing scope accepted")
	}
	p.Scopes = []string{"account:read"}
	out, err := s.Execute(context.Background(), p, "get_current_user", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	if _, ok := payload["PasswordHash"]; ok {
		t.Fatal("secret user fields exposed")
	}
	users.user.Status = "disabled"
	if _, err = s.Execute(context.Background(), p, "get_current_user", nil); err == nil {
		t.Fatal("disabled user accepted")
	}
	users.user.Status = "active"
	sentinel := errors.New("revoked grant")
	s.deps.ValidatePrincipal = func(context.Context, Principal) error { return sentinel }
	if _, err = s.Execute(context.Background(), p, "get_current_user", nil); !errors.Is(err, sentinel) {
		t.Fatalf("live revocation ignored: %v", err)
	}
}
func TestOpenPlatformQuotesBindIdentityRequestPriceAndExpiry(t *testing.T) {
	now := time.Unix(12345, 0)
	s := New(Dependencies{QuoteKey: []byte("01234567890123456789012345678901"), Now: func() time.Time { return now }})
	p := Principal{UserID: 7, APIKeyID: "42", Scopes: []string{"images:write"}}
	request := map[string]any{"prompt": "one", "price": "1.00000"}
	token, _, err := s.signQuote(p, request)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.verifyQuote(p, request, token); err != nil {
		t.Fatal(err)
	}
	other := p
	other.APIKeyID = "43"
	if s.verifyQuote(other, request, token) == nil {
		t.Fatal("cross-key quote accepted")
	}
	if s.verifyQuote(p, map[string]any{"prompt": "two", "price": "1.00000"}, token) == nil {
		t.Fatal("changed prompt accepted")
	}
	if s.verifyQuote(p, map[string]any{"prompt": "one", "price": "2.00000"}, token) == nil {
		t.Fatal("changed price accepted")
	}
	now = now.Add(120 * time.Second)
	if s.verifyQuote(p, request, token) == nil {
		t.Fatal("expired quote accepted")
	}
}
func TestOpenPlatformStrictInputAndCatalogue(t *testing.T) {
	var in ImageInput
	if decode(json.RawMessage(`{"user_id":99}`), &in) == nil {
		t.Fatal("actor override field accepted")
	}
	if decode(json.RawMessage(`{} {}`), &in) == nil {
		t.Fatal("trailing input accepted")
	}
	seen := map[string]bool{}
	for _, spec := range Specs() {
		if seen[spec.Name] {
			t.Fatalf("duplicate %s", spec.Name)
		}
		seen[spec.Name] = true
		if spec.Scope == "" || spec.InputSchema["type"] != "object" {
			t.Fatalf("invalid spec %s", spec.Name)
		}
	}
	p := Principal{UserID: 1, APIKeyID: "1"}
	other := p
	other.APIKeyID = "2"
	if taskID(p, "same") == taskID(other, "same") {
		t.Fatal("key namespaces collide")
	}
	_, err := keyID(Principal{APIKeyID: "invalid"})
	var app *errs.Error
	if !errors.As(err, &app) {
		t.Fatal("invalid key identity accepted")
	}
}

func TestOpenPlatformImageReplayIgnoresAccountPricingAndQuoteChanges(t *testing.T) {
	ctx := context.Background()
	users := &testUsers{domainauth.User{ID: 7, Status: "active", GroupCode: "old", GroupCodes: []string{"old"}, GroupMultiplier: "1.00000"}}
	p := Principal{UserID: 7, APIKeyID: "42", Scopes: []string{"images:write"}}
	in := ImageInput{ProjectID: "project", RouteModelCode: "route", Prompt: "same business request", IdempotencyKey: "retry", CapabilityVersion: "old-version"}
	id := taskID(p, in.IdempotencyKey)
	store := imagetask.NewMemoryStore()
	if err := store.Save(ctx, image.Task{ID: id, UserID: 7, ProjectID: "project", Status: "queued", RequestFingerprint: imageInputFingerprint(in), EstimatedPoints: "1.00000"}); err != nil {
		t.Fatal(err)
	}
	// No Billing, QuoteKey, model routing, reference loader, or quote is supplied:
	// any accidental preparation/reservation on replay necessarily fails.
	s := New(Dependencies{Auth: users, Images: imagetask.NewServiceWithStore(config.Config{}, store)})
	users.user.GroupCode = "new"
	users.user.GroupCodes = []string{"new"}
	users.user.GroupMultiplier = "99.00000"
	in.CapabilityVersion = "new-version"
	in.QuoteToken = "expired"
	raw, _ := json.Marshal(in)
	result, err := s.Execute(ctx, p, "create_image_task", raw)
	if err != nil {
		t.Fatalf("persisted retry checked current pricing/quote: %v", err)
	}
	task, ok := result.(image.Task)
	if !ok || task.ID != id || task.EstimatedPoints != "1.00000" {
		t.Fatalf("wrong replay %#v", result)
	}
	in.Prompt = "different business request"
	raw, _ = json.Marshal(in)
	if _, err = s.Execute(ctx, p, "create_image_task", raw); err == nil {
		t.Fatal("changed business input accepted")
	}
}
