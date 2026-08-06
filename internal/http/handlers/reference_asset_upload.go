package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const referenceAssetMultipartOverheadBytes int64 = 1024 * 1024

func readReferenceAssetUpload(w http.ResponseWriter, r *http.Request, service *assetservice.Service) (string, string, []byte, *errs.Error) {
	policy, err := service.AttachmentPolicy(r.Context())
	if err != nil {
		return "", "", nil, normalizeAppError(fmt.Errorf("resolve attachment policy: %w", err))
	}
	maxBytes := policy.Image.MaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+referenceAssetMultipartOverheadBytes)

	file, header, err := r.FormFile("file")
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return "", "", nil, referenceAssetTooLargeError(policy.Image, maxBytes+1)
		}
		return "", "", nil, errs.New(http.StatusBadRequest, errs.CodeImageReferenceRequired, "file is required")
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", "", nil, errs.BadRequest("failed to read upload")
	}
	if int64(len(content)) > maxBytes {
		return "", "", nil, referenceAssetTooLargeError(policy.Image, int64(len(content)))
	}
	return header.Filename, header.Header.Get("Content-Type"), content, nil
}

func referenceAssetTooLargeError(policy assetservice.FilePolicy, actualSize int64) *errs.Error {
	return errs.WithDetails(
		errs.New(http.StatusBadRequest, errs.CodeImageReferenceTooLarge, fmt.Sprintf("参考图文件超过 %d MB，请压缩后重新上传。", policy.MaxMB)),
		map[string]any{
			"max_size_bytes":    policy.MaxBytes,
			"max_size_mb":       policy.MaxMB,
			"actual_size_bytes": actualSize,
		},
	)
}
