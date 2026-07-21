package entstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/installation"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestSetupStoreBindsFirstAdminAndRetriesWithoutPassword(t *testing.T) {
	client := newSetupStoreSQLiteClient(t)
	installationID := uuid.NewString()
	seedSetupInstallation(t, client, installationID)
	store := entstore.NewSetupStore(client)
	passwordHash, err := adminauthservice.HashPasswordChecked("setup-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	request := setup.SetupInitializationRequest{
		OperationID: uuid.NewString(), InstallationID: installationID,
		ConfigRevision: 1, RequestDigest: strings.Repeat("a", 64),
		AdminEmail: "Root@Example.com", AdminPasswordHash: passwordHash,
	}

	created, err := store.Initialize(t.Context(), request)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if created.OperationID != request.OperationID || created.InstallationID != installationID || created.AdminID <= 0 || created.AdminEmail != "root@example.com" {
		t.Fatalf("created binding=%+v", created)
	}
	entity, err := client.AdminUser.Get(t.Context(), int(created.AdminID))
	if err != nil {
		t.Fatalf("load first admin: %v", err)
	}
	if entity.Role != domainadminauth.RoleSuperAdmin || entity.Status != "active" || entity.PasswordHash != passwordHash || strings.Contains(entity.PasswordHash, "setup-password") {
		t.Fatalf("unsafe first admin=%+v", entity)
	}

	retry := request
	retry.AdminPasswordHash = ""
	retried, err := store.Initialize(t.Context(), retry)
	if err != nil || retried != created {
		t.Fatalf("passwordless retry=(%+v, %v), want %+v", retried, err, created)
	}
	if count, err := client.AdminUser.Query().Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("admin count=%d err=%v", count, err)
	}

	conflictingOperation := retry
	conflictingOperation.OperationID = uuid.NewString()
	if _, err := store.Initialize(t.Context(), conflictingOperation); !errors.Is(err, setup.ErrSetupOperationConflict) {
		t.Fatalf("conflicting operation error=%v", err)
	}
	conflictingDigest := retry
	conflictingDigest.RequestDigest = strings.Repeat("b", 64)
	if _, err := store.Initialize(t.Context(), conflictingDigest); !errors.Is(err, setup.ErrSetupBindingMismatch) {
		t.Fatalf("conflicting digest error=%v", err)
	}
	conflictingEmail := retry
	conflictingEmail.AdminEmail = "other@example.com"
	if _, err := store.Initialize(t.Context(), conflictingEmail); !errors.Is(err, setup.ErrFirstAdminConflict) {
		t.Fatalf("same-operation changed-email error=%v", err)
	}
}

func TestSetupStoreRollsBackWhenFirstAdminAlreadyExists(t *testing.T) {
	client := newSetupStoreSQLiteClient(t)
	installationID := uuid.NewString()
	seedSetupInstallation(t, client, installationID)
	if _, err := client.AdminUser.Create().
		SetEmail("existing@example.com").
		SetPasswordHash(adminauthservice.HashPassword("existing-password")).
		SetRole(domainadminauth.RoleSuperAdmin).
		SetStatus("active").
		Save(t.Context()); err != nil {
		t.Fatalf("seed existing admin: %v", err)
	}
	passwordHash, _ := adminauthservice.HashPasswordChecked("setup-password")
	_, err := entstore.NewSetupStore(client).Initialize(t.Context(), setup.SetupInitializationRequest{
		OperationID: uuid.NewString(), InstallationID: installationID,
		ConfigRevision: 1, RequestDigest: strings.Repeat("c", 64),
		AdminEmail: "root@example.com", AdminPasswordHash: passwordHash,
	})
	if !errors.Is(err, setup.ErrFirstAdminConflict) {
		t.Fatalf("Initialize existing admin error=%v", err)
	}
	entity, err := client.Installation.Query().Only(t.Context())
	if err != nil {
		t.Fatalf("load installation: %v", err)
	}
	if entity.SetupOperationID != nil || entity.SetupAdminID != nil || entity.SetupRequestDigest != nil {
		t.Fatalf("failed initialization persisted binding: %+v", entity)
	}
}

