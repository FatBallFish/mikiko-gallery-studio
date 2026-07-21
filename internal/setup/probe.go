package setup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const (
	ProbeCodeOK                     = "OK"
	ProbeCodeInvalidConfiguration   = "INVALID_CONFIGURATION"
	ProbeCodeAuthenticationFailed   = "AUTHENTICATION_FAILED"
	ProbeCodeConnectionFailed       = "CONNECTION_FAILED"
	ProbeCodeInsufficientPrivileges = "INSUFFICIENT_PRIVILEGES"
	ProbeCodeUnsafePrivileges       = "UNSAFE_PRIVILEGES"
	ProbeCodeReadWriteCheckFailed   = "READ_WRITE_CHECK_FAILED"
	ProbeCodeCleanupFailed          = "CLEANUP_FAILED"
	ProbeCodeTimeout                = "PROBE_TIMEOUT"
	ProbeCodeCancelled              = "PROBE_CANCELLED"
	ProbeCodeInternalError          = "INTERNAL_ERROR"

	defaultProbeTimeout = 10 * time.Second
	minimumProbeTimeout = time.Second
	maximumProbeTimeout = 30 * time.Second
	defaultProbeWorkers = 8
)

type ProbeResult struct {
	Kind      string `json:"kind"`
	Success   bool   `json:"success"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
	Version   string `json:"version,omitempty"`
}

type PostgresProbeRequest struct {
	DatabaseURL string `json:"database_url"`
}

type RedisProbeRequest struct {
	RedisURL  string `json:"redis_url"`
	KeyPrefix string `json:"key_prefix"`
}

type StorageProbeRequest struct {
	Config config.StorageConfig `json:"config"`
}

type postgresProbeRunner func(context.Context, string) (string, error)
type redisProbeRunner func(context.Context, RedisProbeRequest) (string, error)
type storageProbeRunner func(context.Context, config.StorageConfig) (string, error)

type probeDependencies struct {
	timeout       time.Duration
	maxConcurrent int
	postgres      postgresProbeRunner
	redis         redisProbeRunner
	storage       storageProbeRunner
}

type ProbeService struct {
	timeout  time.Duration
	postgres postgresProbeRunner
	redis    redisProbeRunner
	storage  storageProbeRunner
	slots    chan struct{}
}

type probeExecution struct {
	version string
	err     error
}

type probeFailure struct {
	code  string
	cause error
}

func (failure *probeFailure) Error() string { return failure.code }
func (failure *probeFailure) Unwrap() error { return failure.cause }

func probeFailureError(code string, cause error) error {
	return &probeFailure{code: code, cause: cause}
}

func NewProbeService() *ProbeService {
	return newProbeService(probeDependencies{})
}

func newProbeService(dependencies probeDependencies) *ProbeService {
	timeout := dependencies.timeout
	if timeout == 0 {
		timeout = defaultProbeTimeout
	}
	if timeout < minimumProbeTimeout && dependencies.timeout == 0 {
		timeout = minimumProbeTimeout
	}
	if timeout > maximumProbeTimeout {
		timeout = maximumProbeTimeout
	}
	maxConcurrent := dependencies.maxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultProbeWorkers
	}
	if dependencies.postgres == nil {
		dependencies.postgres = runPostgresProbe
	}
	if dependencies.redis == nil {
		dependencies.redis = runRedisProbe
	}
	if dependencies.storage == nil {
		dependencies.storage = runStorageProbe
	}
	return &ProbeService{
		timeout: timeout, postgres: dependencies.postgres,
		redis: dependencies.redis, storage: dependencies.storage,
		slots: make(chan struct{}, maxConcurrent),
	}
}

func (service *ProbeService) ProbePostgres(ctx context.Context, request PostgresProbeRequest) ProbeResult {
	started := time.Now()
	if err := validatePostgresProbeRequest(request); err != nil {
		return failedProbeResult("database", ProbeCodeInvalidConfiguration, started)
	}
	return service.run(ctx, "database", started, func(runCtx context.Context) (string, error) {
		return service.postgres(runCtx, request.DatabaseURL)
	})
}

func (service *ProbeService) ProbeRedis(ctx context.Context, request RedisProbeRequest) ProbeResult {
	started := time.Now()
	if err := validateRedisProbeRequest(request); err != nil {
		return failedProbeResult("redis", ProbeCodeInvalidConfiguration, started)
	}
	return service.run(ctx, "redis", started, func(runCtx context.Context) (string, error) {
		return service.redis(runCtx, request)
	})
}

func (service *ProbeService) ProbeStorage(ctx context.Context, request StorageProbeRequest) ProbeResult {
	started := time.Now()
	if err := validateStorageProbeSyntax(request.Config); err != nil {
		return failedProbeResult("storage", ProbeCodeInvalidConfiguration, started)
	}
	return service.run(ctx, "storage", started, func(runCtx context.Context) (string, error) {
		return service.storage(runCtx, request.Config)
	})
}

func (service *ProbeService) run(ctx context.Context, kind string, started time.Time, runner func(context.Context) (string, error)) ProbeResult {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(ctx, service.timeout)
	defer cancel()
	if err := runCtx.Err(); err != nil {
		return failedProbeResult(kind, classifyProbeError(kind, err), started)
	}
	select {
	case service.slots <- struct{}{}:
	case <-runCtx.Done():
		return failedProbeResult(kind, classifyProbeError(kind, runCtx.Err()), started)
	}
	if err := runCtx.Err(); err != nil {
		<-service.slots
		return failedProbeResult(kind, classifyProbeError(kind, err), started)
	}

	resultChannel := make(chan probeExecution, 1)
	go func() {
		defer func() { <-service.slots }()
		execution := probeExecution{}
		func() {
			defer func() {
				if recover() != nil {
					execution.err = probeFailureError(ProbeCodeInternalError, errors.New("probe runner panicked"))
				}
			}()
			if err := runCtx.Err(); err != nil {
				execution.err = err
				return
			}
			execution.version, execution.err = runner(runCtx)
		}()
		resultChannel <- execution
	}()

	var execution probeExecution
	select {
	case execution = <-resultChannel:
	case <-runCtx.Done():
		return failedProbeResult(kind, classifyProbeError(kind, runCtx.Err()), started)
	}
	if err := runCtx.Err(); err != nil {
		return failedProbeResult(kind, classifyProbeError(kind, err), started)
	}
	if execution.err != nil {
		return failedProbeResult(kind, classifyProbeError(kind, execution.err), started)
	}
	version := sanitizeProbeVersion(execution.version)
	return ProbeResult{
		Kind: kind, Success: true, Code: ProbeCodeOK,
		Message: probeMessage(kind, ProbeCodeOK), LatencyMS: probeLatencyMS(started),
		Version: version,
	}
}

func classifyProbeError(kind string, err error) string {
	var failure *probeFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProbeCodeTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ProbeCodeCancelled
	}
	if kind == "database" {
		var postgresError *pq.Error
		if errors.As(err, &postgresError) {
			switch string(postgresError.Code) {
			case "28P01", "28000":
				return ProbeCodeAuthenticationFailed
			case "42501":
				return ProbeCodeInsufficientPrivileges
			}
		}
	}
	if kind == "redis" {
		upper := strings.ToUpper(err.Error())
		if strings.Contains(upper, "WRONGPASS") || strings.Contains(upper, "NOAUTH") || strings.Contains(upper, "AUTHENTICATION") {
			return ProbeCodeAuthenticationFailed
		}
	}
	return ProbeCodeConnectionFailed
}

func failedProbeResult(kind, code string, started time.Time) ProbeResult {
	return ProbeResult{
		Kind: kind, Success: false, Code: code,
		Message: probeMessage(kind, code), LatencyMS: probeLatencyMS(started),
	}
}

func probeLatencyMS(started time.Time) int64 {
	latency := time.Since(started).Milliseconds()
	if latency < 0 {
		return 0
	}
	return latency
}

func sanitizeProbeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, version)
	if len(version) > 128 {
		version = version[:128]
	}
	return version
}

func probeMessage(kind, code string) string {
	if code == ProbeCodeOK {
		switch kind {
		case "database":
			return "PostgreSQL connection and schema privileges verified."
		case "redis":
			return "Redis connection and read/write access verified."
		case "storage":
			return "Object storage read/write access verified."
		}
	}
	switch code {
	case ProbeCodeInvalidConfiguration:
		return "The submitted middleware configuration is invalid."
	case ProbeCodeAuthenticationFailed:
		return "Middleware authentication failed."
	case ProbeCodeConnectionFailed:
		return "The middleware endpoint could not be reached."
	case ProbeCodeInsufficientPrivileges:
		return "The middleware account lacks required privileges."
	case ProbeCodeUnsafePrivileges:
		return "The database account has unsafe server-level privileges."
	case ProbeCodeReadWriteCheckFailed:
		return "The middleware read/write check failed."
	case ProbeCodeCleanupFailed:
		return "The middleware probe could not clean up its temporary data."
	case ProbeCodeTimeout:
		return "The middleware probe timed out."
	case ProbeCodeCancelled:
		return "The middleware probe was cancelled."
	default:
		return "The middleware probe failed."
	}
}
