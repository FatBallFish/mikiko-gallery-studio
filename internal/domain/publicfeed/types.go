package publicfeed

import (
	"context"
	"github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
)

type Request struct {
	Page, PageSize     int
	Sort, Query, Model string
	ViewerID           int64
	Liked, Favorited   bool
}
type Item struct {
	MediaType string                  `json:"media_type"`
	Image     *imagetask.GalleryImage `json:"-"`
	Video     *videopublication.Video `json:"video,omitempty"`
}
type Page struct {
	Items                 []Item
	Total, Page, PageSize int
}
type Store interface {
	List(context.Context, Request) (Page, error)
}
