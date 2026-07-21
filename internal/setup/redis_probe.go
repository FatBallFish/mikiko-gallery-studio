package setup

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var redisProbePrefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func validateRedisProbeRequest(request RedisProbeRequest) error {
	parsed, err := url.Parse(strings.TrimSpace(request.RedisURL))
	if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" || parsed.Opaque != "" || parsed.Fragment != "" {
		return errors.New("invalid Redis URL")
	}
	if _, err := redis.ParseURL(request.RedisURL); err != nil {
		return errors.New("invalid Redis URL")
	}
	if !redisProbePrefixPattern.MatchString(strings.TrimSpace(request.KeyPrefix)) {
		return errors.New("invalid Redis key prefix")
	}
	return nil
}

func runRedisProbe(ctx context.Context, request RedisProbeRequest) (version string, resultErr error) {
	options, err := redis.ParseURL(request.RedisURL)
	if err != nil {
		return "", probeFailureError(ProbeCodeInvalidConfiguration, err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		return "", redisProbeFailure(err, ProbeCodeConnectionFailed)
	}

	random := make([]byte, 24)
	if _, err := cryptorand.Read(random); err != nil {
		return "", probeFailureError(ProbeCodeInternalError, err)
	}
	key := strings.TrimSpace(request.KeyPrefix) + ":setup-probe:" + hex.EncodeToString(random[:12])
	value := hex.EncodeToString(random[12:])
	clear(random)

	cleanup := func(cleanupCtx context.Context, requireDeleted bool) error {
		const compareAndDelete = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
		deleted, err := client.Eval(cleanupCtx, compareAndDelete, []string{key}, value).Int64()
		if err != nil {
			return err
		}
		if requireDeleted && deleted != 1 {
			return errors.New("Redis probe key was not deleted")
		}
		return nil
	}
	cleanupPending := false
	setSucceeded := false
	defer func() {
		if !cleanupPending || resultErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if cleanupErr := cleanup(cleanupCtx, setSucceeded); cleanupErr != nil {
			resultErr = probeFailureError(ProbeCodeCleanupFailed, errors.Join(resultErr, cleanupErr))
		}
	}()
	cleanupPending = true
	if err := client.Set(ctx, key, value, time.Minute).Err(); err != nil {
		return "", redisProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	setSucceeded = true
	loaded, err := client.Get(ctx, key).Result()
	if err != nil || loaded != value {
		if err == nil {
			err = errors.New("Redis probe value mismatch")
		}
		return "", redisProbeFailure(err, ProbeCodeReadWriteCheckFailed)
	}
	if err := cleanup(ctx, true); err != nil {
		return "", redisProbeFailure(err, ProbeCodeCleanupFailed)
	}
	cleanupPending = false

	info, err := client.Info(ctx, "server").Result()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", err
		}
		return "", nil
	}
	version = parseRedisVersion(info)
	return version, nil
}

func redisProbeFailure(err error, fallback string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	upper := strings.ToUpper(err.Error())
	if strings.Contains(upper, "WRONGPASS") || strings.Contains(upper, "NOAUTH") || strings.Contains(upper, "AUTHENTICATION") {
		return probeFailureError(ProbeCodeAuthenticationFailed, err)
	}
	if strings.Contains(upper, "NOPERM") || strings.Contains(upper, "PERMISSION") {
		return probeFailureError(ProbeCodeInsufficientPrivileges, err)
	}
	return probeFailureError(fallback, err)
}

func parseRedisVersion(info string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "redis_version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
		}
	}
	return ""
}
