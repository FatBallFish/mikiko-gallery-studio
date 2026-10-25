package publicfeed

import (
	"context"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	"strings"
)

type Service struct{ store domain.Store }

func NewService(store domain.Store) *Service { return &Service{store: store} }
func (s *Service) List(ctx context.Context, req domain.Request) (domain.Page, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Model = strings.TrimSpace(req.Model)
	if req.Sort != "hot" {
		req.Sort = "latest"
	}
	if req.ViewerID <= 0 && (req.Liked || req.Favorited) {
		return domain.Page{Items: []domain.Item{}, Page: req.Page, PageSize: req.PageSize}, nil
	}
	return s.store.List(ctx, req)
}
