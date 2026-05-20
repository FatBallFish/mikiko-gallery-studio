package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Meta struct {
	RequestID string `json:"request_id,omitempty"`
}

type SuccessResponse struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta,omitempty"`
}

type ErrorResponse struct {
	Error errs.Error `json:"error"`
	Meta  Meta       `json:"meta,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, status int, data any) {
	WriteJSON(w, status, SuccessResponse{Data: data, Meta: Meta{RequestID: RequestIDFromContext(r.Context())}})
}

func WriteError(w http.ResponseWriter, r *http.Request, err *errs.Error) {
	if err == nil {
		err = errs.Internal("")
	}
	WriteJSON(w, err.StatusCode, ErrorResponse{Error: *err, Meta: Meta{RequestID: RequestIDFromContext(r.Context())}})
}
