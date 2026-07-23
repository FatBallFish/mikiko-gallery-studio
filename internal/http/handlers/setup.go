package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/setupui"
	"github.com/fatballfish/pic-gallery/internal/setup"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const (
	defaultSetupSessionTTL = 15 * time.Minute
	maxSetupRequestBytes   = 1 << 20
)

type setupAuth interface {
	Exchange(string, string) (string, error)
	VerifySession(string) error
}

type setupProber interface {
	ProbePostgres(context.Context, setup.PostgresProbeRequest) setup.ProbeResult
	ProbeRedis(context.Context, setup.RedisProbeRequest) setup.ProbeResult
	ProbeStorage(context.Context, setup.StorageProbeRequest) setup.ProbeResult
}

type setupApplication interface {
	Apply(context.Context, setup.ApplyRequest) (setup.OperationView, error)
	Progress(context.Context, string) (setup.OperationView, error)
	RecoveryOperationID() (string, error)
}

type SetupAPIOptions struct {
	System           *SystemAPI
	Bootstrap        config.BootstrapConfig
	Auth             setupAuth
	Prober           setupProber
	Application      setupApplication
	SessionTTL       time.Duration
	Now              func() time.Time
	OnRestartPending func()
}

type SetupAPI struct {
	system           *SystemAPI
	probeDraft       setupProbeDraft
	page             *setupui.Page
	auth             setupAuth
	prober           setupProber
	application      setupApplication
	sessionTTL       time.Duration
	now              func() time.Time
	onRestartPending func()
	restartOnce      sync.Once
}

type setupProbeDraft struct {
	postgresManaged      bool
	redisManaged         bool
	objectStorageManaged bool
	values               map[string]string
}

func NewSetupAPI(options SetupAPIOptions) (*SetupAPI, error) {
	if options.System == nil || options.Auth == nil || options.Prober == nil || options.Application == nil {
		return nil, errors.New("setup HTTP dependencies are incomplete")
	}
	if options.SessionTTL == 0 {
		options.SessionTTL = defaultSetupSessionTTL
	}
	if options.SessionTTL < time.Minute || options.SessionTTL > 24*time.Hour {
		return nil, errors.New("setup session TTL is invalid")
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	pageModel, err := setupui.NewModel(config.DefaultRuntimeSchema(), options.Bootstrap)
	if err != nil {
		return nil, fmt.Errorf("initialize setup page model: %w", err)
	}
	page, err := setupui.NewPage(pageModel)
	if err != nil {
		return nil, fmt.Errorf("initialize setup page: %w", err)
	}
	return &SetupAPI{
		system: options.System, probeDraft: newSetupProbeDraft(options.Bootstrap), page: page, auth: options.Auth, prober: options.Prober,
		application: options.Application, sessionTTL: options.SessionTTL, now: options.Now,
		onRestartPending: options.OnRestartPending,
	}, nil
}

func (api *SetupAPI) System() *SystemAPI { return api.system }

func (api *SetupAPI) HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	api.page.ServeHTTP(w, r)
}

func (api *SetupAPI) HandleSession(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token string `json:"token"`
	}
	if !decodeSetupJSON(w, r, &request) {
		return
	}
	session, err := api.auth.Exchange(directClientIP(r.RemoteAddr), request.Token)
	request.Token = ""
	if err != nil {
		writeSetupAuthError(w, r, err)
		return
	}
	operationID, err := api.application.RecoveryOperationID()
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, "SETUP_INTERNAL_ERROR", "setup recovery state could not be loaded"))
		return
	}
	cookie, err := setup.NewSetupSessionCookie(session, api.now(), api.sessionTTL, api.secureSessionCookie(r))
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, "SETUP_INTERNAL_ERROR", "setup session could not be created"))
		return
	}
	http.SetCookie(w, cookie)
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, struct {
		OperationID string `json:"operation_id,omitempty"`
	}{OperationID: operationID})
}

func (api *SetupAPI) secureSessionCookie(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	parsed, err := url.Parse(api.system.BootstrapStatus().SetupURL)
	return err == nil && parsed.Scheme == "https"
}

