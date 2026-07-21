package entstore

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/installation"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

const setupStoreAttempts = 8

var setupDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type SetupStore struct {
	client     *repoent.Client
	ownsClient bool
}

func NewSetupStore(client *repoent.Client) *SetupStore {
	return &SetupStore{client: client}
}

func OpenSetupStore(ctx context.Context, databaseURL string) (setup.SetupStoreSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := db.OpenContext(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open setup database: %w", err)
	}
	return &SetupStore{client: client, ownsClient: true}, nil
}

func (store *SetupStore) Close() error {
	if store == nil || store.client == nil || !store.ownsClient {
		return nil
	}
	store.ownsClient = false
	return store.client.Close()
}

func (store *SetupStore) Initialize(ctx context.Context, request setup.SetupInitializationRequest) (setup.SetupBinding, error) {
	if err := validateSetupInitializationIdentity(request); err != nil {
		return setup.SetupBinding{}, err
	}
	if store == nil || store.client == nil {
		return setup.SetupBinding{}, fmt.Errorf("setup store is not configured")
	}

	var lastErr error
	for attempt := 0; attempt < setupStoreAttempts; attempt++ {
		binding, err := store.initializeOnce(ctx, request)
		if err == nil {
			return binding, nil
		}
		lastErr = err
		if existing, lookupErr := store.GetBinding(ctx, request.InstallationID); lookupErr == nil {
			return matchSetupBinding(existing, request)
		} else if !errors.Is(lookupErr, setup.ErrSetupBindingNotFound) {
			lastErr = errors.Join(lastErr, lookupErr)
		}
		if isDeterministicSetupStoreError(err) {
			return setup.SetupBinding{}, err
		}
		select {
		case <-ctx.Done():
			return setup.SetupBinding{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	return setup.SetupBinding{}, fmt.Errorf("initialize setup binding: %w", lastErr)
}

func (store *SetupStore) initializeOnce(ctx context.Context, request setup.SetupInitializationRequest) (binding setup.SetupBinding, returnErr error) {
	tx, err := store.client.Tx(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("begin setup transaction: %w", err)
	}
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, tx.Rollback())
		}
	}()
	txClient := tx.Client()
	installations, err := txClient.Installation.Query().Limit(2).All(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("load setup installation: %w", err)
	}
	if len(installations) == 0 {
		return setup.SetupBinding{}, setup.ErrSetupBindingNotFound
	}
	if len(installations) != 1 || installations[0].SingletonKey != "installation" {
		return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
	}
	entity := installations[0]
	if entity.InstallationID != request.InstallationID {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	if entity.SetupOperationID != nil || entity.SetupAdminID != nil || entity.SetupConfigRevision != nil || entity.SetupRequestDigest != nil {
		existing, err := setupBindingFromClient(ctx, txClient, entity)
		if err != nil {
			return setup.SetupBinding{}, err
		}
		matched, err := matchSetupBinding(existing, request)
		if err != nil {
			return setup.SetupBinding{}, err
		}
		if err := tx.Commit(); err != nil {
			return setup.SetupBinding{}, fmt.Errorf("commit resumed setup transaction: %w", err)
		}
		return matched, nil
	}

	email, err := validateFirstAdmin(request.AdminEmail, request.AdminPasswordHash)
	if err != nil {
		return setup.SetupBinding{}, err
	}
	count, err := txClient.AdminUser.Query().Count(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("count existing administrators: %w", err)
	}
	if count != 0 {
		return setup.SetupBinding{}, setup.ErrFirstAdminConflict
	}
	admin, err := txClient.AdminUser.Create().
		SetEmail(email).
		SetPasswordHash(request.AdminPasswordHash).
		SetRole(domainadminauth.RoleSuperAdmin).
		SetStatus("active").
		Save(ctx)
	if err != nil {
		if repoent.IsConstraintError(err) {
			return setup.SetupBinding{}, setup.ErrFirstAdminConflict
		}
		return setup.SetupBinding{}, fmt.Errorf("create first administrator: %w", err)
	}
	updated, err := txClient.Installation.Update().
		Where(installation.IDEQ(entity.ID), installation.SetupOperationIDIsNil()).
		SetSetupOperationID(request.OperationID).
		SetSetupAdminID(int64(admin.ID)).
		SetSetupConfigRevision(request.ConfigRevision).
		SetSetupRequestDigest(request.RequestDigest).
		Save(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("bind setup operation: %w", err)
	}
	if updated != 1 {
		return setup.SetupBinding{}, setup.ErrSetupOperationConflict
	}
	binding = setup.SetupBinding{
		OperationID: request.OperationID, InstallationID: request.InstallationID,
		ConfigRevision: request.ConfigRevision, RequestDigest: request.RequestDigest,
		AdminID: int64(admin.ID), AdminEmail: email,
	}
	if err := tx.Commit(); err != nil {
		return setup.SetupBinding{}, fmt.Errorf("commit setup transaction: %w", err)
	}
	return binding, nil
}

func (store *SetupStore) GetBinding(ctx context.Context, installationID string) (setup.SetupBinding, error) {
	if store == nil || store.client == nil {
		return setup.SetupBinding{}, fmt.Errorf("setup store is not configured")
	}
	entities, err := store.client.Installation.Query().Limit(2).All(ctx)
	if err != nil {
		if setupSchemaMissing(err) {
			return setup.SetupBinding{}, setup.ErrSetupBindingNotFound
		}
		return setup.SetupBinding{}, fmt.Errorf("load setup binding installation: %w", err)
	}
	if len(entities) == 0 {
		return setup.SetupBinding{}, setup.ErrSetupBindingNotFound
	}
	if len(entities) != 1 || entities[0].SingletonKey != "installation" {
		return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
	}
	if entities[0].InstallationID != installationID {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	return setupBindingFromClient(ctx, store.client, entities[0])
}

func (store *SetupStore) MigrationCompleted(ctx context.Context, expected db.SchemaVersion) (bool, error) {
	if store == nil || store.client == nil {
		return false, fmt.Errorf("setup store is not configured")
	}
	if err := db.CheckSchemaCompatibility(ctx, store.client, expected); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		var compatibilityError *db.CompatibilityError
		if errors.As(err, &compatibilityError) && compatibilityError.Kind == db.CompatibilityMissing {
			return false, nil
		}
		return false, setup.ErrSetupBindingMismatch
	}
	return true, nil
}

func setupSchemaMissing(err error) bool {
	var postgresError *pq.Error
	if errors.As(err, &postgresError) && postgresError.Code == "42P01" {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table: installations")
}

func setupBindingFromClient(ctx context.Context, client *repoent.Client, entity *repoent.Installation) (setup.SetupBinding, error) {
	missing := 0
	for _, present := range []bool{
		entity.SetupOperationID != nil, entity.SetupAdminID != nil,
		entity.SetupConfigRevision != nil, entity.SetupRequestDigest != nil,
	} {
		if !present {
			missing++
		}
	}
	if missing == 4 {
		return setup.SetupBinding{}, setup.ErrSetupBindingNotFound
	}
	if missing != 0 {
		return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
	}
	if int64(int(*entity.SetupAdminID)) != *entity.SetupAdminID {
		return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
	}
	admin, err := client.AdminUser.Query().Where(adminuser.IDEQ(int(*entity.SetupAdminID))).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
		}
		return setup.SetupBinding{}, fmt.Errorf("load setup administrator: %w", err)
	}
	if admin.Role != domainadminauth.RoleSuperAdmin || admin.Status != "active" {
		return setup.SetupBinding{}, setup.ErrSetupBindingCorrupt
	}
	return setup.SetupBinding{
		OperationID: *entity.SetupOperationID, InstallationID: entity.InstallationID,
		ConfigRevision: *entity.SetupConfigRevision, RequestDigest: *entity.SetupRequestDigest,
		AdminID: *entity.SetupAdminID, AdminEmail: admin.Email,
	}, nil
}

