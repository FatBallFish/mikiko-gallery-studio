package setup

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

var (
	ErrSetupBindingNotFound   = errors.New("setup binding was not found")
	ErrSetupOperationConflict = errors.New("another setup operation owns this installation")
	ErrSetupBindingMismatch   = errors.New("setup binding does not match the requested commit")
	ErrSetupBindingCorrupt    = errors.New("setup binding is incomplete or corrupt")
	ErrFirstAdminConflict     = errors.New("the first administrator conflicts with existing data")
)

type SetupInitializationRequest struct {
	OperationID       string
	InstallationID    string
	ConfigRevision    int
	RequestDigest     string
	AdminEmail        string
	AdminPasswordHash string
}

type SetupBinding struct {
	OperationID    string
	InstallationID string
	ConfigRevision int
	RequestDigest  string
	AdminID        int64
	AdminEmail     string
}

type SetupBindingDigestUpdate struct {
	OperationID           string
	InstallationID        string
	ConfigRevision        int
	ExpectedRequestDigest string
	RequestDigest         string
}

type SetupBindingDigestReconciler interface {
	ReconcileRequestDigest(context.Context, SetupBindingDigestUpdate) (SetupBinding, error)
}

type CompletedBindingStateStore interface {
	Load() (InstallState, bool, error)
	ReconcileCompletedCommit(CommitProof, time.Time) (InstallState, error)
}

type SetupStore interface {
	Initialize(context.Context, SetupInitializationRequest) (SetupBinding, error)
	GetBinding(context.Context, string) (SetupBinding, error)
	MigrationCompleted(context.Context, db.SchemaVersion) (bool, error)
}

type SetupStoreSession interface {
	SetupStore
	io.Closer
}

type SetupStoreOpener func(context.Context, string) (SetupStoreSession, error)