func TestSetupStoreRejectsForgedBcryptPrefix(t *testing.T) {
	client := newSetupStoreSQLiteClient(t)
	installationID := uuid.NewString()
	seedSetupInstallation(t, client, installationID)
	_, err := entstore.NewSetupStore(client).Initialize(t.Context(), setup.SetupInitializationRequest{
		OperationID: uuid.NewString(), InstallationID: installationID,
		ConfigRevision: 1, RequestDigest: strings.Repeat("e", 64),
		AdminEmail: "root@example.com", AdminPasswordHash: "bcrypt$garbage",
	})
	if !errors.Is(err, setup.ErrFirstAdminConflict) {
		t.Fatalf("forged bcrypt error=%v", err)
	}
	if count, countErr := client.AdminUser.Query().Count(t.Context()); countErr != nil || count != 0 {
		t.Fatalf("forged bcrypt created admins=%d err=%v", count, countErr)
	}
}

func TestSetupStoreConcurrentPostgresInitializationIsIdempotent(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if databaseURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run isolated setup-store PostgreSQL integration")
	}
	installationID := uuid.NewString()
	if _, err := db.Migrate(t.Context(), databaseURL, db.MigrationRequest{
		InstallationID: installationID, AppVersion: "setup-store-integration", ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("migrate isolated PostgreSQL: %v", err)
	}
	client, err := db.Open(databaseURL)
	if err != nil {
		t.Fatalf("open isolated PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = client.AdminUser.Delete().Where(adminuser.EmailEQ("root-setup-store@example.com")).Exec(cleanupCtx)
		_, _ = client.Installation.Delete().Where(installation.InstallationIDEQ(installationID)).Exec(cleanupCtx)
	})
	passwordHash, _ := adminauthservice.HashPasswordChecked("setup-password")
	request := setup.SetupInitializationRequest{
		OperationID: uuid.NewString(), InstallationID: installationID,
		ConfigRevision: 1, RequestDigest: strings.Repeat("d", 64),
		AdminEmail: "root-setup-store@example.com", AdminPasswordHash: passwordHash,
	}
	store := entstore.NewSetupStore(client)
	const callers = 12
	results := make(chan setup.SetupBinding, callers)
	errorsChannel := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			binding, err := store.Initialize(context.Background(), request)
			results <- binding
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Errorf("concurrent Initialize error=%v", err)
		}
	}
	var first setup.SetupBinding
	for binding := range results {
		if first == (setup.SetupBinding{}) {
			first = binding
		} else if binding != first {
			t.Errorf("concurrent binding=%+v want %+v", binding, first)
		}
	}
	if count, err := client.AdminUser.Query().Where(adminuser.EmailEQ(request.AdminEmail)).Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("same-operation administrator count=%d err=%v", count, err)
	}
	cleanupPostgresSetupScenario(t, client, installationID, request.AdminEmail)

	installationID = uuid.NewString()
	if _, err := db.Migrate(t.Context(), databaseURL, db.MigrationRequest{
		InstallationID: installationID, AppVersion: "setup-store-integration", ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("migrate competing-operation scenario: %v", err)
	}
	requests := []setup.SetupInitializationRequest{
		{
			OperationID: uuid.NewString(), InstallationID: installationID,
			ConfigRevision: 1, RequestDigest: strings.Repeat("e", 64),
			AdminEmail: "root-setup-store-a@example.com", AdminPasswordHash: passwordHash,
		},
		{
			OperationID: uuid.NewString(), InstallationID: installationID,
			ConfigRevision: 1, RequestDigest: strings.Repeat("f", 64),
			AdminEmail: "root-setup-store-b@example.com", AdminPasswordHash: passwordHash,
		},
	}
	type competingResult struct {
		request setup.SetupInitializationRequest
		binding setup.SetupBinding
		err     error
	}
	competingResults := make(chan competingResult, callers)
	wait = sync.WaitGroup{}
	for index := range callers {
		request := requests[index%len(requests)]
		wait.Add(1)
		go func() {
			defer wait.Done()
			binding, err := store.Initialize(context.Background(), request)
			competingResults <- competingResult{request: request, binding: binding, err: err}
		}()
	}
	wait.Wait()
	close(competingResults)
	winnerOperation := ""
	for result := range competingResults {
		if result.err == nil {
			if winnerOperation == "" {
				winnerOperation = result.binding.OperationID
			}
			if result.binding.OperationID != winnerOperation || result.request.OperationID != winnerOperation {
				t.Errorf("competing success=%+v request=%+v winner=%s", result.binding, result.request, winnerOperation)
			}
			continue
		}
		if !errors.Is(result.err, setup.ErrSetupOperationConflict) {
			t.Errorf("competing operation error=%v", result.err)
		}
	}
	if winnerOperation == "" {
		t.Fatal("competing operations produced no winner")
	}
	if count, err := client.AdminUser.Query().Where(adminuser.EmailIn(requests[0].AdminEmail, requests[1].AdminEmail)).Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("competing-operation administrator count=%d err=%v", count, err)
	}
	winner, err := store.GetBinding(t.Context(), installationID)
	if err != nil || winner.OperationID != winnerOperation {
		t.Fatalf("competing-operation binding=(%+v, %v), winner=%s", winner, err, winnerOperation)
	}
	cleanupPostgresSetupScenario(t, client, installationID, requests[0].AdminEmail, requests[1].AdminEmail)

	installationID = uuid.NewString()
	if _, err := db.Migrate(t.Context(), databaseURL, db.MigrationRequest{
		InstallationID: installationID, AppVersion: "setup-store-integration", ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("migrate rollback scenario: %v", err)
	}
	existingEmail := "existing-setup-store@example.com"
	if _, err := client.AdminUser.Create().
		SetEmail(existingEmail).
		SetPasswordHash(passwordHash).
		SetRole(domainadminauth.RoleSuperAdmin).
		SetStatus("active").
		Save(t.Context()); err != nil {
		t.Fatalf("seed rollback administrator: %v", err)
	}
	_, err = store.Initialize(t.Context(), setup.SetupInitializationRequest{
		OperationID: uuid.NewString(), InstallationID: installationID,
		ConfigRevision: 1, RequestDigest: strings.Repeat("9", 64),
		AdminEmail: "rejected-setup-store@example.com", AdminPasswordHash: passwordHash,
	})
	if !errors.Is(err, setup.ErrFirstAdminConflict) {
		t.Fatalf("rollback scenario error=%v", err)
	}
	entity, err := client.Installation.Query().Where(installation.InstallationIDEQ(installationID)).Only(t.Context())
	if err != nil {
		t.Fatalf("load rollback installation: %v", err)
	}
	if entity.SetupOperationID != nil || entity.SetupAdminID != nil || entity.SetupConfigRevision != nil || entity.SetupRequestDigest != nil {
		t.Fatalf("rollback scenario persisted setup binding: %+v", entity)
	}
	if count, err := client.AdminUser.Query().Where(adminuser.EmailIn(existingEmail, "rejected-setup-store@example.com")).Count(t.Context()); err != nil || count != 1 {
		t.Fatalf("rollback administrator count=%d err=%v", count, err)
	}
	cleanupPostgresSetupScenario(t, client, installationID, existingEmail, "rejected-setup-store@example.com")
}

