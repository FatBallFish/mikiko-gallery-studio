package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

var knownEmailCodeScenes = []string{"login", "password_reset"}

type RedisRuntime interface {
	EmailCooldownActive(ctx context.Context, email string) (bool, error)
	StoreEmailCode(ctx context.Context, email string, record emailCode, codeTTL time.Duration, cooldownTTL time.Duration) error
	LoadEmailCode(ctx context.Context, email string, allowedScenes []string) (emailCode, bool, error)
	DeleteEmailCodes(ctx context.Context, email string, allowedScenes []string) error
	StoreRefreshTokenState(ctx context.Context, tokenHash string, session refreshSession, ttl time.Duration) error
	LoadRefreshTokenState(ctx context.Context, tokenHash string) (refreshSession, bool, error)
	MarkRefreshFamilyReplayBlocked(ctx context.Context, familyID string, ttl time.Duration) error
	IsRefreshFamilyReplayBlocked(ctx context.Context, familyID string) (bool, error)
}

type redisRuntime struct {
	client    redis.UniversalClient
	keyPrefix string
}

func NewRedisRuntime(client redis.UniversalClient, keyPrefix string) RedisRuntime {
	return &redisRuntime{
		client:    client,
		keyPrefix: strings.Trim(strings.TrimSpace(keyPrefix), ":"),
	}
}

func (r *redisRuntime) EmailCooldownActive(ctx context.Context, email string) (bool, error) {
	key := r.emailCooldownKey(email)
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check email cooldown: %w", err)
	}
	return count > 0, nil
}

func (r *redisRuntime) StoreEmailCode(ctx context.Context, email string, record emailCode, codeTTL time.Duration, cooldownTTL time.Duration) error {
	allowedScenes := knownEmailCodeScenes
	if strings.TrimSpace(record.Scene) != "" {
		allowedScenes = append([]string{record.Scene}, knownEmailCodeScenes...)
	}
	seen := make(map[string]struct{}, len(allowedScenes))
	pipe := r.client.TxPipeline()
	for _, scene := range allowedScenes {
		scene = strings.TrimSpace(scene)
		if scene == "" {
			continue
		}
		if _, exists := seen[scene]; exists {
			continue
		}
		seen[scene] = struct{}{}
		pipe.Del(ctx, r.emailCodeKey(scene, email))
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal email code: %w", err)
	}
	pipe.Set(ctx, r.emailCodeKey(record.Scene, email), payload, codeTTL)
	pipe.Set(ctx, r.emailCooldownKey(email), "1", cooldownTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store email code: %w", err)
	}
	return nil
}

func (r *redisRuntime) LoadEmailCode(ctx context.Context, email string, allowedScenes []string) (emailCode, bool, error) {
	scenes := normalizeAllowedScenes(allowedScenes)
	for _, scene := range scenes {
		payload, err := r.client.Get(ctx, r.emailCodeKey(scene, email)).Bytes()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return emailCode{}, false, fmt.Errorf("load email code: %w", err)
		}
		var record emailCode
		if err := json.Unmarshal(payload, &record); err != nil {
			return emailCode{}, false, fmt.Errorf("unmarshal email code: %w", err)
		}
		return record, true, nil
	}
	return emailCode{}, false, nil
}

func (r *redisRuntime) DeleteEmailCodes(ctx context.Context, email string, allowedScenes []string) error {
	keys := make([]string, 0, len(allowedScenes))
	for _, scene := range normalizeAllowedScenes(allowedScenes) {
		keys = append(keys, r.emailCodeKey(scene, email))
	}
	if len(keys) == 0 {
		return nil
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete email code: %w", err)
	}
	return nil
}

func (r *redisRuntime) StoreRefreshTokenState(ctx context.Context, tokenHash string, session refreshSession, ttl time.Duration) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal refresh session: %w", err)
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := r.client.Set(ctx, r.refreshTokenKey(tokenHash), payload, ttl).Err(); err != nil {
		return fmt.Errorf("store refresh session: %w", err)
	}
	return nil
}

func (r *redisRuntime) LoadRefreshTokenState(ctx context.Context, tokenHash string) (refreshSession, bool, error) {
	payload, err := r.client.Get(ctx, r.refreshTokenKey(tokenHash)).Bytes()
	if err == redis.Nil {
		return refreshSession{}, false, nil
	}
	if err != nil {
		return refreshSession{}, false, fmt.Errorf("load refresh session: %w", err)
	}
	var session refreshSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return refreshSession{}, false, fmt.Errorf("unmarshal refresh session: %w", err)
	}
	return session, true, nil
}

func (r *redisRuntime) MarkRefreshFamilyReplayBlocked(ctx context.Context, familyID string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := r.client.Set(ctx, r.refreshFamilyBlockedKey(familyID), "1", ttl).Err(); err != nil {
		return fmt.Errorf("mark refresh family replay blocked: %w", err)
	}
	return nil
}

func (r *redisRuntime) IsRefreshFamilyReplayBlocked(ctx context.Context, familyID string) (bool, error) {
	count, err := r.client.Exists(ctx, r.refreshFamilyBlockedKey(familyID)).Result()
	if err != nil {
		return false, fmt.Errorf("check refresh family replay block: %w", err)
	}
	return count > 0, nil
}

func (r *redisRuntime) emailCodeKey(scene string, email string) string {
	return r.key("auth", "email_code", strings.ToLower(strings.TrimSpace(scene)), strings.ToLower(strings.TrimSpace(email)))
}

func (r *redisRuntime) emailCooldownKey(email string) string {
	return r.key("auth", "email_cooldown", strings.ToLower(strings.TrimSpace(email)))
}

func (r *redisRuntime) refreshTokenKey(tokenHash string) string {
	return r.key("auth", "refresh_hash", tokenHash)
}

func (r *redisRuntime) refreshFamilyBlockedKey(familyID string) string {
	return r.key("auth", "refresh_family_blocked", familyID)
}

func (r *redisRuntime) key(parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	if r.keyPrefix != "" {
		segments = append(segments, r.keyPrefix)
	}
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), ":")
		if part != "" {
			segments = append(segments, part)
		}
	}
	return strings.Join(segments, ":")
}

func normalizeAllowedScenes(allowedScenes []string) []string {
	if len(allowedScenes) == 0 {
		return append([]string{}, knownEmailCodeScenes...)
	}
	seen := make(map[string]struct{}, len(allowedScenes))
	scenes := make([]string, 0, len(allowedScenes))
	for _, scene := range allowedScenes {
		scene = strings.ToLower(strings.TrimSpace(scene))
		if scene == "" {
			continue
		}
		if _, exists := seen[scene]; exists {
			continue
		}
		seen[scene] = struct{}{}
		scenes = append(scenes, scene)
	}
	if len(scenes) == 0 {
		return append([]string{}, knownEmailCodeScenes...)
	}
	return scenes
}
