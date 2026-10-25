package mediaasset

import (
	"context"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type RangedContentStream struct {
	storage.RangeStream
	ContentType string
}

// OpenContentRange requires native range support; a backend without it must
// fail instead of transferring the whole object to discard its prefix.
func (s *Service) OpenContentRange(ctx context.Context, userID int64, id uuid.UUID, purpose, value string, metadataOnly bool) (RangedContentStream, error) {
	_, object, err := s.assetObject(ctx, userID, id, purpose)
	if err != nil {
		return RangedContentStream{}, err
	}
	ref, err := s.router.BackendFor(ctx, object.StorageConfigID, object.StorageDriver)
	if err != nil {
		return RangedContentStream{}, err
	}
	backend, ok := ref.Backend.(storage.RangeBackend)
	if !ok {
		return RangedContentStream{}, errs.New(503, errs.CodeArtifactStorageUnavailable, "media storage does not support ranged access")
	}
	stream, err := backend.OpenRange(ctx, object.ObjectKey, value, domainmedia.SingleFileHardMaxBytes, metadataOnly)
	return RangedContentStream{RangeStream: stream, ContentType: object.MIMEType}, err
}