func cleanupPostgresSetupScenario(t *testing.T, client *repoent.Client, installationID string, emails ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if len(emails) > 0 {
		if _, err := client.AdminUser.Delete().Where(adminuser.EmailIn(emails...)).Exec(ctx); err != nil {
			t.Fatalf("clean setup scenario administrators: %v", err)
		}
	}
	if _, err := client.Installation.Delete().Where(installation.InstallationIDEQ(installationID)).Exec(ctx); err != nil {
		t.Fatalf("clean setup scenario installation: %v", err)
	}
}

func newSetupStoreSQLiteClient(t *testing.T) *repoent.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:setup-store-%s?mode=memory&cache=shared&_fk=1&_busy_timeout=5000", uuid.NewString())
	client, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create SQLite schema: %v", err)
	}
	return client
}

func seedSetupInstallation(t *testing.T, client *repoent.Client, installationID string) {
	t.Helper()
	if _, err := client.Installation.Create().
		SetSingletonKey("installation").
		SetInstallationID(installationID).
		SetConfigSchemaVersion(1).
		SetDatabaseSchemaVersion(1).
		SetAppVersion("setup-store-test").
		SetInitializedAt(time.Now().UTC()).
		SetMigratedAt(time.Now().UTC()).
		Save(t.Context()); err != nil {
		t.Fatalf("seed installation: %v", err)
	}
}
