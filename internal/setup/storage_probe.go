package setup

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type storageBackendFactory func(config.StorageConfig) (storage.Backend, error)

var s3BucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validateStorageProbeConfig(storageConfig config.StorageConfig) error {
	_, err := normalizeStorageProbeConfig(storageConfig)
	return err
}

func normalizeStorageProbeConfig(storageConfig config.StorageConfig) (config.StorageConfig, error) {
	switch strings.ToLower(strings.TrimSpace(storageConfig.Driver)) {
	case "local":
		root := strings.TrimSpace(storageConfig.LocalRoot)
		if root == "" || strings.ContainsRune(root, 0) {
			return config.StorageConfig{}, errors.New("local storage root is required")
		}
		resolved, err := resolveLocalProbeRoot(root)
		if err != nil {
			return config.StorageConfig{}, err
		}
		// This canonical path is a point-in-time probe target only. The submitted
		// draft remains unchanged and final setup apply must validate/probe again.
		storageConfig.Driver = "local"
		storageConfig.LocalRoot = resolved
		return storageConfig, nil
	case "s3":
		endpoint, err := url.Parse(strings.TrimSpace(storageConfig.S3.Endpoint))
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" || (endpoint.EscapedPath() != "" && endpoint.EscapedPath() != "/") {
			return config.StorageConfig{}, errors.New("invalid S3 endpoint")
		}
		if strings.TrimSpace(storageConfig.S3.Region) == "" ||
			!s3BucketPattern.MatchString(strings.TrimSpace(storageConfig.S3.Bucket)) ||
			strings.TrimSpace(storageConfig.S3.AccessKeyID) == "" ||
			strings.TrimSpace(storageConfig.S3.SecretAccessKey) == "" {
			return config.StorageConfig{}, errors.New("invalid S3 configuration")
		}
		prefix := strings.Trim(strings.TrimSpace(storageConfig.S3.Prefix), "/")
		if prefix != "" {
			for _, component := range strings.Split(prefix, "/") {
				if component == ".." {
					return config.StorageConfig{}, errors.New("invalid S3 prefix")
				}
			}
			cleaned := path.Clean(prefix)
			if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
				return config.StorageConfig{}, errors.New("invalid S3 prefix")
			}
		}
		storageConfig.Driver = "s3"
		return storageConfig, nil
	default:
		return config.StorageConfig{}, errors.New("unsupported storage driver")
	}
}

func resolveLocalProbeRoot(value string) (string, error) {
	if localProbePathHasParentTraversal(value) {
		return "", errors.New("local storage root cannot contain parent traversal")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("local storage root cannot be a symlink")
		}
		if !info.IsDir() {
			return "", errors.New("local storage root is not a directory")
		}
		return filepath.EvalSymlinks(absolute)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	current := absolute
	missing := make([]string, 0, 4)
	for {
		parent := filepath.Dir(current)
		missing = append(missing, filepath.Base(current))
		parentInfo, statErr := os.Lstat(parent)
		if statErr == nil {
			if !parentInfo.IsDir() && parentInfo.Mode()&os.ModeSymlink == 0 {
				return "", errors.New("local storage root ancestor is not a directory")
			}
			resolved, resolveErr := filepath.EvalSymlinks(parent)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return resolved, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) || parent == current {
			return "", statErr
		}
		current = parent
	}
}

func localProbePathHasParentTraversal(value string) bool {
	pathWithoutVolume := value
	if volume := filepath.VolumeName(value); volume != "" {
		pathWithoutVolume = strings.TrimPrefix(value, volume)
	} else if len(value) >= 2 && value[1] == ':' && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		// Recognize Windows drive syntax even when tests run on a non-Windows host.
		pathWithoutVolume = value[2:]
	}
	for _, component := range strings.Split(strings.ReplaceAll(pathWithoutVolume, "\\", "/"), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

func runStorageProbe(ctx context.Context, storageConfig config.StorageConfig) (string, error) {
	return runStorageProbeWithFactory(ctx, storageConfig, cryptorand.Reader, storage.NewBackend)
}

func runStorageProbeWithFactory(ctx context.Context, storageConfig config.StorageConfig, random io.Reader, factory storageBackendFactory) (version string, resultErr error) {
	backend, err := factory(storageConfig)
	if err != nil {
		return "", probeFailureError(ProbeCodeConnectionFailed, err)
	}
	randomBytes := make([]byte, 32)
	if _, err := io.ReadFull(random, randomBytes); err != nil {
		clear(randomBytes)
		return "", probeFailureError(ProbeCodeInternalError, err)
	}
	objectKey := "setup-probe-" + hex.EncodeToString(randomBytes[:16])
	content := append([]byte(nil), randomBytes[16:]...)
	clear(randomBytes)

	cleanupRequired := true
	defer func() {
		if !cleanupRequired {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cleanupErr := backend.Delete(cleanupCtx, objectKey)
		if cleanupErr != nil && !errors.Is(cleanupErr, storage.ErrNotFound) {
			resultErr = probeFailureError(ProbeCodeCleanupFailed, errors.Join(resultErr, cleanupErr))
		}
	}()
	if err := backend.Put(ctx, objectKey, "application/octet-stream", content); err != nil {
		return "", storageProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	loaded, err := backend.Get(ctx, objectKey)
	if err != nil || !bytes.Equal(loaded, content) {
		if err == nil {
			err = errors.New("storage probe value mismatch")
		}
		return "", storageProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	if err := backend.Delete(ctx, objectKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", storageProbeFailure(err, ProbeCodeCleanupFailed)
	}
	cleanupRequired = false
	switch strings.ToLower(strings.TrimSpace(storageConfig.Driver)) {
	case "local":
		return "local", nil
	default:
		return "s3-compatible", nil
	}
}

func storageProbeFailure(err error, fallback string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return probeFailureError(fallback, err)
}
