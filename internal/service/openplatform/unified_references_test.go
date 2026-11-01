package openplatform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

// No alias importer is needed for unified reads. If the platform tries to
// recover a rejected read through alias import, this probe catches the write.
type unifiedReferenceProbe struct {
	assets.Store
	owner          int64
	asset          domainassets.ReferenceAsset
	reads, imports int
}

func (s *unifiedReferenceProbe) GetByUserAndID(_ context.Context, owner int64, id string) (domainassets.ReferenceAsset, error) {
	s.reads++
	if owner != s.owner || id != s.asset.ID {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	return s.asset, nil
}
func (s *unifiedReferenceProbe) ImportMediaAssetAlias(context.Context, int64, mediaasset.Asset) (domainassets.ReferenceAsset, error) {
	s.imports++
	return domainassets.ReferenceAsset{}, errors.New("unexpected alias import")
}

func TestUnifiedImageReferencesValidateAllInputLocationsWithoutAliasImport(t *testing.T) {
	for _, location := range []string{"reference_asset_ids", "reference_bindings", "mask_asset_id"} {
		for _, state := range []struct {
			name, status, mime, code string
			owner                    int64
		}{
			{"ready", "ready", "image/png", "", 7},
			{"ready_original", "ready_original", "image/png", "", 7},
			{"processing", "processing", "image/png", "MEDIA_ASSET_NOT_READY", 7},
			{"failed", "failed", "image/png", "MEDIA_ASSET_PROCESSING_FAILED", 7},
			{"deleted", "deleted", "image/png", "NOT_FOUND", 7},
			{"foreign", "ready", "image/png", "NOT_FOUND", 8},
			{"wrong_type", "ready", "video/mp4", "MEDIA_ASSET_TYPE_MISMATCH", 7},
		} {
			t.Run(location+"/"+state.name, func(t *testing.T) {
				id := uuid.NewString()
				backend := storage.NewLocalBackend(t.TempDir())
				var content bytes.Buffer
				if err := png.Encode(&content, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
					t.Fatal(err)
				}
				if err := backend.Put(t.Context(), "fixture.png", "image/png", content.Bytes()); err != nil {
					t.Fatal(err)
				}
				probe := &unifiedReferenceProbe{owner: state.owner, asset: domainassets.ReferenceAsset{ID: id, Name: "subject", Status: state.status, MimeType: state.mime, ObjectKey: "fixture.png", Width: 2, Height: 2}}
				refs := assets.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, probe, backend)
				bills := billing.NewService(config.BillingConfig{CNYPerPoint: "0.01", PointsScale: 5})
				if _, err := bills.AdminAdjust(t.Context(), domainbilling.AdjustRequest{UserID: 7, ChangePoints: "100.00000", Reason: "fixture", IdempotencyKey: "credit"}); err != nil {
					t.Fatal(err)
				}
				images := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(config.Config{}, nil, imagetask.NewMemoryStore(), refs, preflightImageBilling{bills})
				svc := openplatform.New(openplatform.Dependencies{Auth: preflightUsers{}, Images: images, References: refs, Billing: bills, QuoteKey: []byte("01234567890123456789012345678901")})
				input := map[string]any{"project_id": "project", "route_model_code": "fixture", "prompt": "cat", "idempotency_key": "reference-test"}
				switch location {
				case "reference_asset_ids":
					input[location] = []string{id}
				case "reference_bindings":
					input["prompt"] = "edit {{@subject}}"
					if state.code == "" {
						input["reference_asset_ids"] = []string{id}
					}
					input[location] = []map[string]string{{"name": "subject", "asset_id": id}}
				case "mask_asset_id":
					input[location] = id
				}
				p := openplatform.Principal{UserID: 7, Scopes: []string{"images:write"}}
				for _, op := range []string{"preflight_image_task", "create_image_task"} {
					raw, _ := json.Marshal(input)
					out, err := svc.Execute(t.Context(), p, op, raw)
					if state.code != "" {
						var app *errs.Error
						if !errors.As(err, &app) || app.Code != state.code {
							t.Fatalf("%s: got %v want %s", op, err, state.code)
						}
					} else if op == "preflight_image_task" {
						if err != nil {
							t.Fatal(err)
						}
						encoded, _ := json.Marshal(out)
						var quote struct {
							Token string `json:"quote_token"`
						}
						if err := json.Unmarshal(encoded, &quote); err != nil || quote.Token == "" {
							t.Fatalf("missing quote: %s %v", encoded, err)
						}
						// Keep the original IDs in both the quote and create request.
						input["quote_token"] = quote.Token
					} else if err != nil {
						t.Fatal(err)
					} else {
						probe.asset.Status = "deleted"
						delete(input, "quote_token")
						replayRaw, _ := json.Marshal(input)
						before := probe.reads
						if _, err := svc.Execute(t.Context(), p, op, replayRaw); err != nil {
							t.Fatalf("persisted replay: %v", err)
						}
						if probe.reads != before {
							t.Fatal("replay revalidated deleted reference")
						}
						input["prompt"] = "changed business input"
						changedRaw, _ := json.Marshal(input)
						_, err := svc.Execute(t.Context(), p, op, changedRaw)
						var conflict *errs.Error
						if !errors.As(err, &conflict) || conflict.Code != "IDEMPOTENCY_CONFLICT" {
							t.Fatalf("changed replay: %v", err)
						}
					}
				}
				if probe.reads == 0 || probe.imports != 0 {
					t.Fatalf("reads=%d alias imports=%d", probe.reads, probe.imports)
				}
			})
		}
	}
}

func TestUnifiedUploadToolDescriptions(t *testing.T) {
	specs := map[string]openplatform.OperationSpec{}
	for _, s := range openplatform.Specs() {
		specs[s.Name] = s
	}
	for _, name := range []string{"create_upload", "complete_upload", "get_asset"} {
		for _, want := range []string{"poll", "ready", "ready_original"} {
			if !strings.Contains(specs[name].Description, want) {
				t.Fatalf("%s description omits %s", name, want)
			}
		}
	}
	for _, name := range []string{"preflight_image_task", "create_image_task", "preflight_video_task", "create_video_task"} {
		props := specs[name].InputSchema["properties"].(map[string]any)
		keys := []string{"reference_bindings", "inputs"}
		if strings.Contains(name, "image") {
			keys = []string{"reference_asset_ids", "mask_asset_id", "reference_bindings"}
		}
		for _, key := range keys {
			raw, _ := json.Marshal(props[key])
			if !strings.Contains(string(raw), "ready_original") {
				t.Fatalf("%s.%s omits readiness", name, key)
			}
		}
	}
}
