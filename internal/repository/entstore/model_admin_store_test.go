package entstore_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	_ "github.com/mattn/go-sqlite3"
)

func TestModelAdminStoreAuditedDeleteRollsBackWhenAuditWriteFails(t *testing.T) {
	client := openModelAdminTestClient(t, "model-admin-audit-rollback")
	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(t.Context(), domainmodeladmin.ModelAccountWriteRequest{
		Name: "audit rollback", AdapterType: "openai_compatible", AuthType: "api_key", BaseURL: "https://example.com", Status: "enabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	client.AuditLog.Use(func(repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(context.Context, repoent.Mutation) (repoent.Value, error) {
			return nil, errors.New("injected audit failure")
		})
	})
	err = store.DeleteModelAccountAudited(t.Context(), account.ID, domainmodeladmin.LifecycleAudit{
		ActorType: "admin", ActorID: "1", Action: "model_account.delete", TargetType: "model_account", TargetID: "1",
		RequestID: "audit-rollback-request", IPAddr: "127.0.0.1", UserAgent: "test-agent",
	})
	if err == nil || !strings.Contains(err.Error(), "injected audit failure") {
		t.Fatalf("expected injected audit failure, got %v", err)
	}
	if _, err := store.GetModelAccount(t.Context(), account.ID); err != nil {
		t.Fatalf("account deletion must roll back when audit fails: %v", err)
	}
}

func TestModelAdminStoreLifecycleUsesDependencyAwareSoftDeletion(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-lifecycle?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "lifecycle", AdapterType: "openai_compatible", AuthType: "api_key", BaseURL: "https://example.com",
		Status: "enabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	accountModel, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-image-lifecycle", DisplayName: "Lifecycle", TaskTypes: []string{"text_to_image"},
		SizeModes: []string{"auto"}, MaxImageCount: 1, CostPerImage: "0.10000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}
	route, err := store.CreateRouteModel(ctx, domainmodeladmin.RouteModelWriteRequest{Code: "lifecycle", Name: "Lifecycle", Visibility: "public", Enabled: true})
	if err != nil {
		t.Fatalf("create route model: %v", err)
	}
	candidate, err := store.CreateRouteModelCandidate(ctx, domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: route.ID, AccountModelID: accountModel.ID, Weight: 100, Enabled: true})
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	price, err := store.CreateRouteModelPrice(ctx, domainmodeladmin.RouteModelPriceWriteRequest{RouteModelID: route.ID, TaskType: "text_to_image", BaseResolution: "1K", BasePoints: "8.00000", ReferenceMultiplier: "1.00000", Enabled: true})
	if err != nil {
		t.Fatalf("create price: %v", err)
	}

	assertConfigurationInUse := func(err error, dependency string, count int) {
		t.Helper()
		if !errors.Is(err, repoerr.ErrConfigurationInUse) {
			t.Fatalf("error = %v, want configuration in use", err)
		}
		actualDependency, actualCount, ok := repoerr.ConfigurationInUseDetails(err)
		if !ok || actualDependency != dependency || actualCount != count {
			t.Fatalf("conflict details = (%q,%d,%t), want (%q,%d,true)", actualDependency, actualCount, ok, dependency, count)
		}
	}
	assertConfigurationInUse(store.DeleteModelAccount(ctx, account.ID), "account_models", 1)
	assertConfigurationInUse(store.DeleteModelAccountModel(ctx, accountModel.ID), "route_candidates", 1)
	assertConfigurationInUse(store.DeleteRouteModel(ctx, route.ID), "route_candidates", 1)

	if err := store.DeleteRouteModelCandidate(ctx, candidate.ID); err != nil {
		t.Fatalf("delete candidate: %v", err)
	}
	if err := store.DeleteRouteModelCandidate(ctx, candidate.ID); err != nil {
		t.Fatalf("repeat candidate delete must be idempotent: %v", err)
	}
	listedCandidates, err := store.ListRouteModelCandidates(ctx, route.ID)
	if err != nil || len(listedCandidates) != 0 {
		t.Fatalf("deleted candidate remained visible: %#v err=%v", listedCandidates, err)
	}
	deletedCandidate, err := client.RouteModelCandidate.Get(ctx, int(candidate.ID))
	if err != nil || deletedCandidate.DeletedAt == nil {
		t.Fatalf("candidate tombstone = %#v err=%v", deletedCandidate, err)
	}
	revivedCandidate, err := store.CreateRouteModelCandidate(ctx, domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: route.ID, AccountModelID: accountModel.ID, Weight: 90, Enabled: true})
	if err != nil || revivedCandidate.ID != candidate.ID {
		t.Fatalf("recreate must revive candidate id=%d: %#v err=%v", candidate.ID, revivedCandidate, err)
	}
	if err := store.DeleteRouteModelCandidate(ctx, candidate.ID); err != nil {
		t.Fatalf("delete revived candidate: %v", err)
	}

	assertConfigurationInUse(store.DeleteRouteModel(ctx, route.ID), "route_prices", 1)
	if err := store.DeleteRouteModelPrice(ctx, price.ID); err != nil {
		t.Fatalf("delete price: %v", err)
	}
	if err := store.DeleteRouteModelPrice(ctx, price.ID); err != nil {
		t.Fatalf("repeat price delete must be idempotent: %v", err)
	}
	listedPrices, err := store.ListRouteModelPrices(ctx, domainmodeladmin.RouteModelPriceListRequest{Page: 1, PageSize: 10, RouteModelID: route.ID})
	if err != nil || listedPrices.Total != 0 {
		t.Fatalf("deleted price remained visible: %#v err=%v", listedPrices, err)
	}
	deletedPrice, err := client.RouteModelPrice.Get(ctx, int(price.ID))
	if err != nil || deletedPrice.DeletedAt == nil {
		t.Fatalf("price tombstone = %#v err=%v", deletedPrice, err)
	}
	revivedPrice, err := store.CreateRouteModelPrice(ctx, domainmodeladmin.RouteModelPriceWriteRequest{RouteModelID: route.ID, TaskType: "text_to_image", BaseResolution: "1K", BasePoints: "9.00000", ReferenceMultiplier: "1.00000", Enabled: true})
	if err != nil || revivedPrice.ID != price.ID || revivedPrice.BasePoints != "9.00000" {
		t.Fatalf("recreate must revive price id=%d: %#v err=%v", price.ID, revivedPrice, err)
	}
	if err := store.DeleteRouteModelPrice(ctx, price.ID); err != nil {
		t.Fatalf("delete revived price: %v", err)
	}

	for _, step := range []struct {
		name     string
		deleteFn func() error
	}{
		{"route", func() error { return store.DeleteRouteModel(ctx, route.ID) }},
		{"account model", func() error { return store.DeleteModelAccountModel(ctx, accountModel.ID) }},
		{"account", func() error { return store.DeleteModelAccount(ctx, account.ID) }},
	} {
		name, deleteFn := step.name, step.deleteFn
		if err := deleteFn(); err != nil {
			t.Fatalf("delete %s: %v", name, err)
		}
		if err := deleteFn(); err != nil {
			t.Fatalf("repeat %s delete must be idempotent: %v", name, err)
		}
	}
}

