package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type setupAuthStub struct {
	exchangedIP    string
	exchangedToken string
	session        string
	exchangeErr    error
	verifyErr      error
}

func (stub *setupAuthStub) Exchange(ip, token string) (string, error) {
	stub.exchangedIP, stub.exchangedToken = ip, token
	return stub.session, stub.exchangeErr
}

func (stub *setupAuthStub) VerifySession(string) error { return stub.verifyErr }

type setupProbeStub struct {
	database setup.PostgresProbeRequest
	redis    setup.RedisProbeRequest
	storage  setup.StorageProbeRequest
}

func (stub *setupProbeStub) ProbePostgres(_ context.Context, request setup.PostgresProbeRequest) setup.ProbeResult {
	stub.database = request
	return setup.ProbeResult{Kind: "database", Success: true, Code: setup.ProbeCodeOK}
}

func (stub *setupProbeStub) ProbeRedis(_ context.Context, request setup.RedisProbeRequest) setup.ProbeResult {
	stub.redis = request
	return setup.ProbeResult{Kind: "redis", Success: true, Code: setup.ProbeCodeOK}
}

func (stub *setupProbeStub) ProbeStorage(_ context.Context, request setup.StorageProbeRequest) setup.ProbeResult {
	stub.storage = request
	return setup.ProbeResult{Kind: "storage", Success: true, Code: setup.ProbeCodeOK}
}

type setupApplicationStub struct {
	request setup.ApplyRequest
	view    setup.OperationView
	err     error
}

func (stub *setupApplicationStub) Apply(_ context.Context, request setup.ApplyRequest) (setup.OperationView, error) {
	stub.request = request
	return stub.view, stub.err
}

func (stub *setupApplicationStub) Progress(_ context.Context, _ string) (setup.OperationView, error) {
	return stub.view, stub.err
}

func TestBootstrapStatusUsesTrustedPublicAPIURLAndNeverRequestHost(t *testing.T) {
	system := NewSystemAPI(BootstrapStatus{
		Phase:             handlersBootstrapPhaseForTest(),
		PublicAPIURL:      "https://api.example.test/base",
		RetryAfterSeconds: 2,
	})

	request := httptest.NewRequest(http.MethodGet, "http://attacker.invalid/api/system/v1/bootstrap-status", nil)
	request.Host = "attacker.invalid\r\nX-Injected: yes"
	request.Header.Set("X-Forwarded-Host", "forwarded-attacker.invalid")
	recorder := httptest.NewRecorder()
	system.HandleBootstrapStatus(recorder, request)

	status := decodeSuccessData[BootstrapStatus](t, recorder)
	if status.SetupURL != "https://api.example.test/setup" {
		t.Fatalf("setup_url = %q, want trusted public API origin", status.SetupURL)
	}
	if strings.Contains(recorder.Body.String(), "attacker") || strings.Contains(recorder.Body.String(), "Injected") {
		t.Fatalf("bootstrap response trusted request host headers: %s", recorder.Body.String())
	}
}

