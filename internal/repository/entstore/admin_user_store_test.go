package entstore_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminUserStoreListDetailAndStatus(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminuserstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	group, err := client.UserGroup.Create().
		SetGroupCode("basic").
		SetGroupName("Basic").
		SetMultiplier("1.00000").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user group: %v", err)
	}
	alice, err := client.User.Create().
		SetEmail("alice@example.com").
		SetNickname("Alice").
		SetStatus("active").
		SetUserGroupID(int64(group.ID)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := client.User.Create().
		SetEmail("bob@example.com").
		SetNickname("Bob").
		SetStatus("disabled").
		SetUserGroupID(int64(group.ID)).
		Save(ctx); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	billingSvc := billingservice.NewServiceWithStore(testBillingConfig(), entstore.NewBillingStore(client, 5))
	if _, err := billingSvc.AdminAdjust(ctx, domainbilling.AdjustRequest{
		UserID:          int64(alice.ID),
		ChangePoints:    "15.00000",
		Reason:          "seed balance",
		OperatorAdminID: 7,
		IdempotencyKey:  "seed-alice",
	}); err != nil {
		t.Fatalf("AdminAdjust seed: %v", err)
	}

	store := entstore.NewAdminUserStore(client, entstore.NewBillingStore(client, 5))
	list, err := store.ListUsers(ctx, domainadminuser.ListRequest{Page: 1, PageSize: 10, Query: "ali", Status: "active"})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Email != "alice@example.com" {
		t.Fatalf("unexpected filtered list %#v", list)
	}
	if list.Items[0].Balance != "15.00000" {
		t.Fatalf("expected list item balance, got %#v", list.Items[0])
	}
	groupList, err := store.ListUsers(ctx, domainadminuser.ListRequest{Page: 1, PageSize: 10, GroupCode: "basic", SortBy: "points", SortDir: "desc"})
	if err != nil {
		t.Fatalf("ListUsers group/sort: %v", err)
	}
	if groupList.Total != 2 || groupList.Items[0].Email != "alice@example.com" {
		t.Fatalf("unexpected group/sort list %#v", groupList)
	}

	detail, err := store.GetUserDetail(ctx, int64(alice.ID), 5)
	if err != nil {
		t.Fatalf("GetUserDetail: %v", err)
	}
	if detail.User.Email != "alice@example.com" || detail.Balance.AvailablePoints != "15.00000" {
		t.Fatalf("unexpected detail %#v", detail)
	}
	if len(detail.RecentLedger) != 1 || detail.RecentLedger[0].LedgerType != "admin_adjust" {
		t.Fatalf("expected recent ledger to include admin adjustment, got %#v", detail.RecentLedger)
	}

	updated, err := store.UpdateUserStatus(ctx, int64(alice.ID), "disabled")
	if err != nil {
		t.Fatalf("UpdateUserStatus: %v", err)
	}
	if updated.Status != "disabled" || updated.TokenVersion != 1 {
		t.Fatalf("expected status update to increment token version, got %#v", updated)
	}

	limited, err := store.UpdateUserLimits(ctx, domainadminuser.LimitsRequest{UserID: int64(alice.ID), RPMLimit: 100, ConcurrencyLimit: 2})
	if err != nil {
		t.Fatalf("UpdateUserLimits: %v", err)
	}
	if limited.RPMLimit != 100 || limited.ConcurrencyLimit != 2 {
		t.Fatalf("unexpected user limits %#v", limited)
	}

	if _, err := store.CreateUserGroup(ctx, domainadminuser.UserGroupWriteRequest{
		GroupCode:  "vip",
		GroupName:  "VIP",
		Multiplier: "1.50000",
		Status:     "active",
		SortOrder:  20,
		IsDefault:  true,
	}); err != nil {
		t.Fatalf("CreateUserGroup: %v", err)
	}
	vipGroup, err := store.GetUserGroup(ctx, "vip")
	if err != nil {
		t.Fatalf("GetUserGroup vip: %v", err)
	}
	if vipGroup.SortOrder != 20 || !vipGroup.IsDefault {
		t.Fatalf("expected group sort/default to round trip, got %#v", vipGroup)
	}
	reassigned, err := store.AssignUserGroup(ctx, domainadminuser.GroupAssignmentRequest{UserID: int64(alice.ID), UserGroupCode: "vip"})
	if err != nil {
		t.Fatalf("AssignUserGroup: %v", err)
	}
	if reassigned.UserGroupCode != "vip" {
		t.Fatalf("expected vip group, got %#v", reassigned)
	}

	promoGroup, err := store.CreateUserGroup(ctx, domainadminuser.UserGroupWriteRequest{
		GroupCode:  "promo",
		GroupName:  "Promo",
		Multiplier: "0.80000",
		Status:     "active",
		SortOrder:  30,
		IsDefault:  true,
	})
	if err != nil {
		t.Fatalf("CreateUserGroup promo: %v", err)
	}
	vipGroup, err = store.GetUserGroup(ctx, "vip")
	if err != nil {
		t.Fatalf("GetUserGroup vip after promo: %v", err)
	}
	if vipGroup.IsDefault {
		t.Fatalf("expected vip default flag to be cleared after promo default, got %#v", vipGroup)
	}
	if !promoGroup.IsDefault {
		t.Fatalf("expected promo to be default, got %#v", promoGroup)
	}

	if _, err := store.AssignUserGroups(ctx, domainadminuser.MultiGroupAssignmentRequest{
		UserID:   int64(alice.ID),
		GroupIDs: []int64{promoGroup.ID},
	}); err != nil {
		t.Fatalf("AssignUserGroups promo: %v", err)
	}
	if err := store.DeleteUserGroup(ctx, "promo"); err == nil {
		t.Fatal("expected deleting membership-referenced group to fail")
	}
}

