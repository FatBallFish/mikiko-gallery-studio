package entstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"entgo.io/ent/dialect"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
)

func TestBindLocalInstallationSelectsExistingActiveSuperAdmin(t *testing.T) {
	client := newLocalBootstrapClient(t)
	preferredHash := adminauthservice.HashPasswordForTest("preferred-password", "preferred-salt")
	alternateHash := adminauthservice.HashPasswordForTest("alternate-password", "alternate-salt")
	if _, err := client.AdminUser.Create().SetEmail("z-root@example.com").SetPasswordHash(alternateHash).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AdminUser.Create().SetEmail("admin@example.com").SetPasswordHash(preferredHash).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	binding, err := entstore.BindLocalInstallation(t.Context(), client, localBindingRequest(t))
	if err != nil {
		t.Fatalf("BindLocalInstallation returned error: %v", err)
	}
	if binding.AdminEmail != "admin@example.com" {
		t.Fatalf("selected administrator = %q, want preferred local administrator", binding.AdminEmail)
	}
	admin, err := client.AdminUser.Query().Where(adminuser.EmailEQ("admin@example.com")).Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if admin.PasswordHash != preferredHash {
		t.Fatal("existing administrator password was overwritten")
	}
}

func TestBindLocalInstallationFallsBackDeterministically(t *testing.T) {
	client := newLocalBootstrapClient(t)
	for _, email := range []string{"z-root@example.com", "a-root@example.com"} {
		if _, err := client.AdminUser.Create().SetEmail(email).SetPasswordHash(adminauthservice.HashPasswordForTest("password", email)).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	request := localBindingRequest(t)
	first, err := entstore.BindLocalInstallation(t.Context(), client, request)
	if err != nil {
		t.Fatalf("first BindLocalInstallation returned error: %v", err)
	}
	second, err := entstore.BindLocalInstallation(t.Context(), client, request)
	if err != nil {
		t.Fatalf("repeated BindLocalInstallation returned error: %v", err)
	}
	if first.AdminEmail != "a-root@example.com" || second != first {
		t.Fatalf("fallback binding is not deterministic/idempotent: first=%+v second=%+v", first, second)
	}
}

func TestBindLocalInstallationPreservesCompletedBindingAdministrator(t *testing.T) {
	client := newLocalBootstrapClient(t)
	original, err := client.AdminUser.Create().SetEmail("z-root@example.com").SetPasswordHash(adminauthservice.HashPasswordForTest("password", "original")).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	request := localBindingRequest(t)
	first, err := entstore.BindLocalInstallation(t.Context(), client, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.AdminID != int64(original.ID) {
		t.Fatalf("initial binding administrator = %d, want %d", first.AdminID, original.ID)
	}
	if _, err := client.AdminUser.Create().SetEmail("admin@example.com").SetPasswordHash(adminauthservice.HashPasswordForTest("password", "preferred")).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := entstore.BindLocalInstallation(t.Context(), client, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.AdminID != first.AdminID || second.AdminEmail != first.AdminEmail || second.RequestDigest != first.RequestDigest {
		t.Fatalf("completed binding changed administrator: first=%+v second=%+v", first, second)
	}
}

func TestBindLocalInstallationCreatesAdminOnlyForEmptyDatabase(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		client := newLocalBootstrapClient(t)
		binding, err := entstore.BindLocalInstallation(t.Context(), client, localBindingRequest(t))
		if err != nil {
			t.Fatalf("BindLocalInstallation returned error: %v", err)
		}
		if binding.AdminEmail != "admin@example.com" {
			t.Fatalf("created administrator = %q", binding.AdminEmail)
		}
		if count, err := client.AdminUser.Query().Count(t.Context()); err != nil || count != 1 {
			t.Fatalf("administrator count = %d, error = %v", count, err)
		}
	})

	t.Run("existing without active super admin", func(t *testing.T) {
		client := newLocalBootstrapClient(t)
		if _, err := client.AdminUser.Create().SetEmail("operator@example.com").SetPasswordHash(adminauthservice.HashPasswordForTest("password", "operator")).SetRole(domainadminauth.RoleAdmin).SetStatus("active").Save(t.Context()); err != nil {
			t.Fatal(err)
		}
		_, err := entstore.BindLocalInstallation(t.Context(), client, localBindingRequest(t))
		if !errors.Is(err, entstore.ErrLocalBootstrapAdminUnavailable) {
			t.Fatalf("BindLocalInstallation error = %v, want ErrLocalBootstrapAdminUnavailable", err)
		}
		if count, countErr := client.AdminUser.Query().Count(t.Context()); countErr != nil || count != 1 {
			t.Fatalf("administrator count changed after rejection: count=%d error=%v", count, countErr)
		}
	})
}

func TestBindLocalInstallationRejectsForeignOrPartialBinding(t *testing.T) {
	for _, tt := range []struct {
		name    string
		partial bool
	}{
		{name: "foreign complete binding"},
		{name: "partial binding", partial: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newLocalBootstrapClient(t)
			admin, err := client.AdminUser.Create().SetEmail("root@example.com").SetPasswordHash(adminauthservice.HashPasswordForTest("password", "root")).SetRole(domainadminauth.RoleSuperAdmin).SetStatus("active").Save(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			installationEntity, err := client.Installation.Query().Only(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			update := client.Installation.UpdateOneID(installationEntity.ID).SetSetupOperationID("production-setup")
			if !tt.partial {
				update.SetSetupAdminID(int64(admin.ID)).SetSetupConfigRevision(1).SetSetupRequestDigest(strings.Repeat("a", 64))
			}
			if _, err := update.Save(t.Context()); err != nil {
				t.Fatal(err)
			}
			_, err = entstore.BindLocalInstallation(t.Context(), client, localBindingRequest(t))
			if !errors.Is(err, entstore.ErrLocalBootstrapBindingConflict) {
				t.Fatalf("BindLocalInstallation error = %v, want ErrLocalBootstrapBindingConflict", err)
			}
		})
	}
}

func TestBindLocalInstallationValidatesRequestAndDatabaseIdentity(t *testing.T) {
	client := newLocalBootstrapClient(t)
	request := localBindingRequest(t)
	request.FreshAdminPasswordHash = ""
	if _, err := entstore.BindLocalInstallation(t.Context(), client, request); err == nil {
		t.Fatal("BindLocalInstallation accepted an incomplete request")
	}

	mismatch, err := ent.Open(dialect.SQLite, "file:"+strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())+"-mismatch?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mismatch.Close() })
	if err := mismatch.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := mismatch.Installation.Create().SetSingletonKey("installation").SetInstallationID("other-installation").SetConfigSchemaVersion(1).SetDatabaseSchemaVersion(1).SetAppVersion("dev").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := entstore.BindLocalInstallation(t.Context(), mismatch, localBindingRequest(t)); err == nil {
		t.Fatal("BindLocalInstallation accepted a different database installation identity")
	}
}

func TestOpenAndBindLocalInstallationHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := entstore.OpenAndBindLocalInstallation(ctx, "postgres://postgres@postgres:5432/pic_gallery?sslmode=disable", localBindingRequest(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenAndBindLocalInstallation error = %v, want context.Canceled", err)
	}
}

func newLocalBootstrapClient(t *testing.T) *ent.Client {
	t.Helper()
	client, err := ent.Open(dialect.SQLite, "file:"+strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Installation.Create().SetSingletonKey("installation").SetInstallationID("pic-gallery-local").SetConfigSchemaVersion(1).SetDatabaseSchemaVersion(1).SetAppVersion("dev").Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	return client
}

func localBindingRequest(t *testing.T) entstore.LocalBindingRequest {
	t.Helper()
	return entstore.LocalBindingRequest{
		OperationID: "local-bootstrap", InstallationID: "pic-gallery-local", ConfigRevision: 1,
		RuntimeValues:          map[string]string{"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "local-dev-secure-config-encryption-key"},
		PreferredAdminEmail:    "admin@example.com",
		FreshAdminPasswordHash: adminauthservice.HashPasswordForTest("admin123456", "local-bootstrap"),
	}
}
