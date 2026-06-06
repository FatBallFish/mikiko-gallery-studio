package entstore_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	_ "github.com/mattn/go-sqlite3"
)

func TestAdminAuthStoreCreatesAndLoadsAdminByEmail(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminauthstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAdminAuthStore(client)
	created, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "Admin@Example.COM",
		PasswordHash: "hash",
		Role:         "super_admin",
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	loaded, err := store.GetAdminByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("GetAdminByEmail: %v", err)
	}
	if loaded.ID != created.ID || loaded.Email != "admin@example.com" || loaded.Role != "super_admin" {
		t.Fatalf("unexpected loaded admin %#v created %#v", loaded, created)
	}
}

func TestAdminAuthStoreDefaultsAdminRoleToBuiltInAdmin(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminauthstore-default-role?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAdminAuthStore(client)
	created, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "default-role@example.com",
		PasswordHash: "hash",
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	if created.Role != domainadminauth.RoleAdmin {
		t.Fatalf("default admin role = %q, want %q", created.Role, domainadminauth.RoleAdmin)
	}
}

func TestAdminAuthStoreUpdatesPasswordHashWithCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:adminauthstore-rehash?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewAdminAuthStore(client)
	created, err := store.CreateAdmin(ctx, domainadminauth.AdminUser{
		Email:        "legacy@example.com",
		PasswordHash: "sha256$fixed$old",
		Role:         "ops_admin",
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if err := store.UpdateAdminPasswordHash(ctx, created.ID, "sha256$fixed$old", "bcrypt$new"); err != nil {
		t.Fatalf("UpdateAdminPasswordHash first: %v", err)
	}
	loaded, err := store.GetAdminByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAdminByID: %v", err)
	}
	if loaded.PasswordHash != "bcrypt$new" {
		t.Fatalf("expected password hash update, got %q", loaded.PasswordHash)
	}
	if err := store.UpdateAdminPasswordHash(ctx, created.ID, "sha256$fixed$old", "bcrypt$stale"); err == nil {
		t.Fatalf("expected stale old hash to be rejected")
	}
	loaded, err = store.GetAdminByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAdminByID after stale update: %v", err)
	}
	if loaded.PasswordHash != "bcrypt$new" {
		t.Fatalf("stale CAS update changed password hash to %q", loaded.PasswordHash)
	}
}