func validateSetupInitializationIdentity(request setup.SetupInitializationRequest) error {
	parsed, err := uuid.Parse(request.OperationID)
	if err != nil || parsed.String() != request.OperationID {
		return fmt.Errorf("setup operation ID must be a canonical UUID")
	}
	if strings.TrimSpace(request.InstallationID) == "" || request.ConfigRevision <= 0 || !setupDigestPattern.MatchString(request.RequestDigest) {
		return fmt.Errorf("setup initialization identity is invalid")
	}
	return nil
}

func validateFirstAdmin(rawEmail, passwordHash string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !adminauthservice.IsCurrentPasswordHash(passwordHash) {
		return "", setup.ErrFirstAdminConflict
	}
	return email, nil
}

func matchSetupBinding(binding setup.SetupBinding, request setup.SetupInitializationRequest) (setup.SetupBinding, error) {
	if binding.OperationID != request.OperationID {
		return setup.SetupBinding{}, setup.ErrSetupOperationConflict
	}
	if binding.InstallationID != request.InstallationID || binding.ConfigRevision != request.ConfigRevision || binding.RequestDigest != request.RequestDigest {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	if email := strings.ToLower(strings.TrimSpace(request.AdminEmail)); email != "" && email != binding.AdminEmail {
		return setup.SetupBinding{}, setup.ErrFirstAdminConflict
	}
	return binding, nil
}

func isDeterministicSetupStoreError(err error) bool {
	return errors.Is(err, setup.ErrSetupOperationConflict) ||
		errors.Is(err, setup.ErrSetupBindingMismatch) ||
		errors.Is(err, setup.ErrSetupBindingCorrupt) ||
		errors.Is(err, setup.ErrFirstAdminConflict)
}
