package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type openAIWSStateOwner struct {
	GroupID          int64
	UserID           int64
	APIKeyID         int64
	AccountID        int64
	SessionScopeHash string
}

type openAIWSStateOwnerContextKey struct{}

var (
	openAIWSStateOwnerKey   = openAIWSStateOwnerContextKey{}
	openAIWSIngressScopeSeq atomic.Uint64
	openAIWSLogHMACKey      = mustNewOpenAIWSLogHMACKey()
)

func (o openAIWSStateOwner) tenantKnown() bool { return o.APIKeyID > 0 }

func (o openAIWSStateOwner) normalized() openAIWSStateOwner {
	o.SessionScopeHash = strings.TrimSpace(o.SessionScopeHash)
	return o
}

func (o openAIWSStateOwner) tenantKey() string {
	o = o.normalized()
	if !o.tenantKnown() {
		return ""
	}
	seed := fmt.Sprintf("oas-owner-v1:%d:%d:%d", o.GroupID, o.UserID, o.APIKeyID)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func openAIWSStateOwnerFromContext(ctx context.Context) (openAIWSStateOwner, bool) {
	if ctx == nil {
		return openAIWSStateOwner{}, false
	}
	owner, ok := ctx.Value(openAIWSStateOwnerKey).(openAIWSStateOwner)
	if !ok || !owner.tenantKnown() {
		return openAIWSStateOwner{}, false
	}
	return owner.normalized(), true
}

// WithOpenAIWSRequestOwner marks an authenticated OpenAI request so legacy,
// ownerless response bindings are never used on the request path.
func WithOpenAIWSRequestOwner(ctx context.Context, c *gin.Context, sessionScopeHash ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return ctx
	}
	value, _ := c.Get("api_key")
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return ctx
	}
	owner := openAIWSStateOwner{
		GroupID:  getOpenAIGroupIDFromContext(c),
		UserID:   apiKey.UserID,
		APIKeyID: apiKey.ID,
	}
	if len(sessionScopeHash) > 0 {
		owner.SessionScopeHash = strings.TrimSpace(sessionScopeHash[0])
	}
	return context.WithValue(ctx, openAIWSStateOwnerKey, owner)
}

func openAIWSStateOwnerForRequest(ctx context.Context, c *gin.Context, accountID int64, sessionScopeHash string) (openAIWSStateOwner, bool) {
	if owner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		owner.AccountID = accountID
		if strings.TrimSpace(sessionScopeHash) != "" {
			owner.SessionScopeHash = strings.TrimSpace(sessionScopeHash)
		}
		return owner.normalized(), true
	}
	if c == nil {
		return openAIWSStateOwner{}, false
	}
	ownerCtx := WithOpenAIWSRequestOwner(ctx, c, sessionScopeHash)
	owner, ok := openAIWSStateOwnerFromContext(ownerCtx)
	if !ok {
		return openAIWSStateOwner{}, false
	}
	owner.AccountID = accountID
	owner.SessionScopeHash = strings.TrimSpace(sessionScopeHash)
	return owner.normalized(), true
}

func deriveOpenAISessionHashesForContext(c *gin.Context, sessionID string) (currentHash string, legacyHash string) {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" {
		return "", ""
	}
	value, _ := c.Get("api_key")
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return deriveOpenAISessionHashes(normalized)
	}
	groupID := getOpenAIGroupIDFromContext(c)
	seed := fmt.Sprintf("oas-session-v2:%d:%d:%d:%s", groupID, apiKey.UserID, apiKey.ID, normalized)
	sum := sha256.Sum256([]byte(seed))
	currentHash = "oas-session-v2:" + hex.EncodeToString(sum[:])
	_, legacyHash = deriveOpenAISessionHashes(normalized)
	return currentHash, legacyHash
}

func openAIWSSessionScopeForIngress(sessionHash string) string {
	if normalized := strings.TrimSpace(sessionHash); normalized != "" {
		return normalized
	}
	seq := openAIWSIngressScopeSeq.Add(1)
	seed := fmt.Sprintf("oas-conn-scope-v1:%d:%d", time.Now().UnixNano(), seq)
	sum := sha256.Sum256([]byte(seed))
	return "oas-conn-scope:" + hex.EncodeToString(sum[:])
}

func mustNewOpenAIWSLogHMACKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("initialize OpenAI WS log HMAC key: %v", err))
	}
	return key
}

func openAIWSSensitiveIDDigest(domain, value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	mac := hmac.New(sha256.New, openAIWSLogHMACKey)
	_, _ = mac.Write([]byte("openai-ws-log-v1:" + strings.TrimSpace(domain) + ":"))
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil)[:6])
}

func openAIWSStateIDDigest(value string) string {
	return openAIWSSensitiveIDDigest("response", value)
}

func bindOpenAIWSResponseAccount(ctx context.Context, store OpenAIWSStateStore, groupID int64, responseID string, accountID int64, ttl time.Duration) error {
	if store == nil {
		return nil
	}
	TrackOpenAIResponseIDFromContext(ctx, responseID)
	if owner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		return store.BindResponseAccountOwned(ctx, groupID, responseID, owner, accountID, ttl)
	}
	return store.BindResponseAccount(ctx, groupID, responseID, accountID, ttl)
}

func getOpenAIWSResponseAccount(ctx context.Context, store OpenAIWSStateStore, groupID int64, responseID string) (int64, error) {
	if store == nil {
		return 0, nil
	}
	if owner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		return store.GetResponseAccountOwned(ctx, groupID, responseID, owner)
	}
	return store.GetResponseAccount(ctx, groupID, responseID)
}

func deleteOpenAIWSResponseAccount(ctx context.Context, store OpenAIWSStateStore, groupID int64, responseID string) error {
	if store == nil {
		return nil
	}
	if owner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		return store.DeleteResponseAccountOwned(ctx, groupID, responseID, owner)
	}
	return store.DeleteResponseAccount(ctx, groupID, responseID)
}

func bindOpenAIWSResponseConn(ctx context.Context, store OpenAIWSStateStore, responseID string, owner openAIWSStateOwner, connID string, ttl time.Duration) {
	if store == nil {
		return
	}
	TrackOpenAIResponseIDFromContext(ctx, responseID)
	TrackOpenAIResponseConnIDFromContext(ctx, connID)
	if owner.tenantKnown() {
		store.BindResponseConnOwned(responseID, owner, connID, ttl)
		return
	}
	if contextOwner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		contextOwner.AccountID = owner.AccountID
		contextOwner.SessionScopeHash = owner.SessionScopeHash
		store.BindResponseConnOwned(responseID, contextOwner, connID, ttl)
		return
	}
	store.BindResponseConn(responseID, connID, ttl)
}

func getOpenAIWSResponseConn(ctx context.Context, store OpenAIWSStateStore, responseID string, owner openAIWSStateOwner) (string, bool) {
	if store == nil {
		return "", false
	}
	if owner.tenantKnown() {
		return store.GetResponseConnOwned(responseID, owner)
	}
	if contextOwner, ok := openAIWSStateOwnerFromContext(ctx); ok {
		contextOwner.AccountID = owner.AccountID
		contextOwner.SessionScopeHash = owner.SessionScopeHash
		return store.GetResponseConnOwned(responseID, contextOwner)
	}
	return store.GetResponseConn(responseID)
}