func TestModelAdminStorePersistsProvidersRoutesAndRuntimeSnapshot(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladminstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	openAI, err := store.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "openai", ProviderType: "openai", HealthStatus: "healthy", Enabled: false})
	if err != nil {
		t.Fatalf("CreateProvider openai: %v", err)
	}
	openRouter, err := store.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "openrouter", ProviderType: "openrouter", HealthStatus: "healthy", Enabled: true})
	if err != nil {
		t.Fatalf("CreateProvider openrouter: %v", err)
	}
	if openAI.ID == 0 || openRouter.ID == 0 {
		t.Fatalf("expected provider ids, got %#v %#v", openAI, openRouter)
	}
	providerModels, err := store.ListProviderModels(ctx, domainmodeladmin.ProviderModelListRequest{Page: 1, PageSize: 10, ProviderCode: "openrouter"})
	if err != nil {
		t.Fatalf("ListProviderModels default: %v", err)
	}
	if providerModels.Total != 1 || len(providerModels.Items) != 1 || providerModels.Items[0].ProviderCode != "openrouter" {
		t.Fatalf("expected default provider model, got %#v", providerModels)
	}
	explicitModel, err := store.CreateProviderModel(ctx, domainmodeladmin.ProviderModelWriteRequest{
		ProviderCode:            "openrouter",
		ModelCode:               "openrouter/vision",
		CompatMode:              "openrouter_chat_image",
		SupportsImageInput:      true,
		SupportedBaseResolution: []string{"1k", "2k"},
		MaxImageCount:           4,
		MaxReferenceImageCount:  2,
		TimeoutMS:               45000,
		InputCost:               "0.12",
		OutputCost:              "0.34",
		Currency:                "USD",
		HealthStatus:            "healthy",
		Enabled:                 true,
	})
	if err != nil {
		t.Fatalf("CreateProviderModel explicit: %v", err)
	}
	if explicitModel.ID == 0 || explicitModel.ModelCode != "openrouter/vision" {
		t.Fatalf("unexpected explicit model %#v", explicitModel)
	}
	route, err := store.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{
		GroupCode:       "plus",
		TaskType:        "text_to_image",
		ProviderModelID: explicitModel.ID,
		Priority:        0,
		FallbackOrder:   1,
		WeightPercent:   100,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if route.ProviderModelID != explicitModel.ID || route.ProviderCode != "openrouter" {
		t.Fatalf("route should store provider id compatibility, got %#v", route)
	}

	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("ModelRoutingConfig: %v", err)
	}
	if len(snapshot.Providers) != 2 || len(snapshot.Routes) != 1 {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	if snapshot.Routes[0].ProviderCode != "openrouter" {
		t.Fatalf("expected provider code in runtime route, got %#v", snapshot.Routes[0])
	}
	if err := store.DeleteProvider(ctx, "openrouter"); err == nil {
		t.Fatal("expected provider with routes to fail deletion")
	}

	if err := store.DeleteRoute(ctx, route.ID); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
	if err := store.DeleteProvider(ctx, "openrouter"); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
}

