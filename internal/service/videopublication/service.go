package videopublication

import (
	"context"
	"time"

	mediaasset "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

const (
	Private  = "private"
	Pending  = "pending_review"
	Approved = "approved"
	Rejected = "rejected"
)

type Video struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	MediaType   string     `json:"media_type"`
	Status      string     `json:"status"`
	Prompt      string     `json:"prompt,omitempty"`
	VideoURL    string     `json:"video_url"`
	PosterURL   string     `json:"poster_url"`
	Width       *int       `json:"width,omitempty"`
	Height      *int       `json:"height,omitempty"`
	DurationMS  *int64     `json:"duration_ms,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
type Record struct {
	Video
	OwnerID      int64  `json:"-"`
	Version      int64  `json:"-"`
	ReviewReason string `json:"review_reason,omitempty"`
}
type Page struct {
	Items    []Video `json:"items"`
	Total    int     `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}
type ReviewPage struct {
	Items    []Record `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}
type Store interface {
	Get(context.Context, uuid.UUID) (Record, error)
	List(context.Context, string, int, int) (ReviewPage, error)
	Transition(context.Context, uuid.UUID, int64, string, string) (Record, error)
}
type Service struct {
	store Store
	media *mediaasset.Service
}

func (s *Service) OpenRange(ctx context.Context, id uuid.UUID, poster, admin bool, value string, metadataOnly bool) (mediaasset.RangedContentStream, error) {
	v, err := s.Get(ctx, id, admin)
	if err != nil {
		return mediaasset.RangedContentStream{}, err
	}
	purpose := mediaasset.AccessPurposePreview
	if poster {
		purpose = mediaasset.AccessPurposePoster
	}
	return s.media.OpenContentRange(ctx, v.OwnerID, id, purpose, value, metadataOnly)
}

func NewService(store Store, media *mediaasset.Service) *Service {
	return &Service{store: store, media: media}
}
func (s *Service) Owner(ctx context.Context, user int64, id uuid.UUID) (Record, error) {
	v, e := s.store.Get(ctx, id)
	if e != nil {
		return Record{}, e
	}
	if v.OwnerID != user {
		return Record{}, errs.New(404, errs.CodeNotFound, "video not found")
	}
	return v, nil
}
func (s *Service) Submit(ctx context.Context, user int64, id uuid.UUID) (Record, error) {
	if _, e := s.Owner(ctx, user, id); e != nil {
		return Record{}, e
	}
	// Both uploaded and generated videos must have completed processing, including
	// the exact playback and poster derivatives reviewers and public viewers see.
	for _, purpose := range []string{mediaasset.AccessPurposePreview, mediaasset.AccessPurposePoster} {
		stream, e := s.media.OpenContent(ctx, user, id, purpose)
		if e != nil {
			return Record{}, e
		}
		_ = stream.Reader.Close()
	}
	return s.store.Transition(ctx, id, user, Pending, "")
}
func (s *Service) Withdraw(ctx context.Context, user int64, id uuid.UUID) (Record, error) {
	return s.store.Transition(ctx, id, user, Private, "")
}
func (s *Service) Review(ctx context.Context, id uuid.UUID, status, reason string) (Record, error) {
	return s.store.Transition(ctx, id, 0, status, reason)
}
func (s *Service) Get(ctx context.Context, id uuid.UUID, admin bool) (Record, error) {
	v, e := s.store.Get(ctx, id)
	if e != nil {
		return Record{}, e
	}
	if !admin && v.Status != Approved {
		return Record{}, errs.New(404, errs.CodeNotFound, "video not found")
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, status string, page, size int, admin bool) (ReviewPage, error) {
	if !admin {
		status = Approved
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return s.store.List(ctx, status, page, size)
}
func (s *Service) Open(ctx context.Context, id uuid.UUID, poster, admin bool) (mediaasset.ContentStream, error) {
	v, e := s.Get(ctx, id, admin)
	if e != nil {
		return mediaasset.ContentStream{}, e
	}
	purpose := mediaasset.AccessPurposePreview
	if poster {
		purpose = mediaasset.AccessPurposePoster
	}
	return s.media.OpenContent(ctx, v.OwnerID, id, purpose)
}
