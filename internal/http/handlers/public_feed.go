package handlers

import (
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	publicfeedservice "github.com/fatballfish/pic-gallery/internal/service/publicfeed"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"net/http"
)

func (a *API) SetPublicFeed(service *publicfeedservice.Service) { a.publicFeed = service }
func (a *API) HandlePublicFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	page, e := parsePositiveIntQuery(r, "page", 1)
	if e != nil {
		httpx.WriteError(w, r, e)
		return
	}
	size, e := parsePositiveIntQuery(r, "page_size", 20)
	if e != nil {
		httpx.WriteError(w, r, e)
		return
	}
	req := domain.Request{Page: page, PageSize: size, Sort: r.URL.Query().Get("sort"), Query: r.URL.Query().Get("query"), Model: r.URL.Query().Get("route_model_code"), Liked: parseBoolQuery(r, "liked"), Favorited: parseBoolQuery(r, "favorited")}
	if user := a.optionalUserWithQueryToken(r); user != nil {
		req.ViewerID = user.ID
	}
	result := domain.Page{Items: []domain.Item{}, Page: page, PageSize: size}
	if a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		if a.publicFeed == nil {
			httpx.WriteError(w, r, errs.New(503, errs.CodeInternal, "public feed unavailable"))
			return
		}
		var err error
		result, err = a.publicFeed.List(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
	}
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		if item.Image != nil {
			if a.tasks == nil {
				httpx.WriteError(w, r, errs.New(503, errs.CodeInternal, "public image projection unavailable"))
				return
			}
			image, err := a.tasks.GetPublicImage(r.Context(), item.Image.ID, req.ViewerID)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			summary := publicGalleryListItems([]domainimagetask.GalleryImage{image})[0]
			items = append(items, map[string]any{"media_type": "image", "id": summary.ID, "image": summary})
		} else if item.Video != nil {
			video := *item.Video
			excerpt := promptExcerpt(video.Prompt, 24)
			video.Prompt = ""
			items = append(items, map[string]any{"media_type": "video", "id": video.ID, "video": struct {
				videopublication.Video
				PromptExcerpt string `json:"prompt_excerpt"`
			}{Video: video, PromptExcerpt: excerpt}})
		}
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items, "pagination": map[string]int{"page": result.Page, "page_size": result.PageSize, "total": result.Total}})
}
