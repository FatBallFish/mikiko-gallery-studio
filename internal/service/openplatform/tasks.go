package openplatform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"strings"
	"sync"
	"time"
)

type ImageInput struct {
	Operation         nativeimage.Operation        `json:"operation,omitempty"`
	NativeOptions     *nativeimage.Options         `json:"native_options,omitempty"`
	MaskAssetID       string                       `json:"mask_asset_id"`
	ProjectID         string                       `json:"project_id"`
	AbstractModel     string                       `json:"abstract_model"`
	RouteModelCode    string                       `json:"route_model_code"`
	TaskType          string                       `json:"task_type"`
	Prompt            string                       `json:"prompt"`
	PromptVariables   []image.PromptVariableInput  `json:"prompt_variables"`
	ReferenceBindings []image.PromptReferenceInput `json:"reference_bindings"`
	ReferenceAssetIDs []string                     `json:"reference_asset_ids"`
	NegativePrompt    string                       `json:"negative_prompt"`
	SizeMode          string                       `json:"size_mode"`
	RequestedSize     string                       `json:"requested_size"`
	BaseResolution    string                       `json:"base_resolution"`
	Quality           string                       `json:"quality"`
	OutputFormat      string                       `json:"output_format"`
	Background        string                       `json:"background"`
	OutputCompression int                          `json:"output_compression"`
	Moderation        string                       `json:"moderation"`
	AspectRatio       string                       `json:"aspect_ratio"`
	OutputImageCount  int                          `json:"output_image_count"`
	ReferenceStrength int                          `json:"reference_strength"`
	Seed              *int64                       `json:"seed"`
	CapabilityVersion string                       `json:"capability_version"`
	QuoteToken        string                       `json:"quote_token"`
	IdempotencyKey    string                       `json:"idempotency_key"`
}

// Serialize creation per idempotency key in this service instance. Database billing
// reservations additionally enforce task identity across replicas.
var createLocks [256]sync.Mutex

