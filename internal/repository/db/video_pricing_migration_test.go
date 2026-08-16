package db

import (
	"reflect"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestRetireLegacyVideoPricingConfigurationIsIdempotent(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:video-pricing-migration-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("legacy-video").SetName("Legacy").SetMediaType("video").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	config, err := client.VideoRouteConfig.Create().SetRouteModelID(int64(route.ID)).SetConfigVersion("legacy-v1").
		SetTaskTypes([]string{"text_to_video"}).SetVisibleOptions(map[string]any{}).SetDefaults(map[string]any{}).SetEnabled(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	costRule, err := client.VideoProviderCostRule.Create().SetAccountModelID(71).SetBillingMode("combination").SetRuleVersion(1).
		SetRatesJSON(map[string]any{"combinations": []any{map[string]any{"cost_cny": "1.50000"}}}).SetEffectiveAt(time.Now().Add(-time.Hour)).SetEnabled(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	strategy, err := client.VideoPricingStrategy.Create().SetCode("legacy").SetName("Legacy pricing").SetEnabled(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	priceRule, err := client.VideoPriceRule.Create().SetPricingStrategyID(int64(strategy.ID)).SetTaskType("text_to_video").SetResolution("720p").
		SetEffectiveAt(time.Now().Add(-time.Hour)).SetFixedTaskPoints("12.00000").SetSafetySnapshot(map[string]any{"legacy": true}).SetEnabled(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(42).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	taskID := uuid.New()
	legacySnapshot := map[string]any{
		"unit_points": "12.00000",
		"sales_rule":  map[string]any{"pricing_mode": "exact", "fixed_task_points": "12.00000", "reserve_markup": "1.00000"},
	}
	if _, err := client.VideoTask.Create().SetID(taskID).SetUserID(42).SetProjectID(project.ID).SetTaskType("text_to_video").
		SetPromptTemplate("legacy").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("legacy").SetRouteModelID(int64(route.ID)).SetRouteModelCode("legacy-video").
		SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetEstimatedPoints("12.00000").SetReservedPoints("12.00000").
		SetPricingSnapshot(legacySnapshot).SetRoutingSnapshot(map[string]any{"account_model_id": float64(71)}).SetIdempotencyKey("legacy-task").SetRequestFingerprint("legacy-fingerprint").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	storedTaskBefore, err := client.VideoTask.Get(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	pricingBefore := storedTaskBefore.PricingSnapshot
	routingBefore := storedTaskBefore.RoutingSnapshot

	if err := RetireLegacyVideoPricingConfiguration(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	config, _ = client.VideoRouteConfig.Get(t.Context(), config.ID)
	if config.Enabled {
		t.Fatal("legacy video route must be disabled")
	}
	costRule, _ = client.VideoProviderCostRule.Get(t.Context(), costRule.ID)
	strategy, _ = client.VideoPricingStrategy.Get(t.Context(), strategy.ID)
	priceRule, _ = client.VideoPriceRule.Get(t.Context(), priceRule.ID)
	if costRule.Enabled || costRule.DeletedAt == nil || strategy.Enabled || strategy.DeletedAt == nil || priceRule.Enabled || priceRule.DeletedAt == nil {
		t.Fatalf("legacy pricing rows were not retired: cost=%#v strategy=%#v price=%#v", costRule, strategy, priceRule)
	}
	storedTaskAfter, err := client.VideoTask.Get(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storedTaskAfter.PricingSnapshot, pricingBefore) || !reflect.DeepEqual(storedTaskAfter.RoutingSnapshot, routingBefore) || storedTaskAfter.ReservedPoints != "12.00000" {
		t.Fatalf("historical video task changed: before=%#v/%#v after=%#v/%#v", pricingBefore, routingBefore, storedTaskAfter.PricingSnapshot, storedTaskAfter.RoutingSnapshot)
	}
	if _, err := client.VideoRouteConfig.UpdateOne(config).SetEnabled(true).Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := RetireLegacyVideoPricingConfiguration(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	config, _ = client.VideoRouteConfig.Get(t.Context(), config.ID)
	if !config.Enabled {
		t.Fatal("completed migration must be a no-op")
	}
}
