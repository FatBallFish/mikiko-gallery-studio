package entstore

import (
	"context"
	"strings"

	domain "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediauploadsession"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	service "github.com/fatballfish/pic-gallery/internal/service/assetgroup"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type AssetGroupStore struct{ client *repoent.Client }

func NewAssetGroupStore(client *repoent.Client) *AssetGroupStore {
	return &AssetGroupStore{client: client}
}

var _ service.Store = (*AssetGroupStore)(nil)

func groupNotFound() error { return errs.New(404, errs.CodeNotFound, "asset group not found") }
func groupConflict() error {
	return errs.New(409, errs.CodeConflict, "asset group changed or name already exists")
}
func ownedGroupProject(ctx context.Context, client *repoent.Client, userID int64, projectID string) (uuid.UUID, error) {
	q := client.Project.Query().Where(project.UserIDEQ(userID), project.StatusEQ("active"), project.DeletedAtIsNil(), lockProjectRows())
	if strings.TrimSpace(projectID) == "" {
		q.Where(project.IsDefaultEQ(true))
	} else {
		id, err := uuid.Parse(projectID)
		if err != nil {
			return uuid.Nil, groupNotFound()
		}
		q.Where(project.IDEQ(id))
	}
	p, err := q.Only(ctx)
	if repoent.IsNotFound(err) {
		return uuid.Nil, groupNotFound()
	}
	if err != nil {
		return uuid.Nil, err
	}
	return p.ID, nil
}
func mapAssetGroup(g *repoent.AssetGroup) domain.Group {
	return domain.Group{ID: g.ID.String(), UserID: g.UserID, ProjectID: g.ProjectID.String(), Name: g.Name, Version: g.Version, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
}
func (s *AssetGroupStore) List(ctx context.Context, userID int64, projectID string) ([]domain.Group, error) {
	p, err := ownedGroupProject(ctx, s.client, userID, projectID)
	if err != nil {
		return nil, err
	}
	gs, err := s.client.AssetGroup.Query().Where(assetgroup.UserIDEQ(userID), assetgroup.ProjectIDEQ(p)).Order(repoent.Asc(assetgroup.FieldNameKey), repoent.Asc(assetgroup.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Group, 0, len(gs))
	for _, g := range gs {
		result = append(result, mapAssetGroup(g))
	}
	return result, nil
}
func (s *AssetGroupStore) Get(ctx context.Context, userID int64, projectID, id string) (domain.Group, error) {
	p, err := ownedGroupProject(ctx, s.client, userID, projectID)
	if err != nil {
		return domain.Group{}, err
	}
	gid, err := uuid.Parse(id)
	if err != nil {
		return domain.Group{}, groupNotFound()
	}
	g, err := s.client.AssetGroup.Query().Where(assetgroup.IDEQ(gid), assetgroup.UserIDEQ(userID), assetgroup.ProjectIDEQ(p)).Only(ctx)
	if repoent.IsNotFound(err) {
		return domain.Group{}, groupNotFound()
	}
	if err != nil {
		return domain.Group{}, err
	}
	return mapAssetGroup(g), nil
}
func (s *AssetGroupStore) Create(ctx context.Context, userID int64, projectID string, req domain.CreateRequest) (domain.Group, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domain.Group, error) {
		p, err := ownedGroupProject(ctx, tx.Client(), userID, projectID)
		if err != nil {
			return domain.Group{}, err
		}
		name, key, err := service.NormalizeName(req.Name)
		if err != nil {
			return domain.Group{}, err
		}
		g, err := tx.AssetGroup.Create().SetUserID(userID).SetProjectID(p).SetName(name).SetNameKey(key).Save(ctx)
		if repoent.IsConstraintError(err) {
			return domain.Group{}, groupConflict()
		}
		if err != nil {
			return domain.Group{}, err
		}
		return mapAssetGroup(g), nil
	})
}
func (s *AssetGroupStore) Update(ctx context.Context, userID int64, projectID, id string, req domain.UpdateRequest) (domain.Group, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (domain.Group, error) {
		current, err := (&AssetGroupStore{tx.Client()}).Get(ctx, userID, projectID, id)
		if err != nil {
			return domain.Group{}, err
		}
		name, key, err := service.NormalizeName(req.Name)
		if err != nil {
			return domain.Group{}, err
		}
		gid := uuid.MustParse(current.ID)
		n, err := tx.AssetGroup.Update().Where(assetgroup.IDEQ(gid), assetgroup.VersionEQ(req.ExpectedVersion)).SetName(name).SetNameKey(key).AddVersion(1).Save(ctx)
		if repoent.IsConstraintError(err) {
			return domain.Group{}, groupConflict()
		}
		if err != nil {
			return domain.Group{}, err
		}
		if n != 1 {
			return domain.Group{}, groupConflict()
		}
		if err := rewriteGroupViews(ctx, tx, current, name, false); err != nil {
			return domain.Group{}, err
		}
		g, err := tx.AssetGroup.Get(ctx, gid)
		if err != nil {
			return domain.Group{}, err
		}
		return mapAssetGroup(g), nil
	})
}
func (s *AssetGroupStore) Delete(ctx context.Context, userID int64, projectID, id string, req domain.DeleteRequest) error {
	_, err := withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (bool, error) {
		current, err := (&AssetGroupStore{tx.Client()}).Get(ctx, userID, projectID, id)
		if err != nil {
			return false, err
		}
		n, err := tx.AssetGroup.Delete().Where(assetgroup.IDEQ(uuid.MustParse(current.ID)), assetgroup.VersionEQ(req.ExpectedVersion)).Exec(ctx)
		if err != nil {
			return false, err
		}
		if n != 1 {
			return false, groupConflict()
		}
		return true, rewriteGroupViews(ctx, tx, current, "", true)
	})
	return err
}
func rewriteGroupViews(ctx context.Context, tx *repoent.Tx, g domain.Group, name string, clear bool) error {
	pid, gid := uuid.MustParse(g.ProjectID), uuid.MustParse(g.ID)
	update := tx.MediaAsset.Update().Where(mediaasset.UserIDEQ(g.UserID), mediaasset.ProjectIDEQ(pid), mediaasset.Or(mediaasset.GroupIDEQ(gid), mediaasset.And(mediaasset.GroupIDIsNil(), mediaasset.GroupNameEqualFold(g.Name)))).SetGroupName(name).AddVersion(1)
	if clear {
		update.ClearGroupID()
	} else {
		update.SetGroupID(gid)
	}
	if _, err := update.Save(ctx); err != nil {
		return err
	}
	if _, err := tx.ImageResult.Update().Where(imageresult.UserIDEQ(g.UserID), imageresult.ProjectIDEQ(pid), imageresult.ImageGroupEqualFold(g.Name)).SetImageGroup(name).Save(ctx); err != nil {
		return err
	}
	_, err := tx.MediaUploadSession.Update().Where(mediauploadsession.UserIDEQ(g.UserID), mediauploadsession.ProjectIDEQ(pid), mediauploadsession.GroupNameEqualFold(g.Name)).SetGroupName(name).Save(ctx)
	return err
}

