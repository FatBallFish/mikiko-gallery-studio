package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type BootstrapPhase string

const (
	BootstrapPhaseSetupRequired  BootstrapPhase = "setup_required"
	BootstrapPhaseInitializing   BootstrapPhase = "initializing"
	BootstrapPhaseRestartPending BootstrapPhase = "restart_pending"
	BootstrapPhaseReady          BootstrapPhase = "ready"
	BootstrapPhaseBroken         BootstrapPhase = "broken"
)

type BootstrapStatus struct {
	Phase             BootstrapPhase `json:"phase"`
	SetupURL          string         `json:"setup_url,omitempty"`
	OperationID       string         `json:"operation_id,omitempty"`
	RetryAfterSeconds int            `json:"retry_after_seconds,omitempty"`
	DiagnosticCode    string         `json:"diagnostic_code,omitempty"`
	PublicAPIURL      string         `json:"-"`
	FallbackAPIURL    string         `json:"-"`
}

type SystemAPI struct {
	mu             sync.RWMutex
	status         BootstrapStatus
	publicAPIURL   string
	fallbackAPIURL string
}

func NewSystemAPI(status BootstrapStatus) *SystemAPI {
	publicAPIURL := status.PublicAPIURL
	fallbackAPIURL := status.FallbackAPIURL
	status = normalizeBootstrapStatus(status)
	return &SystemAPI{status: status, publicAPIURL: publicAPIURL, fallbackAPIURL: fallbackAPIURL}
}

func (api *SystemAPI) SetBootstrapStatus(status BootstrapStatus) {
	if api == nil {
		return
	}
	api.mu.Lock()
	if strings.TrimSpace(status.PublicAPIURL) == "" {
		status.PublicAPIURL = api.publicAPIURL
	} else {
		api.publicAPIURL = status.PublicAPIURL
	}
	if strings.TrimSpace(status.FallbackAPIURL) == "" {
		status.FallbackAPIURL = api.fallbackAPIURL
	} else {
		api.fallbackAPIURL = status.FallbackAPIURL
	}
	api.status = normalizeBootstrapStatus(status)
	api.mu.Unlock()
}

func (api *SystemAPI) BootstrapStatus() BootstrapStatus {
	if api == nil {
		return normalizeBootstrapStatus(BootstrapStatus{Phase: BootstrapPhaseBroken})
	}
	api.mu.RLock()
	defer api.mu.RUnlock()
	return api.status
}

func (api *SystemAPI) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "ok"})
}

func (api *SystemAPI) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	if api.BootstrapStatus().Phase != BootstrapPhaseReady {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, "NOT_READY", "normal business traffic is not ready"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "ready"})
}

func (api *SystemAPI) HandleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	httpx.WriteSuccess(w, r, http.StatusOK, api.BootstrapStatus())
}

func normalizeBootstrapStatus(status BootstrapStatus) BootstrapStatus {
	switch status.Phase {
	case BootstrapPhaseSetupRequired, BootstrapPhaseInitializing, BootstrapPhaseRestartPending, BootstrapPhaseReady, BootstrapPhaseBroken:
	default:
		status.Phase = BootstrapPhaseBroken
	}
	if status.RetryAfterSeconds < 0 {
		status.RetryAfterSeconds = 0
	}
	if status.Phase == BootstrapPhaseSetupRequired || status.Phase == BootstrapPhaseInitializing || status.Phase == BootstrapPhaseRestartPending {
		status.SetupURL = trustedSetupURL(status.PublicAPIURL, status.FallbackAPIURL)
	} else {
		status.SetupURL = ""
	}
	status.PublicAPIURL = ""
	status.FallbackAPIURL = ""
	return status
}

func trustedSetupURL(publicAPIURL, fallbackAPIURL string) string {
	for _, candidate := range []string{publicAPIURL, fallbackAPIURL, "http://127.0.0.1:8080"} {
		parsed, err := url.Parse(strings.TrimSpace(candidate))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			continue
		}
		return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: "/setup"}).String()
	}
	return "http://127.0.0.1:8080/setup"
}

type statusResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func Root(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "bootstrap-ready"})
}

func APINotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "api route not found"))
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	NewSystemAPI(BootstrapStatus{Phase: BootstrapPhaseReady}).HandleHealthz(w, r)
}

func Readyz(w http.ResponseWriter, r *http.Request) {
	NewSystemAPI(BootstrapStatus{Phase: BootstrapPhaseReady}).HandleReadyz(w, r)
}