func lockCreate(id string) func() {
	sum := sha256.Sum256([]byte(id))
	m := &createLocks[int(sum[0])]
	m.Lock()
	return m.Unlock
}
func taskID(p Principal, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("openplatform:"+p.APIKeyID+":"+p.ClientID+":"+p.GrantID+":"+decimal.NewFromInt(p.UserID).String()+":"+key)).String()
}
func (s *Service) imageOperation(ctx context.Context, p Principal, user domainauth.User, op string, raw json.RawMessage) (any, error) {
	if s.deps.Images == nil {
		return nil, unavailable()
	}
	if op == "get_image_task_layers" {
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		task, err := s.deps.Images.GetByID(ctx, user.ID, in.ID)
		if err != nil {
			return nil, err
		}
		if task.Operation != nativeimage.OperationLayerDecomposition {
			return nil, errs.BadRequest("task is not a layer decomposition")
		}
		layers := make([]map[string]any, 0, len(task.Results))
		for _, r := range task.Results {
			layers = append(layers, map[string]any{"image_id": r.ID, "width": r.Width, "height": r.Height, "format": r.Format, "layer": r.Layer, "access_operation": "get_image_result_access", "access_input": map[string]any{"task_id": task.ID, "image_id": r.ID, "purpose": "download"}})
		}
		return map[string]any{"version": 1, "task_id": task.ID, "operation": task.Operation, "layers": layers}, nil
	}
	if op == "get_image_task" || op == "list_image_tasks" {
		var in struct {
			ID        string `json:"id"`
			ProjectID string `json:"project_id"`
			Limit     int    `json:"limit"`
			Cursor    string `json:"cursor"`
			Status    string `json:"status"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if op == "get_image_task" {
			return s.deps.Images.GetByID(ctx, user.ID, in.ID)
		}
		items, err := s.deps.Images.ListByUserProject(ctx, user.ID, in.ProjectID)
		if err != nil {
			return nil, err
		}
		if in.Status != "" {
			filtered := items[:0]
			for _, task := range items {
				if task.Status == in.Status {
					filtered = append(filtered, task)
				}
			}
			items = filtered
		}
		if in.Cursor != "" {
			found := false
			for i, task := range items {
				if task.ID == in.Cursor {
					items = items[i+1:]
					found = true
					break
				}
			}
			if !found {
				return nil, errs.BadRequest("invalid task cursor")
			}
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		next := ""
		if len(items) > limit {
			next = items[limit-1].ID
			items = items[:limit]
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}
	var in ImageInput
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	kid, err := keyID(p)
	if err != nil {
		return nil, err
	}
	req := image.CreateRequest{UserID: user.ID, ProjectID: in.ProjectID, APIKeyID: kid, SourceChannel: "open_api", AbstractModel: in.AbstractModel, RouteModelCode: in.RouteModelCode, TaskType: in.TaskType, Prompt: in.Prompt, PromptVariables: in.PromptVariables, ReferenceBindings: in.ReferenceBindings, ReferenceAssetIDs: in.ReferenceAssetIDs, ReferenceImageCount: len(in.ReferenceAssetIDs), NegativePrompt: in.NegativePrompt, SizeMode: in.SizeMode, RequestedSize: in.RequestedSize, BaseResolution: in.BaseResolution, Quality: in.Quality, OutputFormat: in.OutputFormat, Background: in.Background, OutputCompression: in.OutputCompression, Moderation: in.Moderation, AspectRatio: in.AspectRatio, OutputImageCount: in.OutputImageCount, ReferenceStrength: in.ReferenceStrength, Seed: in.Seed, UserGroupCode: user.GroupCode, UserGroupCodes: user.GroupCodes, UserGroupMultiplier: user.GroupMultiplier, CapabilityVersion: in.CapabilityVersion}
	req.Operation = in.Operation
	req.NativeOptions = in.NativeOptions
	req.MaskAssetID = in.MaskAssetID
	req.MaskPresent = in.MaskAssetID != ""
	if req.TaskType == "" {
		req.TaskType = "text_to_image"
	}
	if req.OutputImageCount == 0 {
		req.OutputImageCount = 1
	}
	req.RequestFingerprint = imageInputFingerprint(in)
	if in.IdempotencyKey != "" {
		req.TaskID = taskID(p, in.IdempotencyKey)
	}
	if op == "create_image_task" {
		if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 128 {
			return nil, errs.BadRequest("idempotency_key of 1..128 characters required")
		}
		unlock := lockCreate(req.TaskID)
		defer unlock()
		if existing, err := s.deps.Images.GetByID(ctx, user.ID, req.TaskID); err == nil {
			fingerprint := req.RequestFingerprint
			if existing.RequestFingerprint != fingerprint {
				return nil, errs.New(409, "IDEMPOTENCY_CONFLICT", "idempotency key was used for another request")
			}
			return existing, nil
		}
	}
	referenceIDs := append([]string(nil), in.ReferenceAssetIDs...)
	if in.MaskAssetID != "" {
		referenceIDs = append(referenceIDs, in.MaskAssetID)
	}
	for _, id := range referenceIDs {
		if s.deps.References == nil {
			return nil, unavailable()
		}
		if _, err := s.deps.References.GetWithContext(ctx, user.ID, id); err != nil {
			parsed, parseErr := uuid.Parse(id)
			if parseErr != nil || s.deps.Assets == nil {
				return nil, err
			}
			asset, assetErr := s.deps.Assets.GetAsset(ctx, user.ID, parsed)
			if assetErr != nil {
				return nil, assetErr
			}
			if _, assetErr = s.deps.References.ImportMediaAssetAlias(ctx, user.ID, asset); assetErr != nil {
				return nil, assetErr
			}
		}
	}
	estimate, err := s.deps.Images.Preflight(ctx, req)
	if err != nil {
		return nil, err
	}
	balance, err := s.checkFunds(ctx, user, estimate.EstimatedPoints, kid)
	if err != nil {
		return nil, err
	}
	estimate.Sufficient = true
	estimate.InsufficientPoints = "0.00000"
	estimate.Balance = &balance
	binding := map[string]any{"request": imageFingerprint(req), "capability": estimate.CapabilityVersion, "pricing": estimate.PricingSnapshot}
	if op == "preflight_image_task" {
		token, expiry, err := s.signQuote(p, binding)
		if err != nil {
			return nil, err
		}
		return struct {
			domainbilling.EstimateResult
			QuoteToken string    `json:"quote_token"`
			ExpiresAt  time.Time `json:"expires_at"`
		}{estimate, token, expiry}, nil
	}
	if err = s.verifyQuote(p, binding, in.QuoteToken); err != nil {
		return nil, err
	}
	task, err := s.deps.Images.CreateTask(ctx, req)
	return task, err
}

// imageInputFingerprint contains client business inputs only. Quote/configuration
// versions and effective account pricing must never change a persisted replay.
func imageInputFingerprint(in ImageInput) string {
	in.QuoteToken = ""
	in.IdempotencyKey = ""
	in.CapabilityVersion = ""
	if in.TaskType == "" {
		in.TaskType = "text_to_image"
	}
	if in.OutputImageCount == 0 {
		in.OutputImageCount = 1
	}
	if len(in.ReferenceAssetIDs) == 0 {
		in.ReferenceAssetIDs = nil
	}
	if len(in.ReferenceBindings) == 0 {
		in.ReferenceBindings = nil
	}
	if len(in.PromptVariables) == 0 {
		in.PromptVariables = nil
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func imageFingerprint(req image.CreateRequest) string {
	req.TaskID = ""
	req.APIKeyID = 0
	req.UserGroupCode = ""
	req.UserGroupCodes = nil
	req.UserGroupMultiplier = ""
	req.RequestFingerprint = ""
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Service) checkFunds(ctx context.Context, user domainauth.User, points string, kid int64) (domainbilling.BalanceSummary, error) {
	if s.deps.Billing == nil {
		return domainbilling.BalanceSummary{}, unavailable()
	}
	balance, err := s.deps.Billing.GetBalance(ctx, user.ID, user.GroupMultiplier)
	if err != nil {
		return balance, err
	}
	available, e1 := decimal.NewFromString(balance.AvailablePoints)
	needed, e2 := decimal.NewFromString(points)
	if e1 != nil || e2 != nil {
		return balance, errs.Internal("invalid points value")
	}
	if available.LessThan(needed) {
		return balance, errs.New(402, "INSUFFICIENT_BALANCE", "insufficient available points")
	}
	if kid > 0 && s.deps.APIKeys != nil {
		_, err = s.deps.APIKeys.CheckTaskAllowed(ctx, kid, user.ID, points, s.deps.Now().UTC())
	}
	return balance, err
}
func (s *Service) videoOperation(ctx context.Context, p Principal, user domainauth.User, op string, raw json.RawMessage) (any, error) {
	if s.deps.Videos == nil {
		return nil, unavailable()
	}
	if op == "get_video_task" || op == "list_video_tasks" || op == "cancel_video_task" {
		var in struct {
			ID             uuid.UUID  `json:"id"`
			ProjectID      *uuid.UUID `json:"project_id"`
			Limit          int        `json:"limit"`
			Cursor         string     `json:"cursor"`
			Status         string     `json:"status"`
			IdempotencyKey string     `json:"idempotency_key"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		switch op {
		case "get_video_task":
			return s.deps.Videos.Get(ctx, user.ID, in.ID)
		case "cancel_video_task":
			return s.deps.Videos.Cancel(ctx, user.ID, in.ID, in.IdempotencyKey)
		default:
			return s.deps.Videos.List(ctx, videotask.ListRequest{UserID: user.ID, ProjectID: in.ProjectID, Limit: in.Limit, Cursor: in.Cursor, Status: in.Status})
		}
	}
	var envelope struct {
		videotask.CreateRequest
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decode(raw, &envelope); err != nil {
		return nil, err
	}
	req := envelope.CreateRequest
	req.UserID = user.ID
	req.UserGroupCodes = user.GroupCodes
	req.SourceChannel = "open_api"
	req.IdempotencyKey = envelope.IdempotencyKey
	kid, err := keyID(p)
	if err != nil {
		return nil, err
	}
	req.APIKeyID = kid
	if op == "create_video_task" {
		if req.IdempotencyKey == "" || len(req.IdempotencyKey) > 128 {
			return nil, errs.BadRequest("idempotency_key of 1..128 characters required")
		}
		unlock := lockCreate(taskID(p, req.IdempotencyKey))
		defer unlock()
		if existing, found, replayErr := s.deps.Videos.Replay(ctx, req); replayErr != nil {
			return nil, replayErr
		} else if found {
			return map[string]any{"task": existing, "replayed": true}, nil
		}
	}
	estimate, err := s.deps.Videos.Estimate(ctx, req)
	if err != nil {
		return nil, err
	}
	if _, err = s.checkFunds(ctx, user, estimate.MaxReservedPoints, kid); err != nil {
		return nil, err
	}
	req.APIKeyID = kid
	binding := map[string]any{"request": req, "capability": estimate.CapabilityVersion, "price": estimate.PriceVersion, "config": estimate.ConfigVersion}
	boundReq := req
	boundReq.QuoteToken = ""
	boundReq.IdempotencyKey = ""
	binding["request"] = boundReq
	if op == "preflight_video_task" {
		token, expiry, err := s.signQuote(p, binding)
		if err != nil {
			return nil, err
		}
		estimate.QuoteToken = token
		estimate.ExpiresAt = expiry
		return estimate, nil
	}
	if err = s.verifyQuote(p, binding, req.QuoteToken); err != nil {
		return nil, err
	}
	req.QuoteToken = estimate.QuoteToken
	task, replayed, err := s.deps.Videos.Create(ctx, req)
	return map[string]any{"task": task, "replayed": replayed}, err
}

type quotePayload struct {
	Principal Principal       `json:"principal"`
	Binding   json.RawMessage `json:"binding"`
	Expiry    int64           `json:"exp"`
}

func (s *Service) signQuote(p Principal, binding any) (string, time.Time, error) {
	if len(s.deps.QuoteKey) < 32 {
		return "", time.Time{}, errs.Internal("quote signing key unavailable")
	}
	raw, err := json.Marshal(binding)
	if err != nil {
		return "", time.Time{}, err
	}
	expiry := s.deps.Now().UTC().Add(120 * time.Second)
	body, _ := json.Marshal(quotePayload{p, raw, expiry.Unix()})
	mac := hmac.New(sha256.New, s.deps.QuoteKey)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), expiry, nil
}
func (s *Service) verifyQuote(p Principal, binding any, token string) error {
	stale := errs.New(409, "QUOTE_STALE", "quote expired or request, identity, capabilities or price changed")
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(s.deps.QuoteKey) < 32 {
		return stale
	}
	body, e1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	mac := hmac.New(sha256.New, s.deps.QuoteKey)
	mac.Write(body)
	if e1 != nil || e2 != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return stale
	}
	var payload quotePayload
	if json.Unmarshal(body, &payload) != nil || payload.Expiry <= s.deps.Now().UTC().Unix() {
		return stale
	}
	wantP, _ := json.Marshal(p)
	gotP, _ := json.Marshal(payload.Principal)
	wantB, _ := json.Marshal(binding)
	if !hmac.Equal(wantP, gotP) || !hmac.Equal(wantB, payload.Binding) {
		return stale
	}
	return nil
}