func TestModelAdminStoreMapsAccountModelCostToRuntimeOutputCost(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-account-cost?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{Name: "paid", AdapterType: "openrouter", AuthType: "api_key", BaseURL: "https://example.com", Status: "enabled", Priority: 1, Weight: 100, ConcurrencyLimit: 1, TimeoutMS: 30000})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{AccountID: account.ID, ModelCode: "paid/image", DisplayName: "Paid Image", TaskTypes: []string{"text_to_image"}, Quality: []string{"auto"}, MaxImageCount: 1, CostPerImage: "0.12345", Currency: "USD", Enabled: true})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}
	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("runtime snapshot: %v", err)
	}
	for _, candidate := range snapshot.ProviderModels {
		if candidate.AccountModelID == model.ID {
			if candidate.OutputCost != "0.12345" {
				t.Fatalf("account model cost must be runtime output cost, got %#v", candidate)
			}
			if len(candidate.SupportedAspectRatios) != 0 || candidate.MaxImageCount != 1 || candidate.MaxReferenceImageCount != 0 || candidate.SupportsImageInput {
				t.Fatalf("runtime mapping must not invent aspect ratios or lose safe limits: %#v", candidate)
			}
			if candidate.ConcurrencyLimit != account.ConcurrencyLimit {
				t.Fatalf("model account concurrency must be propagated to runtime scheduling, got %#v", candidate)
			}
			return
		}
	}
	t.Fatalf("runtime snapshot missing account model %d: %#v", model.ID, snapshot.ProviderModels)
}

func TestModelAdminStoreMapsAccountModelGenerationLimitsToRuntimeSnapshot(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-account-capabilities?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "image-account", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://example.com", Status: "enabled", Priority: 1, Weight: 100,
		ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-image-2", DisplayName: "GPT Image 2",
		TaskTypes: []string{"text_to_image"}, Quality: []string{"auto", "high"},
		SupportedRatios: []string{"1:1", "16:9"}, MaxImageCount: 2, MaxReferenceImageCount: 3,
		CostPerImage: "0.04000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}

	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("runtime snapshot: %v", err)
	}
	for _, candidate := range snapshot.ProviderModels {
		if candidate.AccountModelID != model.ID {
			continue
		}
		wantRatios := []string{"1:1", "16:9"}
		if !reflect.DeepEqual(candidate.SupportedAspectRatios, wantRatios) {
			t.Fatalf("runtime candidate ratios = %#v, want %#v", candidate.SupportedAspectRatios, wantRatios)
		}
		if candidate.MaxImageCount != 2 || candidate.MaxReferenceImageCount != 3 || !candidate.SupportsImageInput {
			t.Fatalf("runtime candidate limits = output:%d reference:%d supports_input:%t, want output:2 reference:3 supports_input:true", candidate.MaxImageCount, candidate.MaxReferenceImageCount, candidate.SupportsImageInput)
		}
		return
	}
	t.Fatalf("runtime snapshot missing account model %d: %#v", model.ID, snapshot.ProviderModels)
}

