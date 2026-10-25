package handlers

import (
	"encoding/json"
	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/publicfeed"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicFeedAnonymousSummary(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:feed-handler-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	if err = client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project := client.Project.Create().SetUserID(44).SetName("feed").SetNameKey("feed").SaveX(ctx)
	task := client.ImageTask.Create().SetUserID(44).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("private full image prompt beyond excerpt boundary").SetAbstractModel("test").SaveX(ctx)
	image := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(44).SetProjectID(project.ID).SetStorageDriver("local").SetObjectKey("private/image.png").SetMimeType("image/png").SetSha256("hash").SetVisibilityStatus("approved").SaveX(ctx)
	videoPrompt := "generated video full prompt with secret suffix beyond excerpt boundary"
	id := uuid.New()
	client.VideoTask.Create().SetID(id).SetUserID(44).SetProjectID(project.ID).SetStatus("succeeded").SetTaskType("text_to_video").SetPromptBindingSnapshot(map[string]any{}).SetPricingSnapshot(map[string]any{}).SetRoutingSnapshot(map[string]any{}).SetPromptTemplate(videoPrompt).SetExecutionPrompt(videoPrompt).SetRouteModelID(1).SetRouteModelCode("test").SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetIdempotencyKey(id.String()).SetRequestFingerprint(id.String()).SaveX(ctx)
	client.MediaAsset.Create().SetUserID(44).SetProjectID(project.ID).SetName("generated clip").SetNameKey("clip").SetMediaType("video").SetSourceType("generated").SetSourceTaskKind("video").SetSourceTaskID(id).SetStatus("ready").SetVisibilityStatus("approved").SetObjectKey("clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx)
	api := &API{tasks: imagetask.NewServiceWithStore(config.Config{}, entstore.NewImageTaskStore(client))}
	api.SetPublicFeed(publicfeed.NewService(entstore.NewPublicFeedStore(client)))
	w := httptest.NewRecorder()
	api.HandlePublicFeed(w, httptest.NewRequest(http.MethodGet, "/api/public/v1/media", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			Items []struct {
				MediaType string `json:"media_type"`
				Image     struct {
					Prompt      *string `json:"prompt"`
					URL         string  `json:"url"`
					DownloadURL string  `json:"download_url"`
				} `json:"image"`
				Video struct {
					Prompt        *string `json:"prompt"`
					PromptExcerpt string  `json:"prompt_excerpt"`
				} `json:"video"`
			} `json:"items"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Items) != 2 {
		t.Fatalf("%s", w.Body.String())
	}
	for _, item := range response.Data.Items {
		if item.MediaType == "image" {
			want := "/api/open/image/v1/gallery/images/" + image.ID.String() + "/image"
			if item.Image.Prompt != nil || item.Image.URL != want || item.Image.DownloadURL != want {
				t.Fatalf("anonymous nonremote image must use public projection: %+v", item.Image)
			}
		} else {
			if item.Video.Prompt != nil || item.Video.PromptExcerpt != promptExcerpt(videoPrompt, 24) {
				t.Fatalf("generated video prompt leak: %+v", item.Video)
			}
		}
	}
	if strings.Contains(w.Body.String(), videoPrompt) || strings.Contains(w.Body.String(), "/api/agent/") {
		t.Fatalf("private data in feed: %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}
