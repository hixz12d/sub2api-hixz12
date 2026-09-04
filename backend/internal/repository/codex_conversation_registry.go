package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const codexConversationPrefix = "codex_conversation:v2:"

var resolveOrCreateCodexConversationScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then
  return {0, current}
end
local created = redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2], 'NX')
if created then
  return {1, ARGV[1]}
end
return {0, redis.call('GET', KEYS[1])}
`)

var compareAndSwapCodexConversationScript = redis.NewScript(`
local current_payload = redis.call('GET', KEYS[1])
if not current_payload then
  return {-1, ''}
end
local current = cjson.decode(current_payload)
if tonumber(current.revision) ~= tonumber(ARGV[1]) or tostring(current.account_id) ~= ARGV[2] then
  return {0, current_payload}
end
local next = cjson.decode(ARGV[3])
next.revision = tonumber(current.revision) + 1
next.created_at_unix_ms = current.created_at_unix_ms
local next_payload = cjson.encode(next)
redis.call('SET', KEYS[1], next_payload, 'PX', ARGV[4])
return {1, next_payload}
`)

var invalidateCodexConversationScript = redis.NewScript(`
local current_payload = redis.call('GET', KEYS[1])
if not current_payload then
  return 0
end
local current = cjson.decode(current_payload)
if tonumber(current.revision) ~= tonumber(ARGV[1]) or tostring(current.account_id) ~= ARGV[2] then
  return -1
end
return redis.call('DEL', KEYS[1])
`)

func codexConversationKey(conversationDigest string) (string, error) {
	conversationDigest = strings.ToLower(strings.TrimSpace(conversationDigest))
	decoded, err := hex.DecodeString(conversationDigest)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("codex conversation key must be a SHA-256/HMAC digest")
	}
	return codexConversationPrefix + conversationDigest, nil
}

func codexConversationTTLMillis(ttl time.Duration) int64 {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	if ttl < time.Minute {
		ttl = time.Minute
	}
	return ttl.Milliseconds()
}

func decodeCodexConversationState(payload string) (service.CodexConversationState, error) {
	if strings.TrimSpace(payload) == "" {
		return service.CodexConversationState{}, service.ErrCodexConversationNotFound
	}
	var state service.CodexConversationState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return service.CodexConversationState{}, fmt.Errorf("decode codex conversation state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return service.CodexConversationState{}, err
	}
	return state, nil
}

func (c *gatewayCache) ResolveOrCreateCodexConversation(
	ctx context.Context,
	conversationDigest string,
	candidate service.CodexConversationState,
	ttl time.Duration,
) (service.CodexConversationState, bool, error) {
	key, err := codexConversationKey(conversationDigest)
	if err != nil {
		return service.CodexConversationState{}, false, err
	}
	if err := candidate.Validate(); err != nil {
		return service.CodexConversationState{}, false, err
	}
	candidate.Revision = 1
	payload, err := json.Marshal(candidate)
	if err != nil {
		return service.CodexConversationState{}, false, err
	}
	values, err := resolveOrCreateCodexConversationScript.Run(ctx, c.rdb, []string{key}, string(payload), codexConversationTTLMillis(ttl)).Slice()
	if err != nil {
		return service.CodexConversationState{}, false, err
	}
	if len(values) != 2 {
		return service.CodexConversationState{}, false, errors.New("invalid codex conversation resolve result")
	}
	created := fmt.Sprint(values[0]) == "1"
	state, err := decodeCodexConversationState(fmt.Sprint(values[1]))
	return state, created, err
}

func (c *gatewayCache) GetCodexConversation(ctx context.Context, conversationDigest string) (service.CodexConversationState, error) {
	key, err := codexConversationKey(conversationDigest)
	if err != nil {
		return service.CodexConversationState{}, err
	}
	payload, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return service.CodexConversationState{}, service.ErrCodexConversationNotFound
	}
	if err != nil {
		return service.CodexConversationState{}, err
	}
	return decodeCodexConversationState(payload)
}

func (c *gatewayCache) CompareAndSwapCodexConversation(
	ctx context.Context,
	conversationDigest string,
	expectedRevision int64,
	expectedAccountID int64,
	next service.CodexConversationState,
	ttl time.Duration,
) (service.CodexConversationState, error) {
	key, err := codexConversationKey(conversationDigest)
	if err != nil {
		return service.CodexConversationState{}, err
	}
	if expectedRevision <= 0 || expectedAccountID <= 0 {
		return service.CodexConversationState{}, errors.New("codex conversation CAS expectation is invalid")
	}
	if err := next.Validate(); err != nil {
		return service.CodexConversationState{}, err
	}
	payload, err := json.Marshal(next)
	if err != nil {
		return service.CodexConversationState{}, err
	}
	values, err := compareAndSwapCodexConversationScript.Run(
		ctx,
		c.rdb,
		[]string{key},
		expectedRevision,
		expectedAccountID,
		string(payload),
		codexConversationTTLMillis(ttl),
	).Slice()
	if err != nil {
		return service.CodexConversationState{}, err
	}
	if len(values) != 2 {
		return service.CodexConversationState{}, errors.New("invalid codex conversation CAS result")
	}
	outcome := fmt.Sprint(values[0])
	if outcome == "-1" {
		return service.CodexConversationState{}, service.ErrCodexConversationNotFound
	}
	state, decodeErr := decodeCodexConversationState(fmt.Sprint(values[1]))
	if decodeErr != nil {
		return service.CodexConversationState{}, decodeErr
	}
	if outcome != "1" {
		return state, service.ErrCodexConversationCASConflict
	}
	return state, nil
}

func (c *gatewayCache) InvalidateCodexConversation(ctx context.Context, conversationDigest string, expectedRevision int64, expectedAccountID int64) (bool, error) {
	key, err := codexConversationKey(conversationDigest)
	if err != nil {
		return false, err
	}
	result, err := invalidateCodexConversationScript.Run(ctx, c.rdb, []string{key}, expectedRevision, expectedAccountID).Int64()
	if err != nil {
		return false, err
	}
	if result < 0 {
		return false, service.ErrCodexConversationCASConflict
	}
	return result == 1, nil
}

var _ service.CodexConversationRegistry = (*gatewayCache)(nil)
