package handlers

import (
	"net/http"

	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type statusResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func Root(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "bootstrap-ready"})
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "ok"})
}

func Readyz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, statusResponse{Name: "pic-gallery", Status: "ready"})
}
