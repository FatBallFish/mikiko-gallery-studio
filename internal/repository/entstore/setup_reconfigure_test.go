package entstore_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	"github.com/fatballfish/pic-gallery/internal/setup"
	"github.com/google/uuid"
)

func TestReconfigureBindingPostgresCASAndRollback(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if dsn == "" {
		t.Skip("requires isolated PIC_GALLERY_TEST_POSTGRES_URL")
	}
	id := uuid.NewString()
	if _, err := db.Migrate(t.Context(), dsn, db.MigrationRequest{InstallationID: id, AppVersion: "reconfigure-test", ConfigVersion: 1}); err != nil {
		t.Fatal("isolated PostgreSQL migration failed")
	}
	client, err := db.Open(dsn)
	if err != nil {
		t.Fatal("isolated PostgreSQL open failed")
	}
	defer client.Close()
	email := "reconfigure-cas@example.test"
	defer cleanupPostgresSetupScenario(t, client, id, email)
	store := entstore.NewSetupStore(client)
	password, _ := adminauthservice.HashPasswordChecked("setup-password")
	old, err := store.Initialize(t.Context(), setup.SetupInitializationRequest{OperationID: uuid.NewString(), InstallationID: id, ConfigRevision: 1, RequestDigest: strings.Repeat("a", 64), AdminEmail: email, AdminPasswordHash: password})
	if err != nil {
		t.Fatal(err)
	}
	next, err := store.CompareSwapConfiguration(t.Context(), setup.SetupBindingConfigUpdate{Expected: old, ConfigRevision: 2, RequestDigest: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if next.AdminID != old.AdminID || next.AdminEmail != old.AdminEmail || next.OperationID != old.OperationID || next.InstallationID != old.InstallationID {
		t.Fatal("binding identity changed")
	}
	for _, stale := range []setup.SetupBinding{old, func() setup.SetupBinding { x := next; x.RequestDigest = old.RequestDigest; return x }()} {
		if _, err := store.CompareSwapConfiguration(t.Context(), setup.SetupBindingConfigUpdate{Expected: stale, ConfigRevision: stale.ConfigRevision + 1, RequestDigest: strings.Repeat("c", 64)}); err == nil {
			t.Fatal("stale proof accepted")
		}
	}
	restored, err := store.CompareSwapConfiguration(t.Context(), setup.SetupBindingConfigUpdate{Expected: next, ConfigRevision: 1, RequestDigest: old.RequestDigest})
	if err != nil || restored != old {
		t.Fatal("reverse CAS failed")
	}
}
