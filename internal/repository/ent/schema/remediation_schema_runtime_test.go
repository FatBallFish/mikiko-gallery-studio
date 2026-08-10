package schema_test

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

func TestRemediationSchemaEnforcesRuntimeInvariants(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:remediation-schema?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	t.Run("upstream max n is limited to ten", func(t *testing.T) {
		_, err := client.ModelAccountModel.Create().
			SetAccountID(1).
			SetModelCode("invalid-max-n").
			SetMaxImageCount(11).
			Save(ctx)
		if err == nil {
			t.Fatal("max_image_count=11 was accepted")
		}
	})

	t.Run("active project defaults and names are unique per user", func(t *testing.T) {
		defaultProject := createProject(t, ctx, client, 101, "默认", "default", true)
		if _, err := client.Project.Create().SetUserID(101).SetName("另一个默认").SetNameKey("another-default").SetIsDefault(true).Save(ctx); !repoent.IsConstraintError(err) {
			t.Fatalf("second active default error = %v, want constraint error", err)
		}
		project := createProject(t, ctx, client, 101, "项目 A", "project-a", false)
		if _, err := client.Project.Create().SetUserID(101).SetName("重复项目 A").SetNameKey("project-a").Save(ctx); !repoent.IsConstraintError(err) {
			t.Fatalf("duplicate active name error = %v, want constraint error", err)
		}
		if _, err := project.Update().SetDeletedAt(time.Now()).Save(ctx); err != nil {
			t.Fatalf("soft-delete project: %v", err)
		}
		createProject(t, ctx, client, 101, "项目 A 重建", "project-a", false)
		if defaultProject.ID == uuid.Nil {
			t.Fatal("default project ID must be generated")
		}
	})

	t.Run("task project id is a real foreign key", func(t *testing.T) {
		_, err := client.ImageTask.Create().
			SetUserID(101).
			SetProjectID(uuid.New()).
			SetTaskType("text_to_image").
			SetPrompt("test").
			SetAbstractModel("image").
			Save(ctx)
		if !repoent.IsConstraintError(err) {
			t.Fatalf("foreign project task error = %v, want constraint error", err)
		}
		_, err = client.ImageResult.Create().
			SetTaskID(uuid.New()).
			SetUserID(101).
			SetProjectID(uuid.New()).
			SetObjectKey("users/101/missing-project.png").
			SetMimeType("image/png").
			SetSha256("missing-project-sha").
			Save(ctx)
		if !repoent.IsConstraintError(err) {
			t.Fatalf("foreign project result error = %v, want constraint error", err)
		}
	})

	t.Run("live cleanup jobs coalesce by canonical object identity", func(t *testing.T) {
		job, err := client.ObjectDeletionJob.Create().
			SetStorageDriver("s3").
			SetBucket("images").
			SetObjectKey("users/101/result.png").
			Save(ctx)
		if err != nil {
			t.Fatalf("create cleanup job: %v", err)
		}
		_, err = client.ObjectDeletionJob.Create().
			SetStorageDriver("s3").
			SetBucket("images").
			SetObjectKey("users/101/result.png").
			SetState("retry").
			Save(ctx)
		if !repoent.IsConstraintError(err) {
			t.Fatalf("duplicate live cleanup error = %v, want constraint error", err)
		}
		if _, err := job.Update().SetState("done").SetCompletedAt(time.Now()).Save(ctx); err != nil {
			t.Fatalf("complete cleanup job: %v", err)
		}
		if _, err := client.ObjectDeletionJob.Create().
			SetStorageDriver("s3").
			SetBucket("images").
			SetObjectKey("users/101/result.png").
			Save(ctx); err != nil {
			t.Fatalf("create cleanup job after completion: %v", err)
		}

		firstConfigID := uuid.New()
		secondConfigID := uuid.New()
		if _, err := client.ObjectDeletionJob.Create().SetStorageConfigID(firstConfigID).SetStorageDriver("s3").SetBucket("shared-name").SetObjectKey("result.png").Save(ctx); err != nil {
			t.Fatalf("create first configured cleanup job: %v", err)
		}
		if _, err := client.ObjectDeletionJob.Create().SetStorageConfigID(secondConfigID).SetStorageDriver("s3").SetBucket("shared-name").SetObjectKey("result.png").Save(ctx); err != nil {
			t.Fatalf("distinct storage configs must not collide: %v", err)
		}
		if _, err := client.ObjectDeletionJob.Create().SetStorageConfigID(firstConfigID).SetStorageDriver("s3").SetBucket("shared-name").SetObjectKey("result.png").Save(ctx); !repoent.IsConstraintError(err) {
			t.Fatalf("duplicate configured cleanup error = %v, want constraint error", err)
		}
		if _, err := client.ObjectDeletionJob.Create().SetStorageConfigID(firstConfigID).SetStorageDriver("renamed-driver").SetBucket("renamed-bucket").SetObjectKey("result.png").Save(ctx); !repoent.IsConstraintError(err) {
			t.Fatalf("configured cleanup identity must ignore driver and bucket: error=%v", err)
		}
	})

	t.Run("active reference aliases are unique per user and source", func(t *testing.T) {
		sourceID := uuid.New()
		first, err := client.ReferenceAsset.Create().
			SetUserID(101).
			SetStatus("ready").
			SetObjectKey("generated-images/101/source.png").
			SetMimeType("image/png").
			SetSha256("alias-source-one").
			SetSourceImageResultID(sourceID).
			SetOwnsObject(false).
			Save(ctx)
		if err != nil {
			t.Fatalf("create first alias: %v", err)
		}
		_, err = client.ReferenceAsset.Create().
			SetUserID(101).
			SetStatus("ready").
			SetObjectKey("generated-images/101/source.png").
			SetMimeType("image/png").
			SetSha256("alias-source-two").
			SetSourceImageResultID(sourceID).
			SetOwnsObject(false).
			Save(ctx)
		if !repoent.IsConstraintError(err) {
			t.Fatalf("duplicate active alias error=%v, want constraint error", err)
		}
		if _, err := first.Update().SetStatus("deleted").SetDeletedAt(time.Now().UTC()).Save(ctx); err != nil {
			t.Fatalf("soft delete first alias: %v", err)
		}
		if _, err := client.ReferenceAsset.Create().
			SetUserID(101).
			SetStatus("ready").
			SetObjectKey("generated-images/101/source.png").
			SetMimeType("image/png").
			SetSha256("alias-source-three").
			SetSourceImageResultID(sourceID).
			SetOwnsObject(false).
			Save(ctx); err != nil {
			t.Fatalf("recreate alias after soft delete: %v", err)
		}
	})
}

func createProject(t *testing.T, ctx context.Context, client *repoent.Client, userID int64, name, nameKey string, isDefault bool) *repoent.Project {
	t.Helper()
	project, err := client.Project.Create().
		SetUserID(userID).
		SetName(name).
		SetNameKey(nameKey).
		SetIsDefault(isDefault).
		Save(ctx)
	if err != nil {
		t.Fatalf("create project %q: %v", name, err)
	}
	return project
}
