package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIAffinityProvider        = "openai"
	openAIAffinityDomain          = "openai-affinity-v1"
	openAIAffinityLogDomain       = "openai-affinity-log-v1"
	openAIAffinityContextKey      = "openai_affinity_identity_v2"
	openAIAffinityMinSecretBytes  = 32
	openAIAffinityMaxAliases      = 12
	openAIAffinityMaxSignalBytes  = 4096
	openAIAffinityDefaultResponse = 72 * time.Hour
	openAIAffinityDefaultStrong   = 24 * time.Hour
	openAIAffinityDefaultExplicit = 12 * time.Hour
	openAIAffinityDefaultWeak     = 30 * time.Minute
	openAIAffinityDefaultRefresh  = 5 * time.Minute
)

var (
	ErrOpenAIAffinityConfiguration = errors.New("openai affinity configuration invalid")
	ErrOpenAIAffinityStateUnbound  = errors.New("openai state has no durable affinity binding")
)

type AffinityStrength uint8

const (
	AffinityWeak AffinityStrength = iota
	AffinityExplicit
	AffinityStrong
)

type SessionIdentity struct {
	OwnerScopeHash       string
	NamespaceHash        string
	PrimaryHash          string
	Aliases              []string
	PreviousResponseHash string
	Source               string
	Strength             AffinityStrength
	Stateful             bool
	ReplaySafe           bool
	Provider             string
	Capability           string
}

type openAIAffinityContextValue struct {
	Identity SessionIdentity
	Enabled  bool
	Writable bool
	Err      error
}

func (s *OpenAIGatewayService) openAIAffinityEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIAffinity.Enabled
}

func (s *OpenAIGatewayService) openAIAffinityWritesEnabled() bool {
	return s.openAIAffinityEnabled() && s.cfg.Gateway.OpenAIAffinity.WritesEnabled
}

func (s *OpenAIGatewayService) openAIAffinitySecret() ([]byte, error) {
	if !s.openAIAffinityEnabled() {
		return nil, nil
	}
	secret := []byte(strings.TrimSpace(s.cfg.Gateway.OpenAIAffinity.Secret))
	if len(secret) < openAIAffinityMinSecretBytes {
		return nil, fmt.Errorf("%w: gateway.openai_affinity.secret must contain at least %d bytes", ErrOpenAIAffinityConfiguration, openAIAffinityMinSecretBytes)
	}
	return secret, nil
}

func openAIAffinityDigest(secret []byte, domain string, parts ...string) string {
	mac := hmac.New(sha256.New, secret)
	writePart := func(value string) {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(value)))
		_, _ = mac.Write(size[:])
		_, _ = mac.Write([]byte(value))
	}
	writePart(domain)
	for _, part := range parts {
		writePart(part)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func openAIAffinityCapability(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return "responses"
	}
	path := strings.ToLower(c.Request.URL.Path)
	switch {
	case strings.Contains(path, "chat/completions"):
		return "responses"
	case strings.Contains(path, "messages"):
		return "responses"
	case strings.Contains(path, "compact"):
		return "compact"
	case strings.Contains(path, "websocket"), strings.Contains(path, "/ws"):
		return "responses"
	default:
		return "responses"
	}
}

func openAIAffinityOwnerSeed(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, _ := c.Get("api_key")
	apiKey, _ := value.(*APIKey)
	if apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 || apiKey.GroupID == nil || *apiKey.GroupID <= 0 {
		return ""
	}
	return fmt.Sprintf("group:%d|user:%d|api_key:%d", *apiKey.GroupID, apiKey.UserID, apiKey.ID)
}

func boundedOpenAIAffinitySignal(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > openAIAffinityMaxSignalBytes {
		return value[:openAIAffinityMaxSignalBytes]
	}
	return value
}

