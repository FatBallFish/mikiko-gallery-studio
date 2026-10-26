package entstore

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"fmt"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	"strings"
)

type PublicFeedStore struct{ client *repoent.Client }

func NewPublicFeedStore(client *repoent.Client) *PublicFeedStore {
	return &PublicFeedStore{client: client}
}

// Both media kinds are filtered before UNION ALL. ORDER/LIMIT/OFFSET apply once
// to the union, never to independently paginated image and video lists.
func publicFeedUnion(req domain.Request) sql.Querier {
	return sql.ExprFunc(func(b *sql.Builder) {
		b.WriteString("(SELECT i.id, 'image' AS media_type, COALESCE(i.published_at,i.created_at) AS published_at, i.created_at, COALESCE(s.like_count,0)*2+COALESCE(s.favorite_count,0)*3 AS score FROM task_images i JOIN image_tasks t ON t.id=i.task_id LEFT JOIN media_assets a ON a.legacy_image_result_id=i.id AND a.deleted_at IS NULL LEFT JOIN users u ON u.id=i.user_id LEFT JOIN public_image_stats s ON s.image_id=i.id WHERE i.deleted_at IS NULL AND i.visibility_status='approved' AND t.deleted_at IS NULL")
		if req.Query != "" {
			b.WriteString(" AND (LOWER(t.prompt) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(" OR LOWER(COALESCE(a.name,'')) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(" OR LOWER(COALESCE(u.nickname,'')) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(")")
		}
		if req.Model != "" {
			b.WriteString(" AND (t.route_model_code=")
			b.Arg(req.Model)
			b.WriteString(" OR t.abstract_model=")
			b.Arg(req.Model)
			b.WriteString(")")
		}
		if req.Liked || req.Favorited {
			b.WriteString(" AND EXISTS (SELECT 1 FROM public_image_interactions x WHERE x.image_id=i.id AND x.user_id=")
			b.Arg(req.ViewerID)
			if req.Liked {
				b.WriteString(" AND x.liked=TRUE")
			}
			if req.Favorited {
				b.WriteString(" AND x.favorited=TRUE")
			}
			b.WriteString(")")
		}
		b.WriteString(" UNION ALL SELECT v.id, 'video' AS media_type, COALESCE(v.published_at,v.created_at) AS published_at, v.created_at, 0 AS score FROM media_assets v LEFT JOIN video_tasks vt ON vt.id=v.source_task_id AND v.source_task_kind='video' LEFT JOIN users vu ON vu.id=v.user_id WHERE v.deleted_at IS NULL AND v.media_type='video' AND v.status='ready' AND v.visibility_status='approved'")
		if req.Liked || req.Favorited {
			b.WriteString(" AND 1=0")
		}
		if req.Query != "" {
			b.WriteString(" AND (LOWER(v.name) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(" OR LOWER(COALESCE(vt.prompt_template,'')) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(" OR LOWER(COALESCE(vu.nickname,'')) LIKE ")
			b.Arg("%" + strings.ToLower(req.Query) + "%")
			b.WriteString(")")
		}
		if req.Model != "" {
			b.WriteString(" AND vt.route_model_code=")
			b.Arg(req.Model)
		}
		b.WriteString(") AS feed")
	})
}
func (s *PublicFeedStore) List(ctx context.Context, req domain.Request) (domain.Page, error) {
	out := domain.Page{Items: []domain.Item{}, Page: req.Page, PageSize: req.PageSize}
	total, err := s.client.ImageResult.Query().Aggregate(func(q *sql.Selector) string { q.FromExpr(publicFeedUnion(req)); return "COUNT(*)" }).Int(ctx)
	if err != nil {
		return out, fmt.Errorf("count public feed: %w", err)
	}
	out.Total = total
	var rows []struct {
		ID        uuid.UUID `json:"id"`
		MediaType string    `json:"media_type"`
	}
	err = s.client.ImageResult.Query().Aggregate(func(q *sql.Selector) string {
		q.FromExpr(publicFeedUnion(req))
		if req.Sort == "hot" {
			q.OrderBy(sql.Desc("score"))
		}
		q.OrderBy(sql.Desc("published_at"), sql.Desc("created_at"), sql.Desc("id"), sql.Desc("media_type")).Limit(req.PageSize).Offset((req.Page - 1) * req.PageSize)
		return "id"
	}, func(*sql.Selector) string { return "media_type" }).Scan(ctx, &rows)
	if err != nil {
		return out, fmt.Errorf("query public feed: %w", err)
	}
	images := NewImageTaskStore(s.client)
	videos := NewVideoPublicationStore(s.client)
	for _, row := range rows {
		if row.MediaType == "image" {
			image, e := images.GetPublicImage(ctx, row.ID.String(), req.ViewerID)
			if e != nil {
				return out, e
			}
			out.Items = append(out.Items, domain.Item{MediaType: "image", Image: &image})
		} else {
			video, e := videos.Get(ctx, row.ID)
			if e != nil {
				return out, e
			}
			if video.Status != "approved" {
				continue
			}
			out.Items = append(out.Items, domain.Item{MediaType: "video", Video: &video.Video})
		}
	}
	return out, nil
}
