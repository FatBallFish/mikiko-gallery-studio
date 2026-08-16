package adminvideo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const hardMaxMediaBytes int64 = 1024 * 1024 * 1024

type Store interface {
	Snapshot(context.Context) (Snapshot, error)
	ListTasks(context.Context, TaskFilter) (TaskPage, error)
	GetTask(context.Context, uuid.UUID) (TaskDetail, error)
	Retry(context.Context, RetryRequest) error
	GetMediaPolicy(context.Context) (MediaPolicy, error)
	SaveMediaPolicy(context.Context, MediaPolicy, int64) (MediaPolicy, error)
	Readiness(context.Context, time.Time) (ReadinessSnapshot, error)
	SaveCapability(context.Context, CapabilityWrite) (CapabilitySummary, error)
	ListVideoModelRateCards(context.Context, int64) ([]RateCardSummary, error)
	SaveVideoModelRateCard(context.Context, RateCardWrite) (RateCardSummary, error)
	DeleteVideoModelRateCard(context.Context, int64, int) error
	GetEffectiveVideoModelRateCard(context.Context, int64, time.Time) (RateCardSummary, error)
	GetVideoModelPricingContext(context.Context, int64) (ModelPricingContext, error)
	GetVideoRouteQuoteContext(context.Context, int64, time.Time) (RouteQuoteContext, error)
	SaveCostRule(context.Context, CostRuleWrite) (CostRuleSummary, error)
	SaveStrategy(context.Context, StrategyWrite) (PricingStrategySummary, error)
	SavePriceRule(context.Context, PriceRuleWrite) (PriceRuleSummary, error)
	SaveRouteConfig(context.Context, RouteConfigWrite) (RouteConfigSummary, error)
	DeleteVideoConfig(context.Context, ConfigKind, int64, int64) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

type CapabilitySummary struct {
	AccountModelID  int64          `json:"account_model_id"`
	Version         string         `json:"capability_version"`
	ValidationState string         `json:"validation_status"`
	Capability      map[string]any `json:"capability"`
	Enabled         bool           `json:"enabled"`
}

type CostRuleSummary struct {
	ID                int64          `json:"id"`
	AccountModelID    int64          `json:"account_model_id"`
	BillingMode       string         `json:"billing_mode"`
	RuleVersion       int            `json:"rule_version"`
	Currency          string         `json:"currency"`
	Rates             map[string]any `json:"rates"`
	CostReserveMarkup string         `json:"cost_reserve_markup"`
	SourceType        string         `json:"source_type"`
	SourceReference   string         `json:"source_reference"`
	Validation        string         `json:"validation_status"`
	EffectiveAt       time.Time      `json:"effective_at"`
	ExpiresAt         *time.Time     `json:"expires_at,omitempty"`
	Enabled           bool           `json:"enabled"`
}

type RateCardSummary struct {
	ID              int64          `json:"id"`
	AccountModelID  int64          `json:"account_model_id"`
	ProviderCode    string         `json:"provider_code"`
	PricingSchema   string         `json:"pricing_schema"`
	RateVersion     int            `json:"rate_version"`
	Currency        string         `json:"currency"`
	RateConfig      map[string]any `json:"rate_config"`
	SourceReference string         `json:"source_reference"`
	EffectiveAt     time.Time      `json:"effective_at"`
	Enabled         bool           `json:"enabled"`
}

type PricingStrategySummary struct {
	ID                          int64  `json:"id"`
	Code                        string `json:"code"`
	Name                        string `json:"name"`
	StrategyVersion             int    `json:"strategy_version"`
	MinimumNetPointIncomeCNY    string `json:"minimum_net_point_income_cny"`
	TargetMarginRate            string `json:"target_margin_rate"`
	ProviderCostBufferRate      string `json:"provider_cost_buffer_rate"`
	PaymentFeeRate              string `json:"payment_fee_rate"`
	PlatformFixedCostCNY        string `json:"platform_fixed_cost_cny"`
	PlatformOutputSecondCostCNY string `json:"platform_output_second_cost_cny"`
	PlatformReferenceCostCNY    string `json:"platform_reference_cost_cny"`
	GrossPointValueCNY          string `json:"gross_point_value_cny"`
	MaxBonusRatio               string `json:"max_bonus_ratio"`
	PlatformAudioFixedCostCNY   string `json:"platform_audio_fixed_cost_cny"`
	PlatformAudioSecondCostCNY  string `json:"platform_audio_second_cost_cny"`
	ExactReserveMarkup          string `json:"exact_reserve_markup"`
	MeteredReserveMarkup        string `json:"metered_reserve_markup"`
	Enabled                     bool   `json:"enabled"`
}

type PriceRuleSummary struct {
	ID                         int64      `json:"id"`
	StrategyID                 int64      `json:"pricing_strategy_id"`
	TaskType                   string     `json:"task_type"`
	Resolution                 string     `json:"resolution"`
	AudioMode                  string     `json:"audio_mode"`
	PricingMode                string     `json:"pricing_mode"`
	RuleVersion                int        `json:"rule_version"`
	EffectiveAt                time.Time  `json:"effective_at"`
	ExpiresAt                  *time.Time `json:"expires_at,omitempty"`
	OutputSecondPoints         string     `json:"output_second_points"`
	FixedTaskPoints            string     `json:"fixed_task_points"`
	ReferenceImagePoints       string     `json:"reference_image_points"`
	InputVideoSecondPoints     string     `json:"input_video_second_points"`
	ReferenceAudioSecondPoints string     `json:"reference_audio_second_points"`
	GeneratedAudioFixedPoints  string     `json:"generated_audio_fixed_points"`
	GeneratedAudioSecondPoints string     `json:"generated_audio_second_points"`
	MinimumBillableSeconds     int        `json:"minimum_billable_seconds"`
	MinimumTaskPoints          string     `json:"minimum_task_points"`
	ReserveMarkup              string     `json:"reserve_markup"`
	SafetyPoints               string     `json:"safety_points"`
	SalesPoints                string     `json:"sales_points"`
	CandidateCostUpperCNY      string     `json:"candidate_cost_upper_cny"`
	Enabled                    bool       `json:"enabled"`
}

type RouteConfigSummary struct {
	RouteModelID               int64          `json:"route_model_id"`
	RouteCode                  string         `json:"route_code"`
	RouteName                  string         `json:"route_name"`
	ConfigVersion              string         `json:"config_version"`
	PricingStrategyID          int64          `json:"pricing_strategy_id"`
	CandidateParameterMappings map[string]any `json:"candidate_parameter_mappings"`
	MinimumTaskPoints          string         `json:"minimum_task_points"`
	RoundingStepPoints         int            `json:"rounding_step_points"`
	CandidateCount             int            `json:"candidate_count"`
	CandidateAccountModelIDs   []int64        `json:"candidate_account_model_ids"`
	TaskTypes                  []string       `json:"task_types"`
	VisibleOptions             map[string]any `json:"visible_options"`
	Defaults                   map[string]any `json:"defaults"`
	MaxOutputCount             int            `json:"max_output_count"`
	Enabled                    bool           `json:"enabled"`
}

type Impact struct {
	RouteModelID int64  `json:"route_model_id,omitempty"`
	StrategyID   int64  `json:"pricing_strategy_id,omitempty"`
	Code         string `json:"code"`
	Summary      string `json:"summary"`
	Blocking     bool   `json:"blocking"`
	FixRoute     string `json:"fix_route"`
}

type Snapshot struct {
	Capabilities []CapabilitySummary      `json:"capabilities"`
	RateCards    []RateCardSummary        `json:"rate_cards"`
	CostRules    []CostRuleSummary        `json:"cost_rules"`
	Strategies   []PricingStrategySummary `json:"pricing_strategies"`
	PriceRules   []PriceRuleSummary       `json:"price_rules"`
	Routes       []RouteConfigSummary     `json:"routes"`
	Plans        []PointProduct           `json:"point_products"`
	Impacts      []Impact                 `json:"impacts"`
	GeneratedAt  time.Time                `json:"generated_at"`
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if s == nil || s.store == nil {
		return Snapshot{}, errs.Internal("video administration is unavailable")
	}
	snapshot, err := s.store.Snapshot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Impacts = append(snapshot.Impacts, deriveImpacts(snapshot)...)
	normalizeSnapshotCollections(&snapshot)
	if snapshot.GeneratedAt.IsZero() {
		snapshot.GeneratedAt = time.Now().UTC()
	}
	return snapshot, nil
}

func normalizeSnapshotCollections(snapshot *Snapshot) {
	if snapshot.Capabilities == nil {
		snapshot.Capabilities = []CapabilitySummary{}
	}
	if snapshot.CostRules == nil {
		snapshot.CostRules = []CostRuleSummary{}
	}
	if snapshot.RateCards == nil {
		snapshot.RateCards = []RateCardSummary{}
	}
	if snapshot.Strategies == nil {
		snapshot.Strategies = []PricingStrategySummary{}
	}
	if snapshot.PriceRules == nil {
		snapshot.PriceRules = []PriceRuleSummary{}
	}
	if snapshot.Routes == nil {
		snapshot.Routes = []RouteConfigSummary{}
	}
	if snapshot.Plans == nil {
		snapshot.Plans = []PointProduct{}
	}
	if snapshot.Impacts == nil {
		snapshot.Impacts = []Impact{}
	}
}

func deriveImpacts(snapshot Snapshot) []Impact {
	impacts := make([]Impact, 0)
	strategyRules := make(map[int64][]PriceRuleSummary)
	for _, rule := range snapshot.PriceRules {
		if rule.Enabled {
			strategyRules[rule.StrategyID] = append(strategyRules[rule.StrategyID], rule)
		}
		if !rule.Enabled {
			continue
		}
		sales, salesErr := decimal.NewFromString(rule.SalesPoints)
		safety, safetyErr := decimal.NewFromString(rule.SafetyPoints)
		if salesErr != nil || safetyErr != nil || sales.LessThan(safety) {
			summary := "价格规则无法证明满足候选最坏成本安全线"
			if salesErr == nil && safetyErr == nil {
				summary = "价格策略 " + decimal.NewFromInt(rule.StrategyID).String() + " 的 " + rule.TaskType + " / " + rule.Resolution + " / " + rule.AudioMode + " 组合售价 " + sales.String() + " 积分，低于安全线 " + safety.String() + " 积分"
			}
			impacts = append(impacts, Impact{StrategyID: rule.StrategyID, Code: "price_below_safety_floor", Summary: summary, Blocking: true, FixRoute: "pricing"})
		}
	}
	for _, route := range snapshot.Routes {
		if !route.Enabled {
			continue
		}
		if route.CandidateCount == 0 {
			impacts = append(impacts, Impact{RouteModelID: route.RouteModelID, StrategyID: route.PricingStrategyID, Code: "missing_candidate", Summary: "启用的视频路由没有可用候选", Blocking: true, FixRoute: "routing"})
		}
		combinations := visibleCombinations(route.VisibleOptions)
		bindings, _ := decodePricingBindings(route.VisibleOptions)
		if len(combinations) == 0 && len(strategyRules[route.PricingStrategyID]) == 0 {
			impacts = append(impacts, Impact{RouteModelID: route.RouteModelID, StrategyID: route.PricingStrategyID, Code: "missing_price", Summary: "路由 " + routeImpactName(route) + " 没有可用销售价格", Blocking: true, FixRoute: "pricing"})
		}
		for _, combo := range combinations {
			strategyID := pricingStrategyForCombination(route.PricingStrategyID, bindings, combo)
			if hasPriceForCombination(strategyRules[strategyID], combo) {
				continue
			}
			impacts = append(impacts, Impact{
				RouteModelID: route.RouteModelID, StrategyID: strategyID, Code: "missing_price",
				Summary:  "路由 " + routeImpactName(route) + " 缺少 " + combo.TaskType + " / " + combo.Resolution + " / " + combo.AudioMode + " / " + fmt.Sprintf("%d", combo.DurationSeconds) + " 秒的销售价格",
				Blocking: true, FixRoute: "pricing",
			})
		}
	}
	return impacts
}

func visibleCombinations(options map[string]any) []VisibleCombination {
	raw, ok := options["combinations"]
	if !ok || raw == nil {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var combinations []VisibleCombination
	if err := json.Unmarshal(payload, &combinations); err != nil {
		return nil
	}
	return combinations
}

func hasPriceForCombination(rules []PriceRuleSummary, combo VisibleCombination) bool {
	for _, rule := range rules {
		if rule.TaskType == combo.TaskType && rule.Resolution == combo.Resolution && rule.AudioMode == combo.AudioMode {
			return true
		}
	}
	return false
}

func routeImpactName(route RouteConfigSummary) string {
	if strings.TrimSpace(route.RouteName) != "" {
		return strings.TrimSpace(route.RouteName)
	}
	if strings.TrimSpace(route.RouteCode) != "" {
		return strings.TrimSpace(route.RouteCode)
	}
	return fmt.Sprintf("#%d", route.RouteModelID)
}

type TaskFilter struct {
	UserID         int64
	TaskID         *uuid.UUID
	ProviderTaskID string
	ProjectID      *uuid.UUID
	RouteModelID   int64
	AccountModelID int64
	Status         string
	From           *time.Time
	To             *time.Time
	Cursor         string
	Limit          int
}

type TaskSummary struct {
	ID               uuid.UUID `json:"id"`
	UserID           int64     `json:"user_id"`
	ProjectID        uuid.UUID `json:"project_id"`
	RouteModelID     int64     `json:"route_model_id"`
	RouteModelCode   string    `json:"route_model_code"`
	Status           string    `json:"status"`
	SettlementStatus string    `json:"settlement_status"`
	EstimatedPoints  string    `json:"estimated_points"`
	ActualPoints     string    `json:"actual_points"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TaskPage struct {
	Items      []TaskSummary `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type Attempt struct {
	ID              uuid.UUID      `json:"id"`
	ItemID          uuid.UUID      `json:"item_id"`
	AttemptNo       int            `json:"attempt_no"`
	ProviderCode    string         `json:"provider_code"`
	ModelCode       string         `json:"model_code"`
	ProviderJobID   string         `json:"provider_job_id,omitempty"`
	Status          string         `json:"status"`
	UsageRaw        map[string]any `json:"usage_raw"`
	UsageNormalized map[string]any `json:"usage_normalized"`
	CostSnapshot    map[string]any `json:"cost_snapshot"`
	ProviderCost    string         `json:"provider_cost"`
	ErrorCategory   string         `json:"error_category,omitempty"`
	ErrorCode       string         `json:"error_code,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
}

type TaskItem struct {
	ID               uuid.UUID      `json:"id"`
	Ordinal          int            `json:"ordinal"`
	Status           string         `json:"status"`
	Stage            string         `json:"stage"`
	ResultAssetID    *uuid.UUID     `json:"result_asset_id,omitempty"`
	ActualPoints     string         `json:"actual_points"`
	ProviderCost     string         `json:"provider_cost"`
	ArtifactSnapshot map[string]any `json:"artifact_snapshot"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	Attempts         []Attempt      `json:"attempts"`
}

type TaskDetail struct {
	TaskSummary
	PricingSnapshot map[string]any `json:"pricing_snapshot"`
	RoutingSnapshot map[string]any `json:"routing_snapshot"`
	ReservedPoints  string         `json:"reserved_points"`
	Items           []TaskItem     `json:"items"`
}

func (s *Service) ListTasks(ctx context.Context, filter TaskFilter) (TaskPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	return s.store.ListTasks(ctx, filter)
}

func (s *Service) GetTask(ctx context.Context, id uuid.UUID) (TaskDetail, error) {
	if id == uuid.Nil {
		return TaskDetail{}, errs.BadRequest("video task id is required")
	}
	return s.store.GetTask(ctx, id)
}

type RetryKind string

const (
	RetryArtifact   RetryKind = "artifact"
	RetryDerivative RetryKind = "derivative"
	RetrySettlement RetryKind = "settlement"
)

type RetryRequest struct {
	Kind   RetryKind
	TaskID uuid.UUID
	ItemID uuid.UUID
	JobID  uuid.UUID
}

func (s *Service) Retry(ctx context.Context, request RetryRequest) error {
	switch request.Kind {
	case RetryArtifact:
		if request.TaskID == uuid.Nil {
			return errs.BadRequest("video task id is required")
		}
	case RetryDerivative:
		if request.JobID == uuid.Nil {
			return errs.BadRequest("media processing job id is required")
		}
	case RetrySettlement:
		if request.TaskID == uuid.Nil {
			return errs.BadRequest("video task id is required")
		}
	default:
		return errs.BadRequest("only artifact, derivative, and settlement recovery are allowed")
	}
	return s.store.Retry(ctx, request)
}

type MediaPolicy struct {
	Version                       int64               `json:"version"`
	AllowedFormats                map[string][]string `json:"allowed_formats"`
	SingleFileMaxBytes            int64               `json:"single_file_max_bytes"`
	VideoMaxDurationSeconds       int                 `json:"video_max_duration_seconds"`
	UserQuotaBytes                int64               `json:"user_quota_bytes"`
	ImageThumbnailWidths          []int               `json:"image_thumbnail_widths"`
	VideoPosterEnabled            bool                `json:"video_poster_enabled"`
	VideoHoverPreviewEnabled      bool                `json:"video_hover_preview_enabled"`
	VideoProxyEnabled             bool                `json:"video_proxy_enabled"`
	AudioProxyEnabled             bool                `json:"audio_proxy_enabled"`
	AudioWaveformEnabled          bool                `json:"audio_waveform_enabled"`
	UploadSessionTTLHours         int                 `json:"upload_session_ttl_hours"`
	FailedProcessingRetentionDays int                 `json:"failed_processing_retention_days"`
	SoftDeleteRetentionDays       int                 `json:"soft_delete_retention_days"`
	AppliesTo                     string              `json:"applies_to"`
}

type RuntimeMediaPolicy struct {
	Policy         domainmedia.Policy
	UserQuotaBytes int64
	UploadTTL      time.Duration
}

func (policy MediaPolicy) RuntimePolicy() RuntimeMediaPolicy {
	runtimePolicy := domainmedia.DefaultPolicy()
	runtimePolicy.SingleFileMaxBytes = policy.SingleFileMaxBytes
	runtimePolicy.VideoMaxDurationMS = int64(policy.VideoMaxDurationSeconds) * int64(time.Second/time.Millisecond)
	runtimePolicy.ImageThumbnailWidths = append([]int(nil), policy.ImageThumbnailWidths...)
	runtimePolicy.VideoPosterEnabled = policy.VideoPosterEnabled
	runtimePolicy.VideoHoverPreviewEnabled = policy.VideoHoverPreviewEnabled
	runtimePolicy.VideoProxyEnabled = policy.VideoProxyEnabled
	runtimePolicy.AudioProxyEnabled = policy.AudioProxyEnabled
	runtimePolicy.AudioWaveformEnabled = policy.AudioWaveformEnabled
	for mediaType, formats := range policy.AllowedFormats {
		runtimePolicy.AllowedFormats[domainmedia.MediaType(mediaType)] = append([]string(nil), formats...)
	}
	return RuntimeMediaPolicy{
		Policy: runtimePolicy, UserQuotaBytes: policy.UserQuotaBytes,
		UploadTTL: time.Duration(policy.UploadSessionTTLHours) * time.Hour,
	}
}

func DefaultMediaPolicy() MediaPolicy {
	return MediaPolicy{
		Version:            1,
		AllowedFormats:     map[string][]string{"image": {"jpg", "jpeg", "png", "webp", "heic", "heif", "bmp", "tiff", "gif"}, "video": {"mp4", "mov"}, "audio": {"mp3", "m4a", "wav"}},
		SingleFileMaxBytes: hardMaxMediaBytes, UserQuotaBytes: 20 * hardMaxMediaBytes,
		ImageThumbnailWidths: []int{320, 640, 1280}, VideoPosterEnabled: true, VideoHoverPreviewEnabled: true, VideoProxyEnabled: true,
		AudioProxyEnabled: true, AudioWaveformEnabled: true, UploadSessionTTLHours: 24, FailedProcessingRetentionDays: 7, SoftDeleteRetentionDays: 7,
		AppliesTo: "new_objects_and_derivative_versions",
	}
}

func (s *Service) GetMediaPolicy(ctx context.Context) (MediaPolicy, error) {
	return s.store.GetMediaPolicy(ctx)
}

func (s *Service) UpdateMediaPolicy(ctx context.Context, expectedVersion int64, policy MediaPolicy, updatedBy int64) (MediaPolicy, error) {
	current, err := s.store.GetMediaPolicy(ctx)
	if err != nil {
		return MediaPolicy{}, err
	}
	if expectedVersion != current.Version || policy.Version != current.Version {
		return MediaPolicy{}, errs.New(409, errs.CodeConflict, "media policy version conflict")
	}
	if err := validateMediaPolicy(policy); err != nil {
		return MediaPolicy{}, err
	}
	policy.Version++
	policy.AppliesTo = "new_objects_and_derivative_versions"
	return s.store.SaveMediaPolicy(ctx, policy, updatedBy)
}

func validateMediaPolicy(policy MediaPolicy) error {
	if policy.SingleFileMaxBytes <= 0 || policy.SingleFileMaxBytes > hardMaxMediaBytes {
		return errs.BadRequest("single_file_max_bytes must be between 1 and 1 GiB")
	}
	if policy.VideoMaxDurationSeconds < 0 || policy.UserQuotaBytes <= 0 {
		return errs.BadRequest("media limits must not be negative")
	}
	if policy.UploadSessionTTLHours < 1 || policy.UploadSessionTTLHours > 72 {
		return errs.BadRequest("upload_session_ttl_hours must be between 1 and 72")
	}
	for _, mediaType := range []string{"image", "video", "audio"} {
		formats := policy.AllowedFormats[mediaType]
		if len(formats) == 0 {
			return errs.BadRequest("allowed formats are required for " + mediaType)
		}
		for _, format := range formats {
			if strings.TrimSpace(format) == "" {
				return errs.BadRequest("media formats must not be empty")
			}
		}
	}
	return nil
}

type ReadinessSnapshot struct {
	EnabledVideoRoutes        int `json:"enabled_video_routes"`
	RoutesMissingCandidate    int `json:"routes_missing_candidate"`
	VisibleCombosMissingPrice int `json:"visible_combinations_missing_price"`
	ArtifactBacklog           int `json:"artifact_backlog"`
	DerivativeBacklog         int `json:"derivative_backlog"`
	SettlementBacklog         int `json:"settlement_backlog"`
}

func (s *Service) Readiness(ctx context.Context, now time.Time) (ReadinessSnapshot, error) {
	return s.store.Readiness(ctx, now)
}