func collectOpenAIExplicitSessionSignals(c *gin.Context) []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(explicitOpenAIHeaderSessionNames))
	for _, name := range explicitOpenAIHeaderSessionNames {
		value := boundedOpenAIAffinitySignal(c.GetHeader(name))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *OpenAIGatewayService) resolveOpenAISessionIdentity(c *gin.Context, body []byte, legacySessionHash string) (SessionIdentity, error) {
	identity := SessionIdentity{Provider: openAIAffinityProvider, Capability: openAIAffinityCapability(c), ReplaySafe: true}
	if !s.openAIAffinityEnabled() {
		return identity, nil
	}
	secret, err := s.openAIAffinitySecret()
	if err != nil {
		return identity, err
	}
	owner := openAIAffinityOwnerSeed(c)
	if owner == "" {
		return identity, fmt.Errorf("%w: authenticated owner is unavailable", ErrOpenAIAffinityConfiguration)
	}
	group := fmt.Sprintf("group:%d", getOpenAIGroupIDFromContext(c))
	identity.OwnerScopeHash = openAIAffinityDigest(secret, openAIAffinityDomain, "owner", owner)
	identity.NamespaceHash = openAIAffinityDigest(secret, openAIAffinityDomain, "namespace", owner, group, identity.Provider, identity.Capability)

	validJSON := len(body) == 0 || gjson.ValidBytes(body)
	previous := ""
	promptCache := ""
	if len(body) > 0 && validJSON {
		previous = boundedOpenAIAffinitySignal(gjson.GetBytes(body, "previous_response_id").String())
		promptCache = boundedOpenAIAffinitySignal(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if previous != "" {
		identity.PreviousResponseHash = openAIAffinityDigest(secret, openAIAffinityDomain, "response", owner, identity.Provider, previous)
		identity.Source = "previous_response_id"
		identity.Strength = AffinityStrong
		identity.Stateful = true
		identity.ReplaySafe = false
	}

	explicit := collectOpenAIExplicitSessionSignals(c)
	type rawAlias struct{ kind, value string }
	aliases := make([]rawAlias, 0, len(explicit)+2)
	for _, value := range explicit {
		aliases = append(aliases, rawAlias{kind: "explicit", value: value})
	}
	if promptCache != "" {
		aliases = append(aliases, rawAlias{kind: "prompt_cache", value: promptCache})
	}
	if legacySessionHash != "" {
		aliases = append(aliases, rawAlias{kind: "legacy_hash", value: legacySessionHash})
	}

	if identity.Source == "" {
		switch {
		case len(explicit) > 0:
			identity.Source = "explicit_session"
			identity.Strength = AffinityExplicit
			identity.PrimaryHash = openAIAffinityDigest(secret, openAIAffinityDomain, "session", owner, group, identity.Provider, identity.Capability, "explicit", explicit[0])
		case promptCache != "":
			identity.Source = "prompt_cache_key"
			identity.Strength = AffinityWeak
			identity.PrimaryHash = openAIAffinityDigest(secret, openAIAffinityDomain, "session", owner, group, identity.Provider, identity.Capability, "prompt_cache", promptCache)
		case legacySessionHash != "":
			identity.Source = "derived_session"
			identity.Strength = AffinityWeak
			identity.PrimaryHash = openAIAffinityDigest(secret, openAIAffinityDomain, "session", owner, group, identity.Provider, identity.Capability, "legacy_hash", legacySessionHash)
		}
	}

	lower := strings.ToLower(string(body))
	if !validJSON || strings.Contains(lower, "function_call_output") || strings.Contains(lower, "encrypted_content") || strings.Contains(lower, "encrypted_reasoning") {
		identity.Stateful = true
		identity.ReplaySafe = false
		if identity.Strength < AffinityStrong {
			identity.Strength = AffinityStrong
		}
		if identity.Source == "" {
			identity.Source = "unresolved_state"
		}
	}

	seenAlias := make(map[string]struct{})
	for _, alias := range aliases {
		digest := openAIAffinityDigest(secret, openAIAffinityDomain, "alias", owner, group, identity.Provider, identity.Capability, alias.kind, alias.value)
		if digest == identity.PrimaryHash {
			continue
		}
		if _, ok := seenAlias[digest]; ok {
			continue
		}
		seenAlias[digest] = struct{}{}
		identity.Aliases = append(identity.Aliases, digest)
		if len(identity.Aliases) == openAIAffinityMaxAliases {
			break
		}
	}
	sort.Strings(identity.Aliases)
	return identity, nil
}

func attachOpenAIAffinityIdentity(c *gin.Context, identity SessionIdentity, enabled, writable bool) {
	if c == nil {
		return
	}
	value := openAIAffinityContextValue{Identity: identity, Enabled: enabled, Writable: writable}
	c.Set(openAIAffinityContextKey, value)
	if c.Request != nil {
		ctx := context.WithValue(c.Request.Context(), openAIAffinityContextKey, value)
		c.Request = c.Request.WithContext(ctx)
	}
}

func openAIAffinityFromContext(ctx context.Context) (openAIAffinityContextValue, bool) {
	if ctx == nil {
		return openAIAffinityContextValue{}, false
	}
	value, ok := ctx.Value(openAIAffinityContextKey).(openAIAffinityContextValue)
	return value, ok && value.Enabled
}

func openAIAffinityFromGin(c *gin.Context) (openAIAffinityContextValue, bool) {
	if c == nil {
		return openAIAffinityContextValue{}, false
	}
	if raw, ok := c.Get(openAIAffinityContextKey); ok {
		value, valid := raw.(openAIAffinityContextValue)
		return value, valid && value.Enabled
	}
	if c.Request != nil {
		return openAIAffinityFromContext(c.Request.Context())
	}
	return openAIAffinityContextValue{}, false
}

func (s *OpenAIGatewayService) prepareOpenAIAffinityIdentity(c *gin.Context, body []byte, legacySessionHash string) error {
	if !s.openAIAffinityEnabled() {
		return nil
	}
	identity, err := s.resolveOpenAISessionIdentity(c, body, legacySessionHash)
	attachOpenAIAffinityIdentity(c, identity, true, s.openAIAffinityWritesEnabled())
	if err != nil {
		value, _ := openAIAffinityFromGin(c)
		value.Err = err
		attachOpenAIAffinityIdentity(c, value.Identity, true, s.openAIAffinityWritesEnabled())
		if raw, ok := c.Get(openAIAffinityContextKey); ok {
			v := raw.(openAIAffinityContextValue)
			v.Err = err
			c.Set(openAIAffinityContextKey, v)
			if c.Request != nil {
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), openAIAffinityContextKey, v))
			}
		}
	}
	return err
}

func openAIAffinityLogDigest(secret []byte, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return openAIAffinityDigest(secret, openAIAffinityLogDomain, value)[:16]
}
