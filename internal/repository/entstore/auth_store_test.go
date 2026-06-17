package entstore

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/domain/auth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	_ "github.com/mattn/go-sqlite3"
)

func TestAuthStorePersistsUserAndRefreshSession(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:authstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAuthStore(client)
	user, err := store.CreateUser(ctx, auth.User{
		Email:           "repo@example.com",
		Nickname:        "repo-user",
		Status:          "active",
		GroupCode:       "basic",
		GroupMultiplier: "1.00000",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	loaded, err := store.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if loaded.ID == 0 || loaded.Email != user.Email {
		t.Fatalf("unexpected loaded user: %#v", loaded)
	}

	err = store.SaveRefreshSession(ctx, RefreshSessionRecord{
		ID:               "11111111-1111-1111-1111-111111111111",
		FamilyID:         "22222222-2222-2222-2222-222222222222",
		UserID:           loaded.ID,
		RefreshTokenHash: "hash-1",
		Status:           "active",
		ExpiresAt:        time.Now().Add(2 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("SaveRefreshSession: %v", err)
	}

	session, err := store.GetRefreshSessionByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetRefreshSessionByHash: %v", err)
	}
	if session.UserID != loaded.ID || session.Status != "active" {
		t.Fatalf("unexpected session: %#v", session)
	}

	if err := store.MarkRefreshSessionRotated(ctx, "11111111-1111-1111-1111-111111111111", "33333333-3333-3333-3333-333333333333"); err != nil {
		t.Fatalf("MarkRefreshSessionRotated: %v", err)
	}
	if err := store.MarkFamilyReplayBlocked(ctx, "22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatalf("MarkFamilyReplayBlocked: %v", err)
	}
	blocked, err := store.GetRefreshSessionByHash(ctx, "hash-1")
	if err != nil {
		t.Fatalf("GetRefreshSessionByHash after replay: %v", err)
	}
	if blocked.Status != "replay_blocked" {
		t.Fatalf("expected replay_blocked, got %s", blocked.Status)
	}
}

func TestAuthStoreUsesLowestEnabledMembershipMultiplier(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:authstore-groups?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAuthStore(client)
	user, err := store.CreateUser(ctx, auth.User{
		Email:           "special@example.com",
		Nickname:        "special-user",
		Status:          "active",
		GroupCode:       "basic",
		GroupMultiplier: "1.00000",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	special, err := client.UserGroup.Create().
		SetGroupCode("special").
		SetGroupName("Special").
		SetMultiplier("0.50000").
		SetStatus("enabled").
		Save(ctx)
	if err != nil {
		t.Fatalf("create special group: %v", err)
	}
	disabled, err := client.UserGroup.Create().
		SetGroupCode("disabled-special").
		SetGroupName("Disabled Special").
		SetMultiplier("0.10000").
		SetStatus("disabled").
		Save(ctx)
	if err != nil {
		t.Fatalf("create disabled group: %v", err)
	}
	if _, err := client.UserGroupMember.Create().SetUserID(user.ID).SetGroupID(int64(special.ID)).Save(ctx); err != nil {
		t.Fatalf("create special membership: %v", err)
	}
	if _, err := client.UserGroupMember.Create().SetUserID(user.ID).SetGroupID(int64(disabled.ID)).Save(ctx); err != nil {
		t.Fatalf("create disabled membership: %v", err)
	}

	loaded, err := store.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if loaded.GroupCode != "basic" {
		t.Fatalf("expected primary group to remain basic, got %#v", loaded)
	}
	if loaded.GroupMultiplier != "0.50000" {
		t.Fatalf("expected lowest enabled group multiplier, got %#v", loaded)
	}
	if len(loaded.GroupCodes) != 3 {
		t.Fatalf("expected all group codes to remain available for visibility, got %#v", loaded.GroupCodes)
	}
}