func (api *SetupAPI) HandleDatabaseProbe(w http.ResponseWriter, r *http.Request) {
	if !api.requireSession(w, r) {
		return
	}
	var request setup.PostgresProbeRequest
	if !decodeSetupJSON(w, r, &request) {
		return
	}
	databaseURL, ok := api.resolveManagedProbeValue(w, r, api.probeDraft.postgresManaged, "DATABASE_URL", request.DatabaseURL)
	if !ok {
		request.DatabaseURL = ""
		return
	}
	request.DatabaseURL = databaseURL
	result := api.prober.ProbePostgres(r.Context(), request)
	request.DatabaseURL = ""
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (api *SetupAPI) HandleRedisProbe(w http.ResponseWriter, r *http.Request) {
	if !api.requireSession(w, r) {
		return
	}
	var request setup.RedisProbeRequest
	if !decodeSetupJSON(w, r, &request) {
		return
	}
	redisURL, ok := api.resolveManagedProbeValue(w, r, api.probeDraft.redisManaged, "REDIS_URL", request.RedisURL)
	if !ok {
		request.RedisURL = ""
		return
	}
	request.RedisURL = redisURL
	result := api.prober.ProbeRedis(r.Context(), request)
	request.RedisURL = ""
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

type setupStorageProbeRequest struct {
	Driver          string `json:"driver"`
	LocalRoot       string `json:"local_root"`
	PublicBaseURL   string `json:"public_base_url"`
	SharedVolume    bool   `json:"shared_volume"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	ForcePathStyle  bool   `json:"force_path_style"`
	Prefix          string `json:"prefix"`
}

func (api *SetupAPI) HandleStorageProbe(w http.ResponseWriter, r *http.Request) {
	if !api.requireSession(w, r) {
		return
	}
	var request setupStorageProbeRequest
	if !decodeSetupJSON(w, r, &request) {
		return
	}
	if api.probeDraft.objectStorageManaged && !api.resolveManagedStorageProbe(w, r, &request) {
		request.AccessKeyID, request.SecretAccessKey = "", ""
		return
	}
	probeRequest := setup.StorageProbeRequest{Config: config.StorageConfig{
		Driver: request.Driver, LocalRoot: request.LocalRoot, PublicBaseURL: request.PublicBaseURL, SharedVolume: request.SharedVolume,
		S3: config.StorageS3Config{
			Endpoint: request.Endpoint, Region: request.Region, Bucket: request.Bucket,
			AccessKeyID: request.AccessKeyID, SecretAccessKey: request.SecretAccessKey,
			ForcePathStyle: request.ForcePathStyle, Prefix: request.Prefix,
		},
	}}
	result := api.prober.ProbeStorage(r.Context(), probeRequest)
	request.AccessKeyID, request.SecretAccessKey = "", ""
	probeRequest.Config.S3.AccessKeyID, probeRequest.Config.S3.SecretAccessKey = "", ""
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (api *SetupAPI) resolveManagedProbeValue(w http.ResponseWriter, r *http.Request, managed bool, key, submitted string) (string, bool) {
	if !managed {
		return submitted, true
	}
	configured := api.probeDraft.values[key]
	if submitted != "" && submitted != configured {
		writeSetupProbeDraftError(w, r)
		return "", false
	}
	return configured, true
}

func (api *SetupAPI) resolveManagedStorageProbe(w http.ResponseWriter, r *http.Request, request *setupStorageProbeRequest) bool {
	stringsByKey := []struct {
		key   string
		value *string
	}{
		{key: "STORAGE_DRIVER", value: &request.Driver},
		{key: "STORAGE_S3_ENDPOINT", value: &request.Endpoint},
		{key: "STORAGE_S3_REGION", value: &request.Region},
		{key: "STORAGE_S3_BUCKET", value: &request.Bucket},
		{key: "STORAGE_S3_ACCESS_KEY_ID", value: &request.AccessKeyID},
		{key: "STORAGE_S3_SECRET_ACCESS_KEY", value: &request.SecretAccessKey},
		{key: "STORAGE_S3_PREFIX", value: &request.Prefix},
	}
	for _, field := range stringsByKey {
		resolved, ok := api.resolveManagedProbeValue(w, r, true, field.key, *field.value)
		if !ok {
			return false
		}
		*field.value = resolved
	}
	configuredForcePathStyle, err := strconv.ParseBool(api.probeDraft.values["STORAGE_S3_FORCE_PATH_STYLE"])
	if err != nil || request.ForcePathStyle != configuredForcePathStyle {
		writeSetupProbeDraftError(w, r)
		return false
	}
	return true
}

func writeSetupProbeDraftError(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_VALIDATION_FAILED", "managed setup configuration cannot be overridden"))
}

func newSetupProbeDraft(bootstrap config.BootstrapConfig) setupProbeDraft {
	draft := setupProbeDraft{
		postgresManaged: bootstrap.PostgresManaged, redisManaged: bootstrap.RedisManaged,
		objectStorageManaged: bootstrap.ObjectStorageManaged, values: make(map[string]string, 9),
	}
	keys := make([]string, 0, 9)
	if draft.postgresManaged {
		keys = append(keys, "DATABASE_URL")
	}
	if draft.redisManaged {
		keys = append(keys, "REDIS_URL")
	}
	if draft.objectStorageManaged {
		keys = append(keys, "STORAGE_DRIVER", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET",
			"STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX")
	}
	for _, key := range keys {
		draft.values[key] = bootstrap.Values[key]
	}
	return draft
}

func (api *SetupAPI) HandleApply(w http.ResponseWriter, r *http.Request) {
	if !api.requireSession(w, r) {
		return
	}
	var request setup.ApplyRequest
	if !decodeSetupJSON(w, r, &request) {
		return
	}
	api.system.SetBootstrapStatus(BootstrapStatus{
		Phase: BootstrapPhaseInitializing, OperationID: request.OperationID,
		SetupURL: api.system.BootstrapStatus().SetupURL, RetryAfterSeconds: 2,
	})
	view, err := api.application.Apply(r.Context(), request)
	request.AdminPassword = ""
	for key := range request.Runtime {
		request.Runtime[key] = ""
	}
	if err != nil {
		api.system.SetBootstrapStatus(BootstrapStatus{Phase: BootstrapPhaseSetupRequired, RetryAfterSeconds: 2})
		writeSetupServiceError(w, r, err)
		return
	}
	phase := BootstrapPhaseInitializing
	if view.Phase == setup.OperationPhaseRestartPending || view.Phase == setup.OperationPhaseComplete {
		phase = BootstrapPhaseRestartPending
	}
	api.system.SetBootstrapStatus(BootstrapStatus{
		Phase: phase, OperationID: view.OperationID, RetryAfterSeconds: 2,
	})
	httpx.WriteSuccess(w, r, http.StatusAccepted, view)
	if view.Phase == setup.OperationPhaseRestartPending && api.onRestartPending != nil {
		_ = http.NewResponseController(w).Flush()
		api.restartOnce.Do(api.onRestartPending)
	}
}

func (api *SetupAPI) HandleProgress(w http.ResponseWriter, r *http.Request) {
	if !api.requireSession(w, r) {
		return
	}
	operationID := strings.TrimSpace(r.PathValue("operation_id"))
	if operationID == "" {
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_VALIDATION_FAILED", "setup operation id is required"))
		return
	}
	view, err := api.application.Progress(r.Context(), operationID)
	if err != nil {
		writeSetupServiceError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, view)
}

func (api *SetupAPI) requireSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(setup.SetupSessionCookieName)
	if err != nil || api.auth.VerifySession(cookie.Value) != nil {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, "SETUP_SESSION_INVALID", "setup session is invalid"))
		return false
	}
	return true
}

func decodeSetupJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxSetupRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_INVALID_JSON", "invalid setup request"))
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_INVALID_JSON", "invalid setup request"))
		return false
	}
	return true
}

func directClientIP(remoteAddress string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddress)); err == nil {
		if address, parseErr := netip.ParseAddr(host); parseErr == nil {
			return address.Unmap().String()
		}
	}
	if address, err := netip.ParseAddr(strings.TrimSpace(remoteAddress)); err == nil {
		return address.Unmap().String()
	}
	return ""
}

func writeSetupAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, setup.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		httpx.WriteError(w, r, errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "setup authentication is temporarily rate limited"))
	case errors.Is(err, setup.ErrCompleted):
		httpx.WriteError(w, r, errs.New(http.StatusConflict, "SETUP_COMPLETED", "setup is already completed"))
	case errors.Is(err, setup.ErrEntropy), errors.Is(err, setup.ErrClock), errors.Is(err, setup.ErrInvalidConfiguration):
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, "SETUP_INTERNAL_ERROR", "setup authentication failed"))
	default:
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, "SETUP_CREDENTIALS_INVALID", "setup credentials are invalid"))
	}
}

func writeSetupServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		httpx.WriteError(w, r, errs.New(http.StatusRequestTimeout, "SETUP_REQUEST_CANCELLED", "setup request was cancelled"))
	case errors.Is(err, context.DeadlineExceeded):
		httpx.WriteError(w, r, errs.New(http.StatusGatewayTimeout, "SETUP_REQUEST_TIMEOUT", "setup request timed out"))
	case errors.Is(err, setup.ErrSetupValidation):
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_VALIDATION_FAILED", "setup request validation failed"))
	case errors.Is(err, setup.ErrSetupProbe):
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, "SETUP_PROBE_FAILED", "setup middleware verification failed"))
	case errors.Is(err, setup.ErrSetupOperationGone):
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, "SETUP_OPERATION_NOT_FOUND", "setup operation was not found"))
	case errors.Is(err, setup.ErrSetupOperationConflict), errors.Is(err, setup.ErrSetupBindingMismatch):
		httpx.WriteError(w, r, errs.New(http.StatusConflict, "SETUP_OPERATION_CONFLICT", "another setup operation is already active"))
	case errors.Is(err, setup.ErrFirstAdminConflict):
		httpx.WriteError(w, r, errs.New(http.StatusConflict, "SETUP_FIRST_ADMIN_CONFLICT", "the first administrator already exists"))
	default:
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, "SETUP_INTERNAL_ERROR", "setup operation failed"))
	}
}
