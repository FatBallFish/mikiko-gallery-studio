package imagetask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

const (
	redisConcurrencyMinPollInterval = 10 * time.Millisecond
	redisConcurrencyMaxPollInterval = 100 * time.Millisecond
	redisConcurrencyReleaseTimeout  = 2 * time.Second
)

var redisConcurrencyAcquireScript = redis.NewScript(`
local now_reply = redis.call('TIME')
local now_ms = tonumber(now_reply[1]) * 1000 + math.floor(tonumber(now_reply[2]) / 1000)
local lease_id = ARGV[1]
local lease_ttl_ms = tonumber(ARGV[2])
local blocked = false
local wait_ms = lease_ttl_ms

for index, key in ipairs(KEYS) do
  redis.call('ZREMRANGEBYSCORE', key, '-inf', now_ms)
  local limit = tonumber(ARGV[index + 2])
  if redis.call('ZCARD', key) >= limit then
    blocked = true
    local earliest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    if earliest[2] then
      local remaining = tonumber(earliest[2]) - now_ms
      if remaining < wait_ms then
        wait_ms = remaining
      end
    end
  end
end

if blocked then
  return {0, math.max(wait_ms, 1)}
end

local expires_at_ms = now_ms + lease_ttl_ms
for _, key in ipairs(KEYS) do
  redis.call('ZADD', key, expires_at_ms, lease_id)
  redis.call('PEXPIRE', key, lease_ttl_ms + 1000)
end
return {1, lease_ttl_ms}
`)

var redisConcurrencyReleaseScript = redis.NewScript(`
local lease_id = ARGV[1]
local removed = 0
for _, key in ipairs(KEYS) do
  removed = removed + redis.call('ZREM', key, lease_id)
  if redis.call('ZCARD', key) == 0 then
    redis.call('DEL', key)
  end
end
return removed
`)

type redisConcurrencyGate struct {
	client    redis.UniversalClient
	keyPrefix string
}

func NewRedisConcurrencyGate(client redis.UniversalClient, keyPrefix string) ConcurrencyGate {
	if client == nil {
		return NewLocalConcurrencyGate()
	}
	return &redisConcurrencyGate{
		client:    client,
		keyPrefix: strings.Trim(strings.TrimSpace(keyPrefix), ":"),
	}
}

func (g *redisConcurrencyGate) Acquire(ctx context.Context, resources []ConcurrencyResource, leaseTTL time.Duration) (func(), error) {
	resources = normalizeConcurrencyResources(resources)
	if len(resources) == 0 {
		return func() {}, nil
	}
	if leaseTTL <= 0 {
		leaseTTL = time.Second
	}

	leaseID := uuid.NewString()
	keys := make([]string, 0, len(resources))
	args := make([]any, 0, len(resources)+2)
	args = append(args, leaseID, leaseTTL.Milliseconds())
	for _, resource := range resources {
		keys = append(keys, g.key(resource.Key))
		args = append(args, resource.Limit)
	}

	for {
		result, err := redisConcurrencyAcquireScript.Run(ctx, g.client, keys, args...).Slice()
		if err != nil {
			return nil, fmt.Errorf("acquire image concurrency lease: %w", err)
		}
		if len(result) != 2 {
			return nil, fmt.Errorf("acquire image concurrency lease: unexpected Redis result length %d", len(result))
		}
		acquired, err := redisResultInt64(result[0])
		if err != nil {
			return nil, fmt.Errorf("acquire image concurrency lease: decode acquired result: %w", err)
		}
		waitMS, err := redisResultInt64(result[1])
		if err != nil {
			return nil, fmt.Errorf("acquire image concurrency lease: decode wait result: %w", err)
		}
		if acquired == 1 {
			if err := ctx.Err(); err != nil {
				g.release(keys, leaseID)
				return nil, err
			}
			var once sync.Once
			return func() {
				once.Do(func() { g.release(keys, leaseID) })
			}, nil
		}

		wait := time.Duration(waitMS) * time.Millisecond
		if wait < redisConcurrencyMinPollInterval {
			wait = redisConcurrencyMinPollInterval
		}
		if wait > redisConcurrencyMaxPollInterval {
			wait = redisConcurrencyMaxPollInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *redisConcurrencyGate) release(keys []string, leaseID string) {
	ctx, cancel := context.WithTimeout(context.Background(), redisConcurrencyReleaseTimeout)
	defer cancel()
	if err := redisConcurrencyReleaseScript.Run(ctx, g.client, keys, leaseID).Err(); err != nil {
		slog.Warn("failed to release image concurrency lease", "lease_id", leaseID, "err", err)
	}
}

func (g *redisConcurrencyGate) key(resourceKey string) string {
	digest := sha256.Sum256([]byte(resourceKey))
	key := "{image-concurrency}:" + hex.EncodeToString(digest[:16])
	if g.keyPrefix != "" {
		return g.keyPrefix + ":" + key
	}
	return key
}

func redisResultInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected type %T", value)
	}
}
