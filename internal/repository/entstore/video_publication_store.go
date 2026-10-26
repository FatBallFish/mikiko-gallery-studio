package entstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type VideoPublicationStore struct{ client *repoent.Client }

func NewVideoPublicationStore(client *repoent.Client) *VideoPublicationStore {
	return &VideoPublicationStore{client: client}
}
func videoPublicationPredicates() []predicate.MediaAsset {
	return []predicate.MediaAsset{mediaasset.MediaTypeEQ("video"), mediaasset.StatusEQ("ready"), mediaasset.DeletedAtIsNil()}
}
func (s *VideoPublicationStore) Get(ctx context.Context, id uuid.UUID) (videopublication.Record, error) {
	p := append(videoPublicationPredicates(), mediaasset.IDEQ(id))
	v, e := s.client.MediaAsset.Query().Where(p...).Only(ctx)
	if repoent.IsNotFound(e) {
		return videopublication.Record{}, errs.New(404, errs.CodeNotFound, "ready video not found")
	}
	if e != nil {
		return videopublication.Record{}, e
	}
	return s.project(ctx, v), nil
}
func (s *VideoPublicationStore) project(ctx context.Context, v *repoent.MediaAsset) videopublication.Record {
	base := "/api/public/v1/videos/" + v.ID.String()
	item := videopublication.Record{Video: videopublication.Video{ID: v.ID, Title: v.Name, MediaType: "video", Status: v.VisibilityStatus, VideoURL: base + "/video", PosterURL: base + "/poster", Width: v.Width, Height: v.Height, DurationMS: v.DurationMs, PublishedAt: v.PublishedAt, CreatedAt: v.CreatedAt}, OwnerID: v.UserID, Version: v.Version, ReviewReason: v.PublicationReviewReason}
	if v.SourceTaskID != nil && v.SourceTaskKind != nil && *v.SourceTaskKind == "video" {
		if task, e := s.client.VideoTask.Get(ctx, *v.SourceTaskID); e == nil && task.UserID == v.UserID {
			item.Prompt = task.PromptTemplate
		}
	}
	return item
}
func (s *VideoPublicationStore) List(ctx context.Context, status string, page, size int) (videopublication.ReviewPage, error) {
	p := videoPublicationPredicates()
	if status != "" && status != "all" {
		p = append(p, mediaasset.VisibilityStatusEQ(status))
	} else {
		p = append(p, mediaasset.VisibilityStatusNEQ(videopublication.Private))
	}
	q := s.client.MediaAsset.Query().Where(p...)
	total, e := q.Clone().Count(ctx)
	if e != nil {
		return videopublication.ReviewPage{}, e
	}
	items, e := q.Order(repoent.Desc(mediaasset.FieldCreatedAt), repoent.Desc(mediaasset.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	if e != nil {
		return videopublication.ReviewPage{}, e
	}
	result := videopublication.ReviewPage{Items: make([]videopublication.Record, 0, len(items)), Total: total, Page: page, PageSize: size}
	for _, v := range items {
		result.Items = append(result.Items, s.project(ctx, v))
	}
	return result, nil
}
func (s *VideoPublicationStore) Transition(ctx context.Context, id uuid.UUID, user int64, next, reason string) (videopublication.Record, error) {
	v, e := s.Get(ctx, id)
	if e != nil {
		return videopublication.Record{}, e
	}
	if user > 0 && v.OwnerID != user {
		return videopublication.Record{}, errs.New(404, errs.CodeNotFound, "video not found")
	}
	if user > 0 {
		if next != videopublication.Private && next != videopublication.Pending {
			return videopublication.Record{}, errs.BadRequest("invalid publication action")
		}
		if next == videopublication.Pending && (v.Status == videopublication.Pending || v.Status == videopublication.Approved) {
			return v, nil
		}
	} else {
		valid := (next == videopublication.Approved || next == videopublication.Rejected) && v.Status == videopublication.Pending || next == videopublication.Private && v.Status == videopublication.Approved
		if !valid {
			return videopublication.Record{}, errs.New(409, errs.CodeConflict, "video publication has already changed")
		}
	}
	p := append(videoPublicationPredicates(), mediaasset.IDEQ(id), mediaasset.VisibilityStatusEQ(v.Status), mediaasset.VersionEQ(v.Version))
	update := s.client.MediaAsset.Update().Where(p...).SetVisibilityStatus(next).SetPublicationReviewReason(strings.TrimSpace(reason)).AddVersion(1)
	if next == videopublication.Approved {
		update.SetPublishedAt(time.Now().UTC())
	} else {
		update.ClearPublishedAt()
	}
	count, e := update.Save(ctx)
	if e != nil {
		return videopublication.Record{}, fmt.Errorf("update video publication: %w", e)
	}
	if count != 1 {
		return videopublication.Record{}, errs.New(409, errs.CodeConflict, "video publication has already changed")
	}
	return s.Get(ctx, id)
}
