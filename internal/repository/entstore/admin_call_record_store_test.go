package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminCallRecordStoreListsImageTasksWithFilters(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-records?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	imageTasks := NewImageTaskStore(client)
	firstStarted := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	firstFinished := firstStarted.Add(2 * time.Second)
	seedTasks := []domainimagetask.Task{
		{
			UserID:                42,
			APIKeyID:              99,
			SourceChannel:         "openapi",
			ID:                    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Status:                domainimagetask.StatusFailed,
			Provider:              "openrouter",
			AbstractModel:         "plus",
			TaskType:              string(provider.TaskTypeTextToImage),
			Prompt:                "failed",
			RequestedQuality:      "auto",
			ResolvedQualityBucket: "2k",
			OutputImageCount:      3,
			ReferenceImageCount:   1,
			EstimatedPoints:       "12.00000",
			ActualPoints:          "4.00000",
			ErrorCode:             "provider_error",
			ErrorMessage:          "upstream failed",
			Attempts: []domainimagetask.Attempt{
				{Provider: "openai", Status: domainimagetask.StatusFailed, Error: "timeout"},
				{Provider: "openrouter", Status: domainimagetask.StatusFailed, Error: "quota"},
			},
		},
		{
			UserID:                42,
			APIKeyID:              100,
			SourceChannel:         "web",
			ID:                    "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Status:                domainimagetask.StatusSucceeded,
			Provider:              "openai",
			AbstractModel:         "basic",
			TaskType:              string(provider.TaskTypeTextToImage),
			Prompt:                "succeeded",
			RequestedQuality:      "auto",
			ResolvedQualityBucket: "1k",
			OutputImageCount:      1,
			ReferenceImageCount:   0,
			EstimatedPoints:       "2.00000",
			ActualPoints:          "2.00000",
		},
	}
	for _, task := range seedTasks {
		if err := imageTasks.Save(ctx, task); err != nil {
			t.Fatalf("Save %s: %v", task.ID, err)
		}
	}
	firstTask, err := client.ImageTask.Query().Where(imagetask.IDEQ(uuid.MustParse(seedTasks[0].ID))).Only(ctx)
	if err != nil {
		t.Fatalf("query first task: %v", err)
	}
	if err := client.ImageTask.UpdateOne(firstTask).
		SetStartedAt(firstStarted).
		SetFinishedAt(firstFinished).
		Exec(ctx); err != nil {
		t.Fatalf("set timestamps: %v", err)
	}

	store := NewAdminCallRecordStore(client)
	page, err := store.ListCallRecords(ctx, domainadmincallrecord.ListRequest{
		Page:          1,
		PageSize:      10,
		Status:        domainimagetask.StatusFailed,
		Provider:      "openrouter",
		SourceChannel: "openapi",
		UserID:        42,
		TaskID:        "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CreatedFrom:   firstTask.CreatedAt.Add(-time.Hour),
		CreatedTo:     firstTask.CreatedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ListCallRecords: %v", err)
	}
	if page.Total != 1 || page.Page != 1 || page.PageSize != 10 || len(page.Items) != 1 {
		t.Fatalf("unexpected page %#v", page)
	}
	record := page.Items[0]
	if record.TaskID != seedTasks[0].ID || record.UserID != 42 || record.APIKeyID == nil || *record.APIKeyID != 99 {
		t.Fatalf("unexpected identity fields %#v", record)
	}
	if record.SourceChannel != "openapi" || record.TaskType != string(provider.TaskTypeTextToImage) || record.Status != domainimagetask.StatusFailed {
		t.Fatalf("unexpected classification fields %#v", record)
	}
	if record.Provider != "openrouter" || record.AbstractModel != "plus" || record.Quality != "2k" {
		t.Fatalf("unexpected model fields %#v", record)
	}
	if record.RequestedOutputImageCount != 3 || record.SuccessOutputImageCount != 0 || record.ReferenceImageCount != 1 {
		t.Fatalf("unexpected count fields %#v", record)
	}
	if record.EstimatedPoints != "12.00000" || record.ActualPoints != "4.00000" {
		t.Fatalf("unexpected points fields %#v", record)
	}
	if record.ErrorCode == nil || *record.ErrorCode != "provider_error" || record.ErrorMessage == nil || *record.ErrorMessage != "upstream failed" {
		t.Fatalf("unexpected error fields %#v", record)
	}
	if record.StartedAt == nil || !record.StartedAt.Equal(firstStarted) || record.FinishedAt == nil || !record.FinishedAt.Equal(firstFinished) {
		t.Fatalf("unexpected timestamps %#v", record)
	}
	if record.AttemptCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", record.AttemptCount)
	}
}
