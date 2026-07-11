package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

var (
	ErrNoDefaultStorage  = errors.New("default storage config is unavailable")
	ErrStorageUnreadable = errors.New("storage config is not readable")
)

const (
	storageProbeTimeout        = 10 * time.Second
	storageProbeCleanupTimeout = 5 * time.Second
)

type BackendRef struct {
	ConfigID string
	Driver   string
	Backend  Backend
}

type Router interface {
	DefaultWriter(ctx context.Context) (BackendRef, error)
	BackendFor(ctx context.Context, configID string, legacyDriver string) (BackendRef, error)
}

type ConfigSource interface {
	ResolveDefaultWritable(ctx context.Context) (domainstorageconfig.ResolvedConfig, error)
	ResolveByID(ctx context.Context, id string) (domainstorageconfig.ResolvedConfig, error)
	ResolveLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ResolvedConfig, error)
}

type StaticRouter struct {
	ref BackendRef
}

func NewStaticRouter(backend Backend) *StaticRouter {
	if backend == nil {
		backend = NewLocalBackend("")
	}
	return &StaticRouter{ref: BackendRef{Driver: backend.Driver(), Backend: backend}}
}

func (r *StaticRouter) DefaultWriter(context.Context) (BackendRef, error) {
	return r.ref, nil
}

func (r *StaticRouter) BackendFor(_ context.Context, configID string, legacyDriver string) (BackendRef, error) {
	ref := r.ref
	if strings.TrimSpace(configID) != "" {
		ref.ConfigID = strings.TrimSpace(configID)
	}
	if strings.TrimSpace(legacyDriver) != "" {
		ref.Driver = strings.TrimSpace(legacyDriver)
	}
	return ref, nil
}

type Registry struct {
	source ConfigSource
	ttl    time.Duration
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedBackend
}

type cachedBackend struct {
	ref       BackendRef
	expiresAt time.Time
}

func NewRegistry(source ConfigSource, ttl time.Duration) *Registry {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Registry{source: source, ttl: ttl, now: time.Now, cache: map[string]cachedBackend{}}
}

func (r *Registry) DefaultWriter(ctx context.Context) (BackendRef, error) {
	if r == nil || r.source == nil {
		return BackendRef{}, ErrNoDefaultStorage
	}
	resolved, err := r.source.ResolveDefaultWritable(ctx)
	if err != nil {
		return BackendRef{}, err
	}
	if resolved.Status != domainstorageconfig.StatusEnabled || !resolved.ReadEnabled || !resolved.WriteEnabled {
		return BackendRef{}, ErrNoDefaultStorage
	}
	return r.backendForResolved(resolved)
}

func (r *Registry) BackendFor(ctx context.Context, configID string, legacyDriver string) (BackendRef, error) {
	if r == nil || r.source == nil {
		return BackendRef{}, ErrStorageUnreadable
	}
	var (
		resolved domainstorageconfig.ResolvedConfig
		err      error
	)
	if strings.TrimSpace(configID) != "" {
		resolved, err = r.source.ResolveByID(ctx, strings.TrimSpace(configID))
	} else {
		resolved, err = r.source.ResolveLegacyByDriver(ctx, strings.TrimSpace(legacyDriver))
	}
	if err != nil {
		return BackendRef{}, err
	}
	if resolved.Status == domainstorageconfig.StatusDeleted || !resolved.ReadEnabled {
		return BackendRef{}, ErrStorageUnreadable
	}
	return r.backendForResolved(resolved)
}

func (r *Registry) Probe(ctx context.Context, resolved domainstorageconfig.ResolvedConfig) domainstorageconfig.ProbeResult {
	start := r.now()
	backend, err := NewBackend(StorageConfigFromResolved(resolved))
	if err != nil {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, storageProbeTimeout)
	defer cancel()
	key := ".pic-gallery-probe/" + start.UTC().Format("20060102150405.000000000") + ".txt"
	return r.probeBackend(ctx, backend, key, start)
}

func (r *Registry) probeBackend(ctx context.Context, backend Backend, key string, start time.Time) domainstorageconfig.ProbeResult {
	content := []byte("pic-gallery-storage-probe")
	if err := backend.Put(ctx, key, "text/plain", content); err != nil {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), LatencyMS: int64(r.now().Sub(start) / time.Millisecond), Message: err.Error()}
	}
	cleanup := func() string {
		if err := cleanupProbeObject(backend, key); err != nil {
			return "; cleanup failed: " + err.Error()
		}
		return ""
	}
	got, err := backend.Get(ctx, key)
	if err != nil {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), LatencyMS: int64(r.now().Sub(start) / time.Millisecond), Message: err.Error() + cleanup()}
	}
	if !bytes.Equal(got, content) {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), LatencyMS: int64(r.now().Sub(start) / time.Millisecond), Message: "probe object content mismatch" + cleanup()}
	}
	if err := cleanupProbeObject(backend, key); err != nil {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), LatencyMS: int64(r.now().Sub(start) / time.Millisecond), Message: err.Error()}
	}
	return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusSuccess, CheckedAt: r.now().UTC(), LatencyMS: int64(r.now().Sub(start) / time.Millisecond), Message: "put/get/delete probe object succeeded"}
}

func cleanupProbeObject(backend Backend, key string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), storageProbeCleanupTimeout)
	defer cancel()
	if err := backend.Delete(cleanupCtx, key); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("delete probe object %q: %w", key, err)
	}
	return nil
}

func (r *Registry) backendForResolved(resolved domainstorageconfig.ResolvedConfig) (BackendRef, error) {
	if strings.TrimSpace(resolved.ID) == "" {
		return BackendRef{}, fmt.Errorf("storage config id is required")
	}
	cacheKey := strings.Join([]string{resolved.ID, fmt.Sprintf("%d", resolved.Version), resolved.SecretFingerprint}, ":")
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.cache[cacheKey]; ok && cached.expiresAt.After(now) {
		return cached.ref, nil
	}
	backend, err := NewBackend(StorageConfigFromResolved(resolved))
	if err != nil {
		return BackendRef{}, err
	}
	ref := BackendRef{ConfigID: resolved.ID, Driver: backend.Driver(), Backend: backend}
	for key, cached := range r.cache {
		if cached.ref.ConfigID == resolved.ID {
			delete(r.cache, key)
		}
	}
	r.cache[cacheKey] = cachedBackend{ref: ref, expiresAt: now.Add(r.ttl)}
	return ref, nil
}

func StorageConfigFromResolved(resolved domainstorageconfig.ResolvedConfig) config.StorageConfig {
	cfg := config.StorageConfig{
		Driver:        strings.TrimSpace(resolved.Driver),
		LocalRoot:     strings.TrimSpace(resolved.LocalRoot),
		PublicBaseURL: strings.TrimSpace(resolved.PublicBaseURL),
		S3: config.StorageS3Config{
			Endpoint:        strings.TrimSpace(resolved.Endpoint),
			Region:          strings.TrimSpace(resolved.Region),
			Bucket:          strings.TrimSpace(resolved.Bucket),
			ForcePathStyle:  resolved.ForcePathStyle,
			Prefix:          strings.Trim(strings.TrimSpace(resolved.Prefix), "/"),
			AccessKeyID:     strings.TrimSpace(fmt.Sprint(resolved.Secrets["access_key_id"])),
			SecretAccessKey: strings.TrimSpace(fmt.Sprint(resolved.Secrets["secret_access_key"])),
		},
	}
	if cfg.Driver == "" {
		cfg.Driver = "local"
	}
	return cfg
}
