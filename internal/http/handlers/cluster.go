package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const maxClusterAdminBodyBytes = 16 << 10

func (a *API) HandleAdminClusterTokens(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.cluster == nil {
		httpx.WriteError(w, r, errs.Internal("cluster service is not configured"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, queryErr := parsePositiveIntQuery(r, "page", 1)
		if queryErr != nil {
			httpx.WriteError(w, r, queryErr)
			return
		}
		pageSize, queryErr := parsePositiveIntQuery(r, "page_size", 20)
		if queryErr != nil {
			httpx.WriteError(w, r, queryErr)
			return
		}
		result, err := a.cluster.ListTokens(r.Context(), domaincluster.ListTokensRequest{
			Page: page, PageSize: pageSize, Role: domaincluster.JoinRole(strings.TrimSpace(r.URL.Query().Get("role"))),
			Status: strings.TrimSpace(r.URL.Query().Get("status")),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"items":      result.Items,
			"pagination": map[string]any{"page": result.Page, "page_size": result.PageSize, "total": result.Total},
		})
	case http.MethodPost:
		var request struct {
			Role       domaincluster.JoinRole `json:"role"`
			TTLSeconds int64                  `json:"ttl_seconds"`
		}
		if err := decodeClusterRequest(w, r, &request); err != nil {
			httpx.WriteError(w, r, errs.BadRequest(err.Error()))
			return
		}
		issued, err := a.cluster.CreateToken(r.Context(), domaincluster.CreateTokenRequest{
			Role: request.Role, TTL: time.Duration(request.TTLSeconds) * time.Second,
			ActorID: strconv.FormatInt(admin.AdminID, 10),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, issued)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAdminClusterTokenDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if a.cluster == nil {
		httpx.WriteError(w, r, errs.Internal("cluster service is not configured"))
		return
	}
	tokenID, ok := parseClusterTokenRevokePath(r.URL.Path)
	if !ok {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "cluster token route not found"))
		return
	}
	token, err := a.cluster.RevokeToken(r.Context(), tokenID, strconv.FormatInt(admin.AdminID, 10))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, token)
}

func (a *API) HandleClusterChallenge(w http.ResponseWriter, r *http.Request) {
	writeClusterProtocolUnavailable(w, r)
}

func (a *API) HandleClusterJoin(w http.ResponseWriter, r *http.Request) {
	writeClusterProtocolUnavailable(w, r)
}

func writeClusterProtocolUnavailable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	httpx.WriteError(w, r, errs.New(http.StatusNotImplemented, "CLUSTER_PROTOCOL_UNAVAILABLE", "encrypted cluster enrollment is not enabled"))
}

func parseClusterTokenRevokePath(requestPath string) (string, bool) {
	const prefix = "/api/ops/admin/v1/cluster/tokens/"
	value := strings.TrimPrefix(requestPath, prefix)
	if !strings.HasSuffix(value, ":revoke") {
		return "", false
	}
	tokenID := strings.TrimSuffix(value, ":revoke")
	if strings.Contains(tokenID, "/") {
		return "", false
	}
	if _, err := uuid.Parse(tokenID); err != nil {
		return "", false
	}
	return tokenID, true
}

func decodeClusterRequest(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxClusterAdminBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid JSON request body")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}
