package publicfeed

import (
	"context"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	"testing"
)

type captureStore struct {
	req   domain.Request
	calls int
}

func (s *captureStore) List(ctx context.Context, req domain.Request) (domain.Page, error) {
	s.req = req
	s.calls++
	return domain.Page{}, ctx.Err()
}
func TestPublicFeedBoundsAndAnonymousReactions(t *testing.T) {
	store := &captureStore{}
	service := NewService(store)
	if _, err := service.List(t.Context(), domain.Request{Page: -1, PageSize: 1000, Sort: "bad", Query: " x "}); err != nil {
		t.Fatal(err)
	}
	if store.req.Page != 1 || store.req.PageSize != 100 || store.req.Sort != "latest" || store.req.Query != "x" {
		t.Fatalf("%+v", store.req)
	}
	for _, req := range []domain.Request{{Liked: true}, {Favorited: true}} {
		page, err := service.List(t.Context(), req)
		if err != nil || len(page.Items) != 0 || store.calls != 1 {
			t.Fatalf("anonymous reaction %+v %v", page, err)
		}
	}
}
