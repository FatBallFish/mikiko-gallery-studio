package db

import (
	"context"
	"fmt"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imageresult"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	projectent "github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	userent "github.com/fatballfish/pic-gallery/internal/repository/ent/user"
)

func BackfillLegacyProjectOwnership(ctx context.Context, client *repoent.Client, batchSize int) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("project backfill client is required")
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 100
	}
	var afterID int
	updatedRows := 0
	for {
		users, err := client.User.Query().
			Where(userent.IDGT(afterID), userent.DeletedAtIsNil()).
			Order(repoent.Asc(userent.FieldID)).Limit(batchSize).All(ctx)
		if err != nil {
			return updatedRows, fmt.Errorf("list project backfill users: %w", err)
		}
		if len(users) == 0 {
			return updatedRows, nil
		}
		for _, user := range users {
			userID := int64(user.ID)
			project, err := client.Project.Query().Where(
				projectent.UserIDEQ(userID), projectent.IsDefaultEQ(true),
				projectent.StatusEQ(domainproject.StatusActive), projectent.DeletedAtIsNil(),
			).Only(ctx)
			if repoent.IsNotFound(err) {
				project, err = client.Project.Create().
					SetUserID(userID).SetName(domainproject.DefaultName).SetNameKey(domainproject.DefaultName).
					SetIsDefault(true).SetStatus(domainproject.StatusActive).Save(ctx)
				if repoent.IsConstraintError(err) {
					project, err = client.Project.Query().Where(
						projectent.UserIDEQ(userID), projectent.IsDefaultEQ(true),
						projectent.StatusEQ(domainproject.StatusActive), projectent.DeletedAtIsNil(),
					).Only(ctx)
				}
			}
			if err != nil {
				return updatedRows, fmt.Errorf("ensure default project for user %d: %w", userID, err)
			}
			tx, err := client.Tx(ctx)
			if err != nil {
				return updatedRows, fmt.Errorf("start project backfill for user %d: %w", userID, err)
			}
			userUpdatedRows := 0
			taskCount, err := tx.ImageTask.Update().Where(imagetask.UserIDEQ(userID), imagetask.ProjectIDIsNil()).SetProjectID(project.ID).Save(ctx)
			if err == nil {
				var assetCount int
				assetCount, err = tx.ImageResult.Update().Where(imageresult.UserIDEQ(userID), imageresult.ProjectIDIsNil()).SetProjectID(project.ID).Save(ctx)
				userUpdatedRows = taskCount + assetCount
			}
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			if err != nil {
				return updatedRows, fmt.Errorf("backfill project ownership for user %d: %w", userID, err)
			}
			updatedRows += userUpdatedRows
			afterID = user.ID
		}
		if len(users) < batchSize {
			return updatedRows, nil
		}
	}
}
