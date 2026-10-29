package entstore

import (
	"context"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/installation"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

// CompareSwapConfiguration atomically advances (or restores) a verified
// completed binding without changing its setup operation or administrator.
func (store *SetupStore) CompareSwapConfiguration(ctx context.Context, update setup.SetupBindingConfigUpdate) (setup.SetupBinding, error) {
	old := update.Expected
	if store == nil || store.client == nil || old.ConfigRevision <= 0 || (update.ConfigRevision != old.ConfigRevision+1 && update.ConfigRevision != old.ConfigRevision-1) || !setupDigestPattern.MatchString(old.RequestDigest) || !setupDigestPattern.MatchString(update.RequestDigest) {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	binding, err := store.GetBinding(ctx, old.InstallationID)
	if err != nil {
		return setup.SetupBinding{}, err
	}
	if binding != old {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	n, err := store.client.Installation.Update().Where(
		installation.SingletonKeyEQ("installation"), installation.InstallationIDEQ(old.InstallationID),
		installation.SetupOperationIDEQ(old.OperationID), installation.SetupAdminIDEQ(old.AdminID),
		installation.SetupConfigRevisionEQ(old.ConfigRevision), installation.SetupRequestDigestEQ(old.RequestDigest),
	).SetSetupConfigRevision(update.ConfigRevision).SetSetupRequestDigest(update.RequestDigest).Save(ctx)
	if err != nil {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	if n != 1 {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	return store.GetBinding(ctx, old.InstallationID)
}
