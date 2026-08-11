package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	localMultipartHardMaxBytes    = int64(1 << 30)
	localMultipartDiskSafetyBytes = int64(2 << 30)
)

type localMultipartState struct {
	Upload    MultipartUpload           `json:"upload"`
	Status    string                    `json:"status"`
	Completed *CompletedMultipartObject `json:"completed,omitempty"`
}

func (b *LocalBackend) CreateMultipart(ctx context.Context, req MultipartCreateRequest) (MultipartUpload, error) {
	if err := contextError(ctx); err != nil {
		return MultipartUpload{}, err
	}
	upload, err := validateMultipartCreate(req)
	if err != nil {
		return MultipartUpload{}, err
	}
	if upload.SizeBytes > localMultipartHardMaxBytes {
		return MultipartUpload{}, ErrObjectTooLarge
	}
	if _, ok := b.resolvePath(upload.ObjectKey); !ok {
		return MultipartUpload{}, fmt.Errorf("invalid local storage key %q", upload.ObjectKey)
	}
	if err := b.ensureMultipartDiskBudget(upload.SizeBytes); err != nil {
		return MultipartUpload{}, err
	}
	upload.UploadID = uuid.NewString()
	upload.Driver = "local"
	directory, err := b.localMultipartDirectory(upload.UploadID)
	if err != nil {
		return MultipartUpload{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return MultipartUpload{}, fmt.Errorf("create local multipart directory: %w", err)
	}
	if err := writeLocalMultipartState(directory, localMultipartState{Upload: upload, Status: "initialized"}); err != nil {
		_ = os.RemoveAll(directory)
		return MultipartUpload{}, err
	}
	return upload, nil
}

func (*LocalBackend) SignMultipartPart(context.Context, MultipartUpload, int, string, time.Duration) (MultipartPartTarget, error) {
	return MultipartPartTarget{}, ErrDirectUploadRequired
}

func (b *LocalBackend) PutMultipartPart(ctx context.Context, upload MultipartUpload, partNumber int, reader io.Reader, size int64, checksum string) (CompletedPart, error) {
	if reader == nil {
		return CompletedPart{}, ErrSizeMismatch
	}
	state, directory, err := b.loadLocalMultipartState(upload)
	if err != nil {
		return CompletedPart{}, err
	}
	if state.Status == "completed" {
		return CompletedPart{}, errors.New("multipart upload is already completed")
	}
	expectedSize, err := expectedMultipartPartSize(state.Upload, partNumber)
	if err != nil {
		return CompletedPart{}, err
	}
	if size != expectedSize {
		return CompletedPart{}, ErrSizeMismatch
	}
	wantHex, _, err := normalizeSHA256Checksum(checksum)
	if err != nil {
		return CompletedPart{}, err
	}
	temporary, err := os.CreateTemp(directory, fmt.Sprintf(".%06d-*", partNumber))
	if err != nil {
		return CompletedPart{}, err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	hasher := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(temporary, hasher), contextReader{ctx: ctx, reader: reader}, size)
	if copyErr != nil || written != size {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return CompletedPart{}, ctxErr
		}
		return CompletedPart{}, ErrSizeMismatch
	}
	var extra [1]byte
	if count, readErr := reader.Read(extra[:]); count != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return CompletedPart{}, ErrSizeMismatch
	}
	gotHex := hex.EncodeToString(hasher.Sum(nil))
	if gotHex != wantHex {
		return CompletedPart{}, ErrMultipartChecksum
	}
	if err := temporary.Sync(); err != nil {
		return CompletedPart{}, err
	}
	if err := temporary.Close(); err != nil {
		return CompletedPart{}, err
	}
	partPath := localMultipartPartPath(directory, partNumber)
	if err := os.Rename(temporaryPath, partPath); err != nil {
		return CompletedPart{}, err
	}
	if err := writeAtomicFile(partPath+".sha256", []byte(gotHex), 0o600); err != nil {
		_ = os.Remove(partPath)
		return CompletedPart{}, err
	}
	committed = true
	return CompletedPart{PartNumber: partNumber, ETag: gotHex, Checksum: gotHex, SizeBytes: size}, nil
}

func (b *LocalBackend) MultipartStatus(ctx context.Context, upload MultipartUpload) (MultipartStatus, error) {
	if err := contextError(ctx); err != nil {
		return MultipartStatus{}, err
	}
	state, directory, err := b.loadLocalMultipartState(upload)
	if err != nil {
		return MultipartStatus{}, err
	}
	parts, err := scanLocalMultipartParts(directory, state.Upload)
	if err != nil {
		return MultipartStatus{}, err
	}
	return MultipartStatus{Status: state.Status, CompletedParts: parts}, nil
}