// transferAssetGroups resolves names into the destination namespace, including empty groups.
func transferAssetGroups(ctx context.Context, tx *repoent.Tx, userID int64, sourceID, targetID uuid.UUID) error {
	groups, err := tx.AssetGroup.Query().Where(assetgroup.UserIDEQ(userID), assetgroup.ProjectIDEQ(sourceID)).All(ctx)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if _, err := resolveAssetGroup(ctx, tx.Client(), userID, targetID, g.Name); err != nil {
			return err
		}
	}
	assets, err := tx.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.ProjectIDEQ(sourceID)).All(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {
		b := tx.MediaAsset.UpdateOne(a).SetProjectID(targetID).AddVersion(1)
		if err := setUpdateAssetGroup(ctx, tx, b, userID, targetID, a.GroupName); err != nil {
			return err
		}
		if err := b.Exec(ctx); err != nil {
			return err
		}
	}
	legacy, err := tx.ImageResult.Query().Where(imageresult.UserIDEQ(userID), imageresult.ProjectIDEQ(sourceID), imageresult.ImageGroupNEQ("")).All(ctx)
	if err != nil {
		return err
	}
	for _, img := range legacy {
		g, err := resolveAssetGroup(ctx, tx.Client(), userID, targetID, img.ImageGroup)
		if err != nil {
			return err
		}
		name := ""
		if g != nil {
			name = g.Name
		}
		if err := tx.ImageResult.UpdateOne(img).SetImageGroup(name).Exec(ctx); err != nil {
			return err
		}
	}
	uploads, err := tx.MediaUploadSession.Query().Where(mediauploadsession.UserIDEQ(userID), mediauploadsession.ProjectIDEQ(sourceID)).All(ctx)
	if err != nil {
		return err
	}
	for _, u := range uploads {
		g, err := resolveAssetGroup(ctx, tx.Client(), userID, targetID, u.GroupName)
		if err != nil {
			return err
		}
		name := ""
		if g != nil {
			name = g.Name
		}
		if err := tx.MediaUploadSession.UpdateOne(u).SetGroupName(name).Exec(ctx); err != nil {
			return err
		}
	}
	_, err = tx.AssetGroup.Delete().Where(assetgroup.UserIDEQ(userID), assetgroup.ProjectIDEQ(sourceID)).Exec(ctx)
	return err
}

// resolveAssetGroup keeps legacy string writes and the canonical ID in agreement.
func resolveAssetGroup(ctx context.Context, client *repoent.Client, userID int64, projectID uuid.UUID, name string) (*repoent.AssetGroup, error) {
	if _, err := client.Project.Query().Where(project.IDEQ(projectID), activeOwnedProject(userID), lockProjectRows()).Only(ctx); err != nil {
		if repoent.IsNotFound(err) {
			return nil, groupNotFound()
		}
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	name, key, err := service.NormalizeName(name)
	if err != nil {
		return nil, err
	}
	g, err := client.AssetGroup.Query().Where(assetgroup.UserIDEQ(userID), assetgroup.ProjectIDEQ(projectID), assetgroup.NameKeyEQ(key)).Only(ctx)
	if err == nil {
		return g, nil
	}
	if !repoent.IsNotFound(err) {
		return nil, err
	}
	return client.AssetGroup.Create().SetUserID(userID).SetProjectID(projectID).SetName(name).SetNameKey(key).Save(ctx)
}
func setCreateAssetGroup(ctx context.Context, tx *repoent.Tx, b *repoent.MediaAssetCreate, userID int64, pid uuid.UUID, name string) error {
	g, err := resolveAssetGroup(ctx, tx.Client(), userID, pid, name)
	if err != nil {
		return err
	}
	if g != nil {
		b.SetGroupID(g.ID).SetGroupName(g.Name)
	} else {
		b.SetGroupName("")
	}
	return nil
}
func setUpdateAssetGroup(ctx context.Context, tx *repoent.Tx, b *repoent.MediaAssetUpdateOne, userID int64, pid uuid.UUID, name string) error {
	g, err := resolveAssetGroup(ctx, tx.Client(), userID, pid, name)
	if err != nil {
		return err
	}
	if g != nil {
		b.SetGroupID(g.ID).SetGroupName(g.Name)
	} else {
		b.ClearGroupID().SetGroupName("")
	}
	return nil
}
