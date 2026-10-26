package router

import "net/http"

// Keep action-prefix registrations and HEAD media access explicit alongside the
// OpenAPI contract. The route contract currently imports only GET/POST/PUT/PATCH/DELETE.
func init() {
	for template, methods := range map[string]map[string]bool{
		"/api/open/v1/asset-access/{grant}":                          {http.MethodGet: true, http.MethodHead: true},
		"/api/open/v1/asset-access/{grant}/content":                  {http.MethodGet: true, http.MethodHead: true},
		"/api/agent/media/v1/video-publications/{asset_id}":          {http.MethodGet: true, http.MethodPost: true},
		"/api/agent/media/v1/video-publications/{asset_id}:withdraw": {http.MethodPost: true},
		"/api/public/v1/videos/{asset_id}":                           {http.MethodGet: true, http.MethodHead: true},
		"/api/public/v1/videos/{asset_id}/video":                     {http.MethodGet: true, http.MethodHead: true},
		"/api/public/v1/videos/{asset_id}/poster":                    {http.MethodGet: true, http.MethodHead: true},
		"/api/public/v1/videos/{asset_id}/download":                  {http.MethodGet: true, http.MethodHead: true},
		"/api/public/v1/videos/{asset_id}/like":                      {http.MethodPost: true},
		"/api/public/v1/videos/{asset_id}/favorite":                  {http.MethodPost: true},
		"/api/ops/admin/v1/video-reviews/{asset_id}:approve":         {http.MethodPost: true},
		"/api/ops/admin/v1/video-reviews/{asset_id}:reject":          {http.MethodPost: true},
		"/api/ops/admin/v1/video-reviews/{asset_id}:unpublish":       {http.MethodPost: true},
		"/api/ops/admin/v1/video-reviews/{asset_id}/video":           {http.MethodGet: true, http.MethodHead: true},
		"/api/ops/admin/v1/video-reviews/{asset_id}/poster":          {http.MethodGet: true, http.MethodHead: true},
		"/api/agent/image/v1/history/tasks/{task_id}/retry":          {http.MethodPost: true},
		"/api/agent/video/v1/history/tasks/{task_id}":                {http.MethodDelete: true},
		"/api/agent/video/v1/history/tasks/{task_id}/retry":          {http.MethodPost: true},
	} {
		supplementalNormalTemplateRoutes[template] = methods
	}
}