func TestModelAdminStoreNormalizesOnlyLegacyDefaultSizeBoundsForEditing(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-legacy-size-bounds?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "legacy-bounds", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://example.com", Status: "disabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	legacy, err := client.ModelAccountModel.Create().
		SetAccountID(account.ID).
		SetModelCode("legacy-4096").
		SetDisplayName("Legacy 4096").
		SetTaskTypes([]string{"text_to_image"}).
		SetMaxImageCount(1).
		SetMaxWidth(4096).
		SetMaxHeight(4096).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model: %v", err)
	}
	explicitInvalid, err := client.ModelAccountModel.Create().
		SetAccountID(account.ID).
		SetModelCode("explicit-4000").
		SetDisplayName("Explicit 4000").
		SetTaskTypes([]string{"text_to_image"}).
		SetMaxImageCount(1).
		SetMaxWidth(4000).
		SetMaxHeight(4000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create explicit invalid model: %v", err)
	}

	svc := modeladminservice.NewServiceWithStore(store)
	read, err := svc.GetModelAccountModel(ctx, int64(legacy.ID))
	if err != nil {
		t.Fatalf("read legacy model: %v", err)
	}
	if read.MaxWidth != 3840 || read.MaxHeight != 3840 {
		t.Fatalf("legacy defaults must be exposed as editable legal bounds: %#v", read)
	}
	updated, err := svc.UpdateModelAccountModel(ctx, int64(legacy.ID), modelAccountModelWriteRequest(read))
	if err != nil {
		t.Fatalf("update normalized legacy model: %v", err)
	}
	if updated.MaxWidth != 3840 || updated.MaxHeight != 3840 {
		t.Fatalf("updated legacy model has illegal bounds: %#v", updated)
	}
	persisted, err := client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload updated legacy model: %v", err)
	}
	if persisted.MaxWidth != 3840 || persisted.MaxHeight != 3840 {
		t.Fatalf("normalized bounds were not persisted: %#v", persisted)
	}

	invalidRead, err := svc.GetModelAccountModel(ctx, int64(explicitInvalid.ID))
	if err != nil {
		t.Fatalf("read explicit invalid model: %v", err)
	}
	if invalidRead.MaxWidth != 4000 || invalidRead.MaxHeight != 4000 {
		t.Fatalf("non-default invalid bounds must remain visible: %#v", invalidRead)
	}
	if _, err := svc.UpdateModelAccountModel(ctx, int64(explicitInvalid.ID), modelAccountModelWriteRequest(invalidRead)); err == nil {
		t.Fatal("explicit invalid bounds must fail validation instead of being silently clamped")
	}
}

