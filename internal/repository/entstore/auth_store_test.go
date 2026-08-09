package entstore

import (
	"context"
	"errors"
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
	projects, err := client.Project.Query().All(ctx)
	if err != nil {
		t.Fatalf("query default project: %v", err)
	}
	if len(projects) != 1 || projects[0].UserID != loaded.ID || !projects[0].IsDefault || projects[0].Name != "默认" {
		t.Fatalf("user creation must atomically create one default project, got %#v", projects)
	}

	err = store.SaveRefreshSession(ctx, RefreshSessionRecord{
		ID:               "11111111-1111-1111-1111-111111111111",
		FamilyID:         "22222222-2222-2222-2222-222222222222",
		UserID:           loaded.ID,
		TokenVersion:     3,
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
	if session.UserID != loaded.ID || session.TokenVersion != 3 || session.Status != "active" {
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

func TestAuthStoreCreateUserRollsBackWhenDefaultProjectFails(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:authstore-default-project-rollback?mode=memory&cache=shared&_fk=1")
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

	store := NewAuthStore(client)
	if _, err := store.CreateUser(ctx, auth.User{Email: "rollback@example.com", Status: "active", GroupCode: "basic", GroupMultiplier: "1.00000"}); err == nil {
		t.Fatal("CreateUser succeeded despite default project failure")
	}
	if count, countErr := client.User.Query().Count(ctx); countErr != nil || count != 0 {
		t.Fatalf("user transaction did not roll back: count=%d err=%v", count, countErr)
	}
}

func TestAuthStorePasswordUpdateAndSessionRevocationAreAtomic(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:authstore-password-atomic?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAuthStore(client)
	created, err := store.CreateUser(ctx, auth.User{Email: "atomic@example.com", Nickname: "atomic", Status: "active", GroupCode: "basic", GroupMultiplier: "1.00000"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := store.SaveRefreshSession(ctx, RefreshSessionRecord{
		ID: "11111111-1111-1111-1111-111111111111", FamilyID: "22222222-2222-2222-2222-222222222222",
		UserID: created.ID, RefreshTokenHash: "atomic-hash", Status: "active", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("SaveRefreshSession: %v", err)
	}

	client.Use(func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if _, ok := mutation.(*repoent.RefreshSessionMutation); ok {
				return nil, errors.New("injected refresh-session revocation failure")
			}
			return next.Mutate(ctx, mutation)
		})
	})

	if _, err := store.UpdatePasswordAndRevokeSessions(ctx, created.ID, "new-password-hash", time.Now()); err == nil {
		t.Fatal("expected password update transaction to fail when refresh-session revocation fails")
	}
	loaded, err := store.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if loaded.PasswordHash != "" || loaded.TokenVersion != created.TokenVersion {
		t.Fatalf("password mutation must roll back with session revocation: before=%#v after=%#v", created, loaded)
	}
	session, err := store.GetRefreshSessionByHash(ctx, "atomic-hash")
	if err != nil {
		t.Fatalf("GetRefreshSessionByHash: %v", err)
	}
	if session.Status != "active" {
		t.Fatalf("refresh-session mutation must roll back with password update, got %#v", session)
	}
}