func TestBootstrapStatusUsesRelativeFallbackWithoutTrustedPublicAPIURL(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		requestURL string
		host       string
		forwarded  string
	}{
		{name: "wildcard docker listener", requestURL: "http://127.0.0.1:8080/api/system/v1/bootstrap-status", host: "0.0.0.0:8080"},
		{name: "remote IP", requestURL: "http://192.0.2.10:8080/api/system/v1/bootstrap-status", host: "192.0.2.10:8080"},
		{name: "independent frontend", requestURL: "https://user.example.test/api/system/v1/bootstrap-status", host: "user.example.test"},
		{name: "hostile proxy headers", requestURL: "http://internal-api/api/system/v1/bootstrap-status", host: "attacker.invalid", forwarded: "proxy-attacker.invalid"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			system := NewSystemAPI(BootstrapStatus{Phase: BootstrapPhaseSetupRequired})
			request := httptest.NewRequest(http.MethodGet, testCase.requestURL, nil)
			request.Host = testCase.host
			request.Header.Set("X-Forwarded-Host", testCase.forwarded)
			request.Header.Set("X-Forwarded-Proto", "https")
			recorder := httptest.NewRecorder()
			system.HandleBootstrapStatus(recorder, request)
			status := decodeSuccessData[BootstrapStatus](t, recorder)
			if status.SetupURL != "/setup" {
				t.Fatalf("setup_url = %q, want relative API-base fallback", status.SetupURL)
			}
			if recorder.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatalf("public bootstrap status CORS = %q, want *", recorder.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestBootstrapStatusIsSecretFree(t *testing.T) {
	secret := "setup-secret-that-must-not-leak"
	system := NewSystemAPI(BootstrapStatus{
		Phase:             BootstrapPhaseInitializing,
		OperationID:       "019d0000-0000-7000-8000-000000000001",
		RetryAfterSeconds: 2,
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/system/v1/bootstrap-status?token="+secret, nil)
	system.HandleBootstrapStatus(recorder, request)
	if strings.Contains(recorder.Body.String(), secret) || strings.Contains(recorder.Body.String(), "token") {
		t.Fatalf("bootstrap response exposed request secret: %s", recorder.Body.String())
	}
}

func TestSetupSessionUsesRemoteAddressAndSecureCookieOnlyForTLS(t *testing.T) {
	auth := &setupAuthStub{session: "signed-session"}
	api := newSetupAPIForHandlerTest(t, auth, &setupProbeStub{}, &setupApplicationStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/session", strings.NewReader(`{"token":"operator-token"}`))
	request.RemoteAddr = "192.0.2.10:4567"
	request.Header.Set("X-Forwarded-For", "203.0.113.20")
	recorder := httptest.NewRecorder()
	api.HandleSession(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("session status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
	if auth.exchangedIP != "192.0.2.10" || auth.exchangedToken != "operator-token" {
		t.Fatalf("Exchange = (%q, %q), want direct remote IP and supplied token", auth.exchangedIP, auth.exchangedToken)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != setup.SetupSessionCookieName || !cookies[0].HttpOnly || cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected setup session cookie: %#v", cookies)
	}
}

func TestSetupPageUsesEmbeddedAdminConsole(t *testing.T) {
	api := newSetupAPIForHandlerTest(t, &setupAuthStub{}, &setupProbeStub{}, &setupApplicationStub{})
	recorder := httptest.NewRecorder()
	api.HandleSetupPage(recorder, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "id=\"setup-console\"") {
		t.Fatalf("GET /setup did not serve embedded setup console: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Header().Get("Content-Security-Policy"), "unsafe-inline") {
		t.Fatalf("setup page CSP permits unsafe inline content: %q", recorder.Header().Get("Content-Security-Policy"))
	}
}

func TestManagedDatabaseProbeUsesServerDraftWithoutExposingItToPage(t *testing.T) {
	secretURL := "postgres://app:managed-secret@postgres/app?sslmode=disable"
	bootstrap := setupBootstrapForHandlerTest()
	bootstrap.PostgresManaged = true
	bootstrap.Values["POSTGRES_MANAGED"] = "true"
	bootstrap.Values["DATABASE_URL"] = secretURL
	prober := &setupProbeStub{}
	api := newSetupAPIForHandlerTestWithBootstrap(t, bootstrap, &setupAuthStub{}, prober, &setupApplicationStub{})

	pageRecorder := httptest.NewRecorder()
	api.HandleSetupPage(pageRecorder, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if strings.Contains(pageRecorder.Body.String(), secretURL) || strings.Contains(pageRecorder.Body.String(), "managed-secret") {
		t.Fatal("managed database URL leaked into embedded page")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/probes/database", strings.NewReader(`{"database_url":""}`))
	request.AddCookie(&http.Cookie{Name: setup.SetupSessionCookieName, Value: "valid-session"})
	recorder := httptest.NewRecorder()
	api.HandleDatabaseProbe(recorder, request)
	if recorder.Code != http.StatusOK || prober.database.DatabaseURL != secretURL {
		t.Fatalf("managed database probe status=%d request=%#v", recorder.Code, prober.database)
	}
	if strings.Contains(recorder.Body.String(), secretURL) || strings.Contains(recorder.Body.String(), "managed-secret") {
		t.Fatal("managed database probe response leaked configured URL")
	}
}

func TestManagedProbeRejectsBrowserOverride(t *testing.T) {
	bootstrap := setupBootstrapForHandlerTest()
	bootstrap.RedisManaged = true
	bootstrap.Values["REDIS_MANAGED"] = "true"
	bootstrap.Values["REDIS_URL"] = "redis://:managed-secret@redis:6379/0"
	prober := &setupProbeStub{}
	api := newSetupAPIForHandlerTestWithBootstrap(t, bootstrap, &setupAuthStub{}, prober, &setupApplicationStub{})
	request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/probes/redis", strings.NewReader(`{"redis_url":"redis://attacker:6379/0","key_prefix":"app"}`))
	request.AddCookie(&http.Cookie{Name: setup.SetupSessionCookieName, Value: "valid-session"})
	recorder := httptest.NewRecorder()
	api.HandleRedisProbe(recorder, request)
	if recorder.Code != http.StatusBadRequest || prober.redis.RedisURL != "" {
		t.Fatalf("managed Redis override status=%d reachedProbe=%#v body=%s", recorder.Code, prober.redis, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "managed-secret") || strings.Contains(recorder.Body.String(), "attacker") {
		t.Fatalf("managed override error leaked connection material: %s", recorder.Body.String())
	}
}

func TestSetupAPIRetainsOnlyManagedProbeDraftValues(t *testing.T) {
	bootstrap := setupBootstrapForHandlerTest()
	bootstrap.PostgresManaged = true
	bootstrap.Values["DATABASE_URL"] = "postgres://app:managed-secret@postgres/app"
	bootstrap.Values["SETUP_TOKEN"] = "setup-token-not-needed-by-http-page"
	bootstrap.Values["AUTH_ACCESS_TOKEN_SECRET"] = "application-secret-not-needed-by-http-page"
	api := newSetupAPIForHandlerTestWithBootstrap(t, bootstrap, &setupAuthStub{}, &setupProbeStub{}, &setupApplicationStub{})
	if api.probeDraft.values["DATABASE_URL"] == "" {
		t.Fatal("managed probe draft lost the database URL required for server-side probing")
	}
	for _, forbidden := range []string{"SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET"} {
		if _, exists := api.probeDraft.values[forbidden]; exists {
			t.Fatalf("SetupAPI retained unrelated secret %s", forbidden)
		}
	}
}

func TestSetupSessionCookieUsesTrustedHTTPSPublicAPIURLBehindProxy(t *testing.T) {
	auth := &setupAuthStub{session: "signed-session"}
	api, err := NewSetupAPI(SetupAPIOptions{
		System: NewSystemAPI(BootstrapStatus{
			Phase:        BootstrapPhaseSetupRequired,
			PublicAPIURL: "https://api.example.test",
		}),
		Bootstrap: setupBootstrapForHandlerTest(),
		Auth:      auth, Prober: &setupProbeStub{}, Application: &setupApplicationStub{},
	})
	if err != nil {
		t.Fatalf("NewSetupAPI: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://internal-api/api/setup/v1/session", strings.NewReader(`{"token":"operator-token"}`))
	request.RemoteAddr = "192.0.2.10:4567"
	recorder := httptest.NewRecorder()
	api.HandleSession(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted HTTPS public API must issue Secure setup cookie: %#v", cookies)
	}
}

func TestSetupWriteHandlersRequireValidSession(t *testing.T) {
	auth := &setupAuthStub{verifyErr: setup.ErrInvalidSession}
	application := &setupApplicationStub{}
	api := newSetupAPIForHandlerTest(t, auth, &setupProbeStub{}, application)

	request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/apply", strings.NewReader(`{"operation_id":"op","admin_password":"secret"}`))
	recorder := httptest.NewRecorder()
	api.HandleApply(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("apply without session status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if application.request.OperationID != "" {
		t.Fatal("unauthenticated apply reached setup service")
	}
}

func TestSetupApplyPropagatesContextAndReturnsSanitizedStableError(t *testing.T) {
	secret := "database-password-that-must-not-leak"
	application := &setupApplicationStub{err: errors.New("dial postgres with " + secret)}
	api := newSetupAPIForHandlerTest(t, &setupAuthStub{}, &setupProbeStub{}, application)
	request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/apply", strings.NewReader(`{
		"operation_id":"019d0000-0000-7000-8000-000000000001",
		"runtime":{"DATABASE_URL":"postgres://app:`+secret+`@db/app"},
		"admin_email":"admin@example.test",
		"admin_password":"admin-password"
	}`))
	request.AddCookie(&http.Cookie{Name: setup.SetupSessionCookieName, Value: "valid-session"})
	recorder := httptest.NewRecorder()
	api.HandleApply(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("apply error status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), secret) || strings.Contains(recorder.Body.String(), "postgres://") {
		t.Fatalf("apply error leaked secret: %s", recorder.Body.String())
	}
}

func TestSetupApplyFlushesAcceptedResponseBeforeOneRestartSignal(t *testing.T) {
	application := &setupApplicationStub{view: setup.OperationView{
		OperationID: "019d0000-0000-7000-8000-000000000001",
		Phase:       setup.OperationPhaseRestartPending,
	}}
	var callbackCount atomic.Int32
	var flushedBeforeCallback atomic.Bool
	recorder := httptest.NewRecorder()
	api, err := NewSetupAPI(SetupAPIOptions{
		System:    NewSystemAPI(BootstrapStatus{Phase: BootstrapPhaseSetupRequired}),
		Bootstrap: setupBootstrapForHandlerTest(),
		Auth:      &setupAuthStub{}, Prober: &setupProbeStub{}, Application: application,
		OnRestartPending: func() {
			callbackCount.Add(1)
			flushedBeforeCallback.Store(recorder.Flushed && recorder.Code == http.StatusAccepted && recorder.Body.Len() > 0)
		},
	})
	if err != nil {
		t.Fatalf("NewSetupAPI: %v", err)
	}
	requestBody := `{"operation_id":"019d0000-0000-7000-8000-000000000001","runtime":{},"admin_email":"admin@example.test","admin_password":"password"}`
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/apply", strings.NewReader(requestBody))
		request.AddCookie(&http.Cookie{Name: setup.SetupSessionCookieName, Value: "valid-session"})
		api.HandleApply(recorder, request)
	}
	if callbackCount.Load() != 1 {
		t.Fatalf("restart callback count = %d, want 1", callbackCount.Load())
	}
	if !flushedBeforeCallback.Load() {
		t.Fatal("restart callback ran before accepted response was written and flushed")
	}
}

func TestSetupStableErrorResponseTable(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		writer     func(http.ResponseWriter, *http.Request, error)
		wantStatus int
		wantCode   string
	}{
		{name: "auth invalid", err: setup.ErrInvalidToken, writer: writeSetupAuthError, wantStatus: 401, wantCode: "SETUP_CREDENTIALS_INVALID"},
		{name: "auth rate limited", err: setup.ErrRateLimited, writer: writeSetupAuthError, wantStatus: 429, wantCode: "RATE_LIMITED"},
		{name: "auth completed", err: setup.ErrCompleted, writer: writeSetupAuthError, wantStatus: 409, wantCode: "SETUP_COMPLETED"},
		{name: "auth internal", err: setup.ErrClock, writer: writeSetupAuthError, wantStatus: 500, wantCode: "SETUP_INTERNAL_ERROR"},
		{name: "request cancelled", err: context.Canceled, writer: writeSetupServiceError, wantStatus: 408, wantCode: "SETUP_REQUEST_CANCELLED"},
		{name: "request timeout", err: context.DeadlineExceeded, writer: writeSetupServiceError, wantStatus: 504, wantCode: "SETUP_REQUEST_TIMEOUT"},
		{name: "validation", err: setup.ErrSetupValidation, writer: writeSetupServiceError, wantStatus: 400, wantCode: "SETUP_VALIDATION_FAILED"},
		{name: "probe", err: setup.ErrSetupProbe, writer: writeSetupServiceError, wantStatus: 400, wantCode: "SETUP_PROBE_FAILED"},
		{name: "gone", err: setup.ErrSetupOperationGone, writer: writeSetupServiceError, wantStatus: 404, wantCode: "SETUP_OPERATION_NOT_FOUND"},
		{name: "conflict", err: setup.ErrSetupOperationConflict, writer: writeSetupServiceError, wantStatus: 409, wantCode: "SETUP_OPERATION_CONFLICT"},
		{name: "binding mismatch", err: setup.ErrSetupBindingMismatch, writer: writeSetupServiceError, wantStatus: 409, wantCode: "SETUP_OPERATION_CONFLICT"},
		{name: "first administrator conflict", err: setup.ErrFirstAdminConflict, writer: writeSetupServiceError, wantStatus: 409, wantCode: "SETUP_FIRST_ADMIN_CONFLICT"},
		{name: "internal", err: setup.ErrSetupCommit, writer: writeSetupServiceError, wantStatus: 500, wantCode: "SETUP_INTERNAL_ERROR"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/setup/v1/apply", nil)
			testCase.writer(recorder, request, testCase.err)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status=%d, want %d", recorder.Code, testCase.wantStatus)
			}
			var response httpx.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Error.Code != testCase.wantCode {
				t.Fatalf("code=%q, want %q", response.Error.Code, testCase.wantCode)
			}
		})
	}
}

func newSetupAPIForHandlerTest(t *testing.T, auth setupAuth, prober setupProber, application setupApplication) *SetupAPI {
	t.Helper()
	return newSetupAPIForHandlerTestWithBootstrap(t, setupBootstrapForHandlerTest(), auth, prober, application)
}

func newSetupAPIForHandlerTestWithBootstrap(t *testing.T, bootstrap config.BootstrapConfig, auth setupAuth, prober setupProber, application setupApplication) *SetupAPI {
	t.Helper()
	api, err := NewSetupAPI(SetupAPIOptions{
		System:    NewSystemAPI(BootstrapStatus{Phase: BootstrapPhaseSetupRequired}),
		Bootstrap: bootstrap,
		Auth:      auth, Prober: prober, Application: application,
		SessionTTL: 15 * time.Minute,
		Now:        func() time.Time { return time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewSetupAPI: %v", err)
	}
	return api
}

func setupBootstrapForHandlerTest() config.BootstrapConfig {
	values := map[string]string{
		"DEPLOYMENT_MODE": "docker", "DEPLOYMENT_PROFILE": "core", "DEPLOYMENT_TOPOLOGY": "single", "DEPLOYMENT_ROLE": "single",
		"POSTGRES_MANAGED": "false", "REDIS_MANAGED": "false", "OBJECT_STORAGE_MANAGED": "false",
		"STORAGE_DRIVER": "local", "STORAGE_LOCAL_ROOT": "./data/storage", "STORAGE_SHARED_VOLUME": "true", "REDIS_KEY_PREFIX": "app",
	}
	return config.BootstrapConfig{
		SchemaVersion: config.CurrentRuntimeSchemaVersion,
		Deployment:    config.DeploymentContext{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle, StorageDriver: "local"},
		Values:        values,
	}
}

func handlersBootstrapPhaseForTest() BootstrapPhase { return BootstrapPhaseSetupRequired }

func decodeSuccessData[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope httpx.SuccessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode success envelope: %v; body=%s", err, recorder.Body.String())
	}
	encoded, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatalf("encode success data: %v", err)
	}
	var result T
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode success data: %v", err)
	}
	return result
}
