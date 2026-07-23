package entstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/adminuser"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/installation"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

var (
	ErrLocalBootstrapAdminUnavailable = errors.New("local bootstrap requires an active super administrator")
	ErrLocalBootstrapBindingConflict  = errors.New("local bootstrap database binding conflicts with the local installation")
)

type LocalBindingRequest struct {
	OperationID            string
	InstallationID         string
	ConfigRevision         int
	RuntimeValues          map[string]string
	PreferredAdminEmail    string
	FreshAdminPasswordHash string
}

func OpenAndBindLocalInstallation(ctx context.Context, databaseURL string, request LocalBindingRequest) (setup.SetupBinding, error) {
	client, err := db.OpenContext(ctx, databaseURL)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("open local bootstrap database: %w", err)
	}
	defer client.Close()
	return BindLocalInstallation(ctx, client, request)
}

func BindLocalInstallation(ctx context.Context, client *repoent.Client, request LocalBindingRequest) (binding setup.SetupBinding, returnErr error) {
	if client == nil {
		return setup.SetupBinding{}, fmt.Errorf("local bootstrap database client is required")
	}
	if request.OperationID != "local-bootstrap" || request.InstallationID != "pic-gallery-local" || request.ConfigRevision <= 0 || len(request.RuntimeValues) == 0 {
		return setup.SetupBinding{}, fmt.Errorf("local bootstrap binding identity is invalid")
	}
	preferred := strings.ToLower(strings.TrimSpace(request.PreferredAdminEmail))
	if preferred == "" || strings.TrimSpace(request.FreshAdminPasswordHash) == "" {
		return setup.SetupBinding{}, fmt.Errorf("local bootstrap administrator defaults are required")
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("begin local bootstrap transaction: %w", err)
	}
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, tx.Rollback())
		}
	}()
	txClient := tx.Client()
	installations, err := txClient.Installation.Query().Limit(2).All(ctx)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("load local installation: %w", err)
	}
	if len(installations) != 1 || installations[0].SingletonKey != "installation" || installations[0].InstallationID != request.InstallationID {
		return setup.SetupBinding{}, fmt.Errorf("local bootstrap installation identity does not match database")
	}
	entity := installations[0]
	present := 0
	for _, value := range []bool{entity.SetupOperationID != nil, entity.SetupAdminID != nil, entity.SetupConfigRevision != nil, entity.SetupRequestDigest != nil} {
		if value {
			present++
		}
	}
	if present != 0 && (present != 4 || entity.SetupOperationID == nil || *entity.SetupOperationID != request.OperationID) {
		return setup.SetupBinding{}, ErrLocalBootstrapBindingConflict
	}

	var selected *repoent.AdminUser
	if present == 4 {
		if entity.SetupConfigRevision == nil || *entity.SetupConfigRevision != request.ConfigRevision || entity.SetupAdminID == nil || int64(int(*entity.SetupAdminID)) != *entity.SetupAdminID {
			return setup.SetupBinding{}, ErrLocalBootstrapBindingConflict
		}
		selected, err = txClient.AdminUser.Query().Where(adminuser.IDEQ(int(*entity.SetupAdminID))).Only(ctx)
		if err != nil || selected.Role != domainadminauth.RoleSuperAdmin || selected.Status != "active" {
			return setup.SetupBinding{}, ErrLocalBootstrapBindingConflict
		}
	}
	if selected == nil {
		admins, err := txClient.AdminUser.Query().
			Where(adminuser.RoleEQ(domainadminauth.RoleSuperAdmin), adminuser.StatusEQ("active")).
			Order(adminuser.ByEmail(), adminuser.ByID()).All(ctx)
		if err != nil {
			return setup.SetupBinding{}, fmt.Errorf("query active super administrators: %w", err)
		}
		for _, candidate := range admins {
			if strings.EqualFold(candidate.Email, preferred) {
				selected = candidate
				break
			}
		}
		if selected == nil && len(admins) > 0 {
			selected = admins[0]
		}
	}
	if selected == nil {
		count, err := txClient.AdminUser.Query().Count(ctx)
		if err != nil {
			return setup.SetupBinding{}, fmt.Errorf("count local administrators: %w", err)
		}
		if count != 0 {
			return setup.SetupBinding{}, ErrLocalBootstrapAdminUnavailable
		}
		selected, err = txClient.AdminUser.Create().
			SetEmail(preferred).
			SetPasswordHash(request.FreshAdminPasswordHash).
			SetRole(domainadminauth.RoleSuperAdmin).
			SetStatus("active").
			Save(ctx)
		if err != nil {
			return setup.SetupBinding{}, fmt.Errorf("create local administrator: %w", err)
		}
	}

	digest, err := setup.CanonicalRequestDigest(request.RuntimeValues, selected.Email)
	if err != nil {
		return setup.SetupBinding{}, fmt.Errorf("calculate local setup digest: %w", err)
	}
	if _, err := txClient.Installation.UpdateOneID(entity.ID).
		SetSetupOperationID(request.OperationID).
		SetSetupAdminID(int64(selected.ID)).
		SetSetupConfigRevision(request.ConfigRevision).
		SetSetupRequestDigest(digest).
		Where(installation.InstallationIDEQ(request.InstallationID)).
		Save(ctx); err != nil {
		return setup.SetupBinding{}, fmt.Errorf("bind local installation: %w", err)
	}
	binding = setup.SetupBinding{
		OperationID: request.OperationID, InstallationID: request.InstallationID,
		ConfigRevision: request.ConfigRevision, RequestDigest: digest,
		AdminID: int64(selected.ID), AdminEmail: strings.ToLower(selected.Email),
	}
	if err := tx.Commit(); err != nil {
		return setup.SetupBinding{}, fmt.Errorf("commit local bootstrap transaction: %w", err)
	}
	return binding, nil
}
