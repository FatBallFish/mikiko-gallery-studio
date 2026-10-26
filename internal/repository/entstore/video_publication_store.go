package entstore

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"errors"
	"fmt"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/publicvideointeraction"
	"net/http"
	"strings"
	"time"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/predicate"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
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
	item := videopublication.Record{Video: videopublication.Video{ID: v.ID, Title: v.Name, MediaType: "video", Status: v.VisibilityStatus, VideoURL: base + "/video", PosterURL: base + "/poster", DownloadURL: base + "/download", Width: v.Width, Height: v.Height, DurationMS: v.DurationMs, PublishedAt: v.PublishedAt, CreatedAt: v.CreatedAt}, OwnerID: v.UserID, Version: v.Version, ReviewReason: v.PublicationReviewReason}
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

func (s *VideoPublicationStore) GetForViewer(ctx context.Context, id uuid.UUID, viewer int64) (videopublication.Record, error) {
	v, err := s.Get(ctx, id)
	if err != nil {
		return v, err
	}
	if v.Status != videopublication.Approved {
		return videopublication.Record{}, errs.New(404, errs.CodeNotFound, "video not found")
	}
	q := s.client.PublicVideoInteraction.Query().Where(publicvideointeraction.VideoIDEQ(id))
	v.LikeCount, err = q.Clone().Where(publicvideointeraction.LikedEQ(true)).Count(ctx)
	if err != nil {
		return v, err
	}
	v.FavoriteCount, err = q.Clone().Where(publicvideointeraction.FavoritedEQ(true)).Count(ctx)
	if err != nil {
		return v, err
	}
	if viewer > 0 {
		interaction, e := q.Where(publicvideointeraction.UserIDEQ(viewer)).Only(ctx)
		if e != nil && !repoent.IsNotFound(e) {
			return v, e
		}
		if interaction != nil {
			v.LikedByViewer = interaction.Liked
			v.FavoritedByViewer = interaction.Favorited
		}
	}
	return v, nil
}
func (s *VideoPublicationStore) SetInteraction(ctx context.Context, id uuid.UUID, viewer int64, kind string, active bool) (videopublication.Record, error) {
	if viewer <= 0 || (kind != "like" && kind != "favorite") {
		return videopublication.Record{}, errs.BadRequest("invalid video interaction")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return videopublication.Record{}, err
	}
	defer tx.Rollback()
	// Lock the publication row before checking visibility or mutating interactions.
	// Withdrawal and moderation update this same row, serializing with reactions.
	predicates := append(videoPublicationPredicates(), mediaasset.IDEQ(id), mediaasset.VisibilityStatusEQ(videopublication.Approved))
	n, err := tx.MediaAsset.Update().Where(predicates...).AddVersion(0).Save(ctx)
	if err != nil {
		return videopublication.Record{}, err
	}
	if n != 1 {
		return videopublication.Record{}, errs.New(404, errs.CodeNotFound, "video not found")
	}
	interaction, err := tx.PublicVideoInteraction.Query().Where(publicvideointeraction.VideoIDEQ(id), publicvideointeraction.UserIDEQ(viewer)).Only(ctx)
	if repoent.IsNotFound(err) {
		create := tx.PublicVideoInteraction.Create().SetVideoID(id).SetUserID(viewer)
		if kind == "like" {
			create.SetLiked(active)
		} else {
			create.SetFavorited(active)
		}
		_, err = create.Save(ctx)
	} else if err == nil {
		update := tx.PublicVideoInteraction.UpdateOneID(interaction.ID)
		if kind == "like" {
			update.SetLiked(active)
		} else {
			update.SetFavorited(active)
		}
		_, err = update.Save(ctx)
	}
	if err != nil {
		return videopublication.Record{}, err
	}
	if err = tx.Commit(); err != nil {
		return videopublication.Record{}, err
	}
	return s.GetForViewer(ctx, id, viewer)
}
func (s *VideoPublicationStore) ListPublic(ctx context.Context, viewer int64, liked, favorited bool, sort string, page, size int) (videopublication.ReviewPage, error) {
	predicates := append(videoPublicationPredicates(), mediaasset.VisibilityStatusEQ(videopublication.Approved))
	q := s.client.MediaAsset.Query().Where(predicates...)
	if liked || favorited {
		q.Where(func(sel *sql.Selector) {
			x := sql.Table("public_video_interactions")
			sub := sql.Select(x.C("video_id")).From(x).Where(sql.And(sql.ColumnsEQ(x.C("video_id"), sel.C("id")), sql.EQ(x.C("user_id"), viewer)))
			if liked {
				sub.Where(sql.EQ(x.C("liked"), true))
			}
			if favorited {
				sub.Where(sql.EQ(x.C("favorited"), true))
			}
			sel.Where(sql.Exists(sub))
		})
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return videopublication.ReviewPage{}, err
	}
	if sort == "hot" {
		q.Order(func(sel *sql.Selector) {
			sel.OrderExpr(sql.Expr("(SELECT COALESCE(SUM(CASE WHEN x.liked THEN 2 ELSE 0 END + CASE WHEN x.favorited THEN 3 ELSE 0 END),0) FROM public_video_interactions x WHERE x.video_id=media_assets.id) DESC"))
		})
	}
	assets, err := q.Order(repoent.Desc(mediaasset.FieldPublishedAt), repoent.Desc(mediaasset.FieldCreatedAt), repoent.Desc(mediaasset.FieldID)).Offset((page - 1) * size).Limit(size).All(ctx)
	out := videopublication.ReviewPage{Items: []videopublication.Record{}, Total: total, Page: page, PageSize: size}
	if err != nil {
		return out, err
	}
	for _, asset := range assets {
		v, e := s.GetForViewer(ctx, asset.ID, viewer)
		if e != nil {
			if isPublicItemUnavailable(e) {
				// Keep the selected page window: do not refill from the next page.
				// Total reflects the count query before hydration, like offset pagination.
				continue
			}
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

// Only visibility/not-found errors are safe to skip during list hydration.
// Storage, cancellation and other application errors must still fail the request.
func isPublicItemUnavailable(err error) bool {
	if errors.Is(err, repoerr.ErrNotFound) {
		return true
	}
	var appErr *errs.Error
	return errors.As(err, &appErr) && appErr.StatusCode == http.StatusNotFound && appErr.Code == errs.CodeNotFound
}