func TestModelAdminStoreRejectsDeletedParentsAndTombstoneUpdates(t *testing.T) {
	ctx := t.Context()
	client := openModelAdminTestClient(t, "model-admin-deleted-parent-writes")
	store := entstore.NewModelAdminStore(client)
	accountRequest := domainmodeladmin.ModelAccountWriteRequest{
		Name: "account", AdapterType: "openai_compatible", AuthType: "api_key", BaseURL: "https://example.com", Status: "enabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	}
	sourceAccount, err := store.CreateModelAccount(ctx, accountRequest)
	if err != nil {
		t.Fatalf("create source account: %v", err)
	}
	targetAccount, err := store.CreateModelAccount(ctx, accountRequest)
	if err != nil {
		t.Fatalf("create target account: %v", err)
	}
	if err := store.DeleteModelAccount(ctx, targetAccount.ID); err != nil {
		t.Fatalf("delete target account: %v", err)
	}
	accountRequest.Name = "updated deleted account"
	if _, err := store.UpdateModelAccount(ctx, targetAccount.ID, accountRequest); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("update deleted account = %v, want not found", err)
	}

	modelRequest := domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: sourceAccount.ID, ModelCode: "gpt-image-parent-lock", DisplayName: "Parent lock", TaskTypes: []string{"text_to_image"}, SizeModes: []string{"auto"}, MaxImageCount: 1, CostPerImage: "0.10000", Currency: "USD", Enabled: true,
	}
	model, err := store.CreateModelAccountModel(ctx, modelRequest)
	if err != nil {
		t.Fatalf("create source model: %v", err)
	}
	createOnDeleted := modelRequest
	createOnDeleted.AccountID = targetAccount.ID
	createOnDeleted.ModelCode = "gpt-image-deleted-parent"
	if _, err := store.CreateModelAccountModel(ctx, createOnDeleted); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("create model on deleted account = %v, want conflict", err)
	}
	moveToDeleted := modelAccountModelWriteRequest(model)
	moveToDeleted.AccountID = targetAccount.ID
	if _, err := store.UpdateModelAccountModel(ctx, model.ID, moveToDeleted); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("move model to deleted account = %v, want conflict", err)
	}
	reloadedModel, err := store.GetModelAccountModel(ctx, model.ID)
	if err != nil || reloadedModel.AccountID != sourceAccount.ID {
		t.Fatalf("failed model move changed parent: %#v err=%v", reloadedModel, err)
	}

	disposableModelRequest := modelRequest
	disposableModelRequest.ModelCode = "gpt-image-tombstone"
	disposableModel, err := store.CreateModelAccountModel(ctx, disposableModelRequest)
	if err != nil {
		t.Fatalf("create disposable model: %v", err)
	}
	if err := store.DeleteModelAccountModel(ctx, disposableModel.ID); err != nil {
		t.Fatalf("delete disposable model: %v", err)
	}
	if _, err := store.UpdateModelAccountModel(ctx, disposableModel.ID, disposableModelRequest); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("update deleted model = %v, want not found", err)
	}

	routeRequest := domainmodeladmin.RouteModelWriteRequest{Code: "source-route", Name: "Source route", Visibility: "public", Enabled: true}
	sourceRoute, err := store.CreateRouteModel(ctx, routeRequest)
	if err != nil {
		t.Fatalf("create source route: %v", err)
	}
	targetRoute, err := store.CreateRouteModel(ctx, domainmodeladmin.RouteModelWriteRequest{Code: "target-route", Name: "Target route", Visibility: "public", Enabled: true})
	if err != nil {
		t.Fatalf("create target route: %v", err)
	}
	if err := store.DeleteRouteModel(ctx, targetRoute.ID); err != nil {
		t.Fatalf("delete target route: %v", err)
	}
	if _, err := store.UpdateRouteModel(ctx, targetRoute.ID, domainmodeladmin.RouteModelWriteRequest{Code: "deleted-route", Name: "Deleted", Visibility: "public"}); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("update deleted route = %v, want not found", err)
	}

	candidateRequest := domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: sourceRoute.ID, AccountModelID: model.ID, Weight: 100, Enabled: true}
	candidate, err := store.CreateRouteModelCandidate(ctx, candidateRequest)
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	moveCandidate := candidateRequest
	moveCandidate.RouteModelID = targetRoute.ID
	if _, err := store.UpdateRouteModelCandidate(ctx, candidate.ID, moveCandidate); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("move candidate to deleted route = %v, want conflict", err)
	}
	if err := store.DeleteRouteModelCandidate(ctx, candidate.ID); err != nil {
		t.Fatalf("delete candidate: %v", err)
	}
	if _, err := store.UpdateRouteModelCandidate(ctx, candidate.ID, candidateRequest); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("update deleted candidate = %v, want not found", err)
	}

	priceRequest := domainmodeladmin.RouteModelPriceWriteRequest{RouteModelID: sourceRoute.ID, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1.00000", ReferenceMultiplier: "1.00000", Enabled: true}
	price, err := store.CreateRouteModelPrice(ctx, priceRequest)
	if err != nil {
		t.Fatalf("create price: %v", err)
	}
	movePrice := priceRequest
	movePrice.RouteModelID = targetRoute.ID
	if _, err := store.UpdateRouteModelPrice(ctx, price.ID, movePrice); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("move price to deleted route = %v, want conflict", err)
	}
	if err := store.DeleteRouteModelPrice(ctx, price.ID); err != nil {
		t.Fatalf("delete price: %v", err)
	}
	if _, err := store.UpdateRouteModelPrice(ctx, price.ID, priceRequest); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("update deleted price = %v, want not found", err)
	}
}

func modelAccountModelWriteRequest(model domainmodeladmin.ModelAccountModel) domainmodeladmin.ModelAccountModelWriteRequest {
	return domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: model.AccountID, ModelCode: model.ModelCode, DisplayName: model.DisplayName,
		TaskTypes: model.TaskTypes, BaseResolution: model.BaseResolution, Quality: model.Quality,
		MaxReferenceImageCount: model.MaxReferenceImageCount, MaxImageCount: model.MaxImageCount,
		SizeModes: model.SizeModes, SupportedRatios: model.SupportedRatios,
		SupportedPixelSizes: model.SupportedPixelSizes, SupportsCustomRatio: model.SupportsCustomRatio,
		OutputFormat: model.OutputFormat, SupportedBackgrounds: model.SupportedBackgrounds,
		OutputCompression: model.OutputCompression, SupportsOutputCompression: model.SupportsOutputCompression,
		SupportsCustomSize: model.SupportsCustomSize, MinWidth: model.MinWidth, MaxWidth: model.MaxWidth,
		MinHeight: model.MinHeight, MaxHeight: model.MaxHeight, Moderation: model.Moderation,
		CostPerImage: model.CostPerImage, Currency: model.Currency, Enabled: model.Enabled, Extra: model.Extra,
	}
}

func openModelAdminTestClient(t *testing.T, name string) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:"+name+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}