func (b *LocalBackend) CompleteMultipart(ctx context.Context, upload MultipartUpload, parts []CompletedPart) (CompletedMultipartObject, error) {
	state, directory, err := b.loadLocalMultipartState(upload)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	if state.Status == "completed" && state.Completed != nil {
		return *state.Completed, nil
	}
	parts, err = normalizeCompletedParts(parts, state.Upload.PartCount)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	storedParts, err := scanLocalMultipartParts(directory, state.Upload)
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	if len(storedParts) != state.Upload.PartCount {
		return CompletedMultipartObject{}, ErrMultipartPartMissing
	}
	for index := range storedParts {
		if storedParts[index].PartNumber != parts[index].PartNumber || storedParts[index].ETag != strings.Trim(parts[index].ETag, `"`) {
			return CompletedMultipartObject{}, ErrMultipartChecksum
		}
	}
	destination, ok := b.resolvePath(state.Upload.ObjectKey)
	if !ok {
		return CompletedMultipartObject{}, errors.New("invalid local multipart object key")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return CompletedMultipartObject{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".multipart-complete-*")
	if err != nil {
		return CompletedMultipartObject{}, err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	hasher := sha256.New()
	var total int64
	for _, part := range storedParts {
		if err := contextError(ctx); err != nil {
			return CompletedMultipartObject{}, err
		}
		file, err := os.Open(localMultipartPartPath(directory, part.PartNumber))
		if err != nil {
			return CompletedMultipartObject{}, err
		}
		written, copyErr := io.Copy(io.MultiWriter(temporary, hasher), file)
		closeErr := file.Close()
		if copyErr != nil {
			return CompletedMultipartObject{}, copyErr
		}
		if closeErr != nil {
			return CompletedMultipartObject{}, closeErr
		}
		total += written
	}
	if total != state.Upload.SizeBytes {
		return CompletedMultipartObject{}, ErrSizeMismatch
	}
	if err := temporary.Sync(); err != nil {
		return CompletedMultipartObject{}, err
	}
	if err := temporary.Close(); err != nil {
		return CompletedMultipartObject{}, err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return CompletedMultipartObject{}, err
	}
	completed := CompletedMultipartObject{
		ObjectKey: state.Upload.ObjectKey, SizeBytes: total, SHA256: hex.EncodeToString(hasher.Sum(nil)), ETag: hex.EncodeToString(hasher.Sum(nil)),
	}
	state.Status = "completed"
	state.Completed = &completed
	if err := writeLocalMultipartState(directory, state); err != nil {
		_ = os.Remove(destination)
		return CompletedMultipartObject{}, err
	}
	for _, part := range storedParts {
		_ = os.Remove(localMultipartPartPath(directory, part.PartNumber))
		_ = os.Remove(localMultipartPartPath(directory, part.PartNumber) + ".sha256")
	}
	committed = true
	return completed, nil
}

func (b *LocalBackend) AbortMultipart(ctx context.Context, upload MultipartUpload) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	directory, err := b.localMultipartDirectory(upload.UploadID)
	if err != nil {
		return nil
	}
	if err := os.RemoveAll(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (b *LocalBackend) loadLocalMultipartState(upload MultipartUpload) (localMultipartState, string, error) {
	directory, err := b.localMultipartDirectory(upload.UploadID)
	if err != nil {
		return localMultipartState{}, "", ErrMultipartNotFound
	}
	payload, err := os.ReadFile(filepath.Join(directory, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return localMultipartState{}, "", ErrMultipartNotFound
	}
	if err != nil {
		return localMultipartState{}, "", err
	}
	var state localMultipartState
	if err := json.Unmarshal(payload, &state); err != nil || state.Upload.UploadID != upload.UploadID || state.Upload.ObjectKey != upload.ObjectKey {
		return localMultipartState{}, "", ErrMultipartNotFound
	}
	return state, directory, nil
}

func (b *LocalBackend) localMultipartDirectory(uploadID string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(uploadID))
	if err != nil || parsed.String() != strings.ToLower(strings.TrimSpace(uploadID)) {
		return "", ErrMultipartNotFound
	}
	directory, ok := b.resolvePath(filepath.ToSlash(filepath.Join("media", "uploads", ".sessions", parsed.String())))
	if !ok {
		return "", ErrMultipartNotFound
	}
	return directory, nil
}

func (b *LocalBackend) ensureMultipartDiskBudget(size int64) error {
	if err := os.MkdirAll(b.root, 0o755); err != nil {
		return err
	}
	var stats syscall.Statfs_t
	if err := syscall.Statfs(b.root, &stats); err != nil {
		return fmt.Errorf("inspect local multipart disk space: %w", err)
	}
	available := int64(stats.Bavail) * int64(stats.Bsize)
	required := size*2 + localMultipartDiskSafetyBytes
	if size > (1<<62) || available < required {
		return fmt.Errorf("insufficient local multipart disk space: available=%d required=%d", available, required)
	}
	return nil
}

func writeLocalMultipartState(directory string, state localMultipartState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeAtomicFile(filepath.Join(directory, "state.json"), payload, 0o600)
}

func writeAtomicFile(destination string, payload []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".state-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return err
	}
	committed = true
	return nil
}

func scanLocalMultipartParts(directory string, upload MultipartUpload) ([]CompletedPart, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	parts := make([]CompletedPart, 0, upload.PartCount)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".part"))
		if err != nil || number < 1 || number > upload.PartCount {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		expected, err := expectedMultipartPartSize(upload, number)
		if err != nil || info.Size() != expected {
			continue
		}
		checksum, err := os.ReadFile(filepath.Join(directory, entry.Name()+".sha256"))
		if err != nil || len(strings.TrimSpace(string(checksum))) != 64 {
			continue
		}
		parts = append(parts, CompletedPart{PartNumber: number, ETag: strings.TrimSpace(string(checksum)), Checksum: strings.TrimSpace(string(checksum)), SizeBytes: info.Size()})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	return parts, nil
}

func localMultipartPartPath(directory string, partNumber int) string {
	return filepath.Join(directory, fmt.Sprintf("%06d.part", partNumber))
}