func TestAdminUserStoreCreateUserCreatesDefaultProject(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminuser-default-project?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewAdminUserStore(client, entstore.NewBillingStore(client, 5))
	created, err := store.CreateUser(ctx, domainadminuser.CreateUserRequest{
		Email: "created-with-project@example.com", Nickname: "Created", Status: "active", UserGroupCode: "basic", DefaultLocale: "zh-CN", Theme: "system",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	projects, err := client.Project.Query().All(ctx)
	if err != nil {
		t.Fatalf("query projects: %v", err)
	}
	if len(projects) != 1 || projects[0].UserID != created.ID || !projects[0].IsDefault || projects[0].Name != "默认" {
		t.Fatalf("admin-created user default project = %#v", projects)
	}
}

func TestAdminUserStoreCreateUserRollsBackWhenDefaultProjectFails(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminuser-default-project-rollback?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	client.Use(func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if _, ok := mutation.(*repoent.ProjectMutation); ok {
				return nil, errors.New("injected default project failure")
			}
			return next.Mutate(ctx, mutation)
		})
	})
	store := entstore.NewAdminUserStore(client, entstore.NewBillingStore(client, 5))
	if _, err := store.CreateUser(ctx, domainadminuser.CreateUserRequest{Email: "admin-rollback@example.com", Nickname: "rollback", Status: "active", UserGroupCode: "basic", DefaultLocale: "zh-CN", Theme: "system"}); err == nil {
		t.Fatal("CreateUser succeeded despite default project failure")
	}
	if count, countErr := client.User.Query().Count(ctx); countErr != nil || count != 0 {
		t.Fatalf("admin-created user transaction did not roll back: count=%d err=%v", count, countErr)
	}
}

func testBillingConfig() config.BillingConfig {
	return config.BillingConfig{
		PointsScale:          5,
		UserGroupMultipliers: map[string]string{"basic": "1.00000"},
		CNYPerPoint:          "0.00000",
	}
}

func TestBillingStoreAdminAdjustIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminadjust-idem?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	billingStore := entstore.NewBillingStore(client, 5)
	req := billingservice.AdjustStoreRequest{
		UserID:          42,
		ChangePoints:    "9.00000",
		Reason:          "manual grant",
		OperatorAdminID: 7,
		IdempotencyKey:  "adjust-42-once",
	}
	first, err := billingStore.Adjust(ctx, req)
	if err != nil {
		t.Fatalf("Adjust first: %v", err)
	}
	second, err := billingStore.Adjust(ctx, req)
	if err != nil {
		t.Fatalf("Adjust replay: %v", err)
	}
	if first.AvailablePoints != second.AvailablePoints || second.AvailablePoints != "9.00000" {
		t.Fatalf("expected idempotent balance 9.00000, first=%#v second=%#v", first, second)
	}

	page, err := billingStore.ListLedger(ctx, 42, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one ledger entry after replay, got %#v", page)
	}
	conflictReq := req
	conflictReq.ChangePoints = "10.00000"
	if _, err := billingStore.Adjust(ctx, conflictReq); err == nil {
		t.Fatal("expected conflicting idempotency replay to fail")
	}
	page, err = billingStore.ListLedger(ctx, 42, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger after conflict: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected conflict not to duplicate ledger, got %#v", page)
	}
}
