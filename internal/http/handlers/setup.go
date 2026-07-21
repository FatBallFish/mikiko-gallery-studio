package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
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
}

type SetupAPIOptions struct {
	System           *SystemAPI
	Auth             setupAuth
	Prober           setupProber
	Application      setupApplication
	SessionTTL       time.Duration
	Now              func() time.Time
	OnRestartPending func()
}

type SetupAPI struct {
	system           *SystemAPI
	auth             setupAuth
	prober           setupProber
	application      setupApplication
	sessionTTL       time.Duration
	now              func() time.Time
	onRestartPending func()
	restartOnce      sync.Once
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
	return &SetupAPI{
		system: options.System, auth: options.Auth, prober: options.Prober,
		application: options.Application, sessionTTL: options.SessionTTL, now: options.Now,
		onRestartPending: options.OnRestartPending,
	}, nil
}

func (api *SetupAPI) System() *SystemAPI { return api.system }

func (api *SetupAPI) HandleSetupPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Setup</title></head><body><main><h1>Setup required</h1><p>Use deployctl setup token show or deployctl setup token reset to retrieve setup access.</p></main></body></html>")
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
	cookie, err := setup.NewSetupSessionCookie(session, api.now(), api.sessionTTL, api.secureSessionCookie(r))
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, "SETUP_INTERNAL_ERROR", "setup session could not be created"))
		return
	}
	http.SetCookie(w, cookie)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
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
