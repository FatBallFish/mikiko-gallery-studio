package db

import (
	"context"
	"strings"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	"github.com/google/uuid"
)

// BackfillAssetGroups is restartable and preserves empty names as ungrouped.
func BackfillAssetGroups(ctx context.Context, client *repoent.Client) (int, error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	changed := 0
	resolve := func(user int64, pid uuid.UUID, name string) (*repoent.AssetGroup, error) {
		name = strings.Join(strings.Fields(name), " ")
		if name == "" {
			return nil, nil
		}
		key := strings.ToLower(name)
		g, e := tx.AssetGroup.Query().Where(assetgroup.UserIDEQ(user), assetgroup.ProjectIDEQ(pid), assetgroup.NameKeyEQ(key)).Only(ctx)
		if repoent.IsNotFound(e) {
			g, e = tx.AssetGroup.Create().SetUserID(user).SetProjectID(pid).SetName(name).SetNameKey(key).Save(ctx)
		}
		return g, e
	}
	images, err := tx.ImageResult.Query().Where(imageresult.ImageGroupNEQ("")).All(ctx)
	if err != nil {
		return 0, err
	}
	for _, img := range images {
		pid := img.ProjectID
		if pid == nil {
			p, e := tx.Project.Query().Where(project.UserIDEQ(img.UserID), project.IsDefaultEQ(true), project.DeletedAtIsNil()).Only(ctx)
			if e != nil {
				return 0, e
			}
			pid = &p.ID
		}
		g, e := resolve(img.UserID, *pid, img.ImageGroup)
		if e != nil {
			return 0, e
		}
		if img.ProjectID == nil || (g != nil && img.ImageGroup != g.Name) {
			name := ""
			if g != nil {
				name = g.Name
			}
			if e := tx.ImageResult.UpdateOne(img).SetProjectID(*pid).SetImageGroup(name).Exec(ctx); e != nil {
				return 0, e
			}
			changed++
		}
	}
	assets, err := tx.MediaAsset.Query().All(ctx)
	if err != nil {
		return 0, err
	}
	for _, a := range assets {
		g, e := resolve(a.UserID, a.ProjectID, a.GroupName)
		if e != nil {
			return 0, e
		}
		b := tx.MediaAsset.UpdateOne(a)
		if g == nil {
			if a.GroupID == nil && a.GroupName == "" {
				continue
			}
			b.ClearGroupID().SetGroupName("")
		} else {
			if a.GroupID != nil && *a.GroupID == g.ID && a.GroupName == g.Name {
				continue
			}
			b.SetGroupID(g.ID).SetGroupName(g.Name)
		}
		if e := b.Exec(ctx); e != nil {
			return 0, e
		}
		changed++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}
