package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	CodexIdentityPolicyV1 = "v1"
	CodexIdentityPolicyV2 = "v2"
)

const (
	codexNamespaceConversation    = "codex/conversation/v2"
	codexNamespaceIdentityRoot    = "codex/identity-root/v2"
	codexNamespaceDevice          = "codex/device/v2"
	codexNamespaceSession         = "codex/session/v2"
	codexNamespaceThread          = "codex/thread/v2"
	codexNamespaceWindow          = "codex/window/v2"
	codexNamespaceClientRequest   = "codex/client-request/v2"
	codexNamespaceInternalAttempt = "codex/internal-attempt/v2"
	codexNamespaceTransport       = "codex/transport/v2"
)

type CodexOperationKind string

const (
	CodexOperationResponses CodexOperationKind = "responses"
	CodexOperationCompact   CodexOperationKind = "compact"
	CodexOperationResume    CodexOperationKind = "resume"
)

type CodexEgressTransport string

const (
	CodexTransportHTTP CodexEgressTransport = "http"
	CodexTransportWS   CodexEgressTransport = "websocket"
)

type CodexIdentityDeriver struct {
	secret  []byte
	macs    sync.Pool
	buffers sync.Pool
}

func NewCodexIdentityDeriver(secret string) (*CodexIdentityDeriver, error) {
	secret = strings.TrimSpace(secret)
	if len([]byte(secret)) < openAIAffinityMinSecretBytes {
		return nil, fmt.Errorf("codex identity derivation secret must contain at least %d bytes", openAIAffinityMinSecretBytes)
	}
	deriver := &CodexIdentityDeriver{secret: []byte(secret)}
	deriver.macs.New = func() any { return hmac.New(sha256.New, deriver.secret) }
	deriver.buffers.New = func() any { return new([512]byte) }
	return deriver, nil
}

// ResolveCodexIdentityDerivationSecret returns the HMAC secret used by Relay Kernel.
// gateway.openai_affinity.secret is preferred; jwt.secret is a stable fallback so
// dual-instance deployments that already share JWT can enable Relay Kernel without
// a second independently configured secret.
func ResolveCodexIdentityDerivationSecret(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	secret := strings.TrimSpace(cfg.Gateway.OpenAIAffinity.Secret)
	if len([]byte(secret)) >= openAIAffinityMinSecretBytes {
		return secret
	}
	fallback := strings.TrimSpace(cfg.JWT.Secret)
	if len([]byte(fallback)) >= openAIAffinityMinSecretBytes {
		return fallback
	}
	return secret
}

func (d *CodexIdentityDeriver) digest(namespace string, parts ...string) [sha256.Size]byte {
	encodedSize := 4 + len(namespace)
	for _, part := range parts {
		encodedSize += 4 + len(part)
	}
	buffer := d.buffers.Get().(*[512]byte)
	encoded := buffer[:0]
	if encodedSize > len(buffer) {
		encoded = make([]byte, 0, encodedSize)
	}
	encoded = appendCodexDerivationPart(encoded, namespace)
	for _, part := range parts {
		encoded = appendCodexDerivationPart(encoded, part)
	}

	mac := d.macs.Get().(hash.Hash)
	mac.Reset()
	_, _ = mac.Write(encoded)
	d.buffers.Put(buffer)
	var out [sha256.Size]byte
	_ = mac.Sum(out[:0])
	d.macs.Put(mac)
	return out
}

func appendCodexDerivationPart(dst []byte, value string) []byte {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	dst = append(dst, size[:]...)
	return append(dst, value...)
}

func (d *CodexIdentityDeriver) DigestHex(namespace string, parts ...string) string {
	digest := d.digest(namespace, parts...)
	return hex.EncodeToString(digest[:])
}

func (d *CodexIdentityDeriver) UUIDv4(namespace string, parts ...string) string {
	digest := d.digest(namespace, parts...)
	return formatCodexDerivedUUID(digest[:16], 4)
}

func (d *CodexIdentityDeriver) UUIDv7(namespace string, at time.Time, parts ...string) string {
	digest := d.digest(namespace, parts...)
	var raw [16]byte
	millis := uint64(at.UTC().UnixMilli()) & 0x0000FFFFFFFFFFFF
	raw[0] = byte(millis >> 40)
	raw[1] = byte(millis >> 32)
	raw[2] = byte(millis >> 24)
	raw[3] = byte(millis >> 16)
	raw[4] = byte(millis >> 8)
	raw[5] = byte(millis)
	copy(raw[6:], digest[:10])
	return formatCodexDerivedUUID(raw[:], 7)
}

func formatCodexDerivedUUID(source []byte, version byte) string {
	var raw [16]byte
	copy(raw[:], source)
	raw[6] = (raw[6] & 0x0f) | (version << 4)
	raw[8] = (raw[8] & 0x3f) | 0x80

	var formatted [36]byte
	hex.Encode(formatted[0:8], raw[0:4])
	formatted[8] = '-'
	hex.Encode(formatted[9:13], raw[4:6])
	formatted[13] = '-'
	hex.Encode(formatted[14:18], raw[6:8])
	formatted[18] = '-'
	hex.Encode(formatted[19:23], raw[8:10])
	formatted[23] = '-'
	hex.Encode(formatted[24:36], raw[10:16])
	return string(formatted[:])
}

type CodexRequestPlanInput struct {
	LogicalRequestID   string
	SessionHash        string
	PreviousResponseID string
	PromptCacheKey     string
	RequestedModel     string
	Operation          CodexOperationKind
	Transport          CodexEgressTransport
	InboundHeaders     http.Header
	Body               []byte
	CreatedAt          time.Time
	DerivationSecret   string
}

type CodexRequestPlan struct {
	requireExistingConversation bool
	logicalRequestID            string
	conversationDigest          string
	clientRequestID             string
	previousResponseID          string
	promptCacheKey              string
	requestedModel              string
	operation                   CodexOperationKind
	transport                   CodexEgressTransport
	inboundHeaders              http.Header
	body                        []byte
	createdAt                   time.Time
}

func NewCodexRequestPlan(input CodexRequestPlanInput) (*CodexRequestPlan, error) {
	logicalRequestID := strings.TrimSpace(input.LogicalRequestID)
	if logicalRequestID == "" {
		return nil, errors.New("codex logical request id is required")
	}
	operation := input.Operation
	if operation == "" {
		operation = CodexOperationResponses
	}
	transport := input.Transport
	if transport == "" {
		transport = CodexTransportHTTP
	}
	if transport != CodexTransportHTTP && transport != CodexTransportWS {
		return nil, fmt.Errorf("unsupported codex transport %q", transport)
	}
	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	conversationInput := firstNonEmptyCodexString(input.PreviousResponseID, input.PromptCacheKey, input.SessionHash, logicalRequestID)
	conversationDigest := fallbackCodexConversationDigest(conversationInput)
	clientRequestID := ""
	if strings.TrimSpace(input.DerivationSecret) != "" {
		deriver, err := NewCodexIdentityDeriver(input.DerivationSecret)
		if err != nil {
			return nil, err
		}
		conversationDigest = deriver.DigestHex(codexNamespaceConversation, conversationInput)
		clientRequestID = deriver.UUIDv7(codexNamespaceClientRequest, createdAt, logicalRequestID, conversationDigest)
	}

	return &CodexRequestPlan{
		logicalRequestID:   logicalRequestID,
		conversationDigest: conversationDigest,
		clientRequestID:    clientRequestID,
		previousResponseID: strings.TrimSpace(input.PreviousResponseID),
		promptCacheKey:     strings.TrimSpace(input.PromptCacheKey),
		requestedModel:     strings.TrimSpace(input.RequestedModel),
		operation:          operation,
		transport:          transport,
		inboundHeaders:     cloneCodexHTTPHeader(input.InboundHeaders),
		body:               append([]byte(nil), input.Body...),
		createdAt:          createdAt,
	}, nil
}

func fallbackCodexConversationDigest(value string) string {
	digest := sha256.Sum256([]byte("codex/conversation/fallback/v1\x00" + value))
	return hex.EncodeToString(digest[:])
}

func firstNonEmptyCodexString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func cloneCodexHTTPHeader(source http.Header) http.Header {
	if source == nil {
		return nil
	}
	cloned := make(http.Header, len(source))
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func (p *CodexRequestPlan) LogicalRequestID() string {
	if p == nil {
		return ""
	}
	return p.logicalRequestID
}

func (p *CodexRequestPlan) ConversationDigest() string {
	if p == nil {
		return ""
	}
	return p.conversationDigest
}

func (p *CodexRequestPlan) Operation() CodexOperationKind {
	if p == nil {
		return ""
	}
	return p.operation
}

func (p *CodexRequestPlan) Transport() CodexEgressTransport {
	if p == nil {
		return ""
	}
	return p.transport
}

func (p *CodexRequestPlan) Body() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.body...)
}

func (p *CodexRequestPlan) InboundHeaders() http.Header {
	if p == nil {
		return nil
	}
	return cloneCodexHTTPHeader(p.inboundHeaders)
}

type CodexAttemptInput struct {
	ProfileSnapshot        *CodexClientProfile
	InstallationPolicy     string
	AccountID              int64
	AccountVersion         string
	CredentialVersion      string
	ProxyIdentity          string
	ProfileID              string
	FingerprintMode        string
	AttemptNumber          int
	EgressRoute            string
	TransportConfigVersion string
}

type CodexOrderedHeader struct {
	Name  string
	Value string
}

type CodexAttemptState struct {
	policyVersion       string
	attemptNumber       int
	accountID           int64
	poolSlot            int
	clientRequestID     string
	internalAttemptID   string
	transportKey        string
	profile             CodexClientProfile
	identity            *CodexIdentitySnapshot
	finalHeaders        []CodexOrderedHeader
	finalHTTPBody       []byte
	finalWSPayload      []byte
	installationPolicy  string
	deriver             *CodexIdentityDeriver
	conversationBinding *CodexConversationState
}

func FinalizeCodexAttempt(plan *CodexRequestPlan, input CodexAttemptInput, derivationSecret string) (*CodexAttemptState, error) {
	if plan == nil {
		return nil, errors.New("codex request plan is required")
	}
	if input.AccountID <= 0 {
		return nil, errors.New("codex attempt account id is required")
	}
	if input.AttemptNumber < 0 {
		return nil, errors.New("codex attempt number cannot be negative")
	}
	deriver, err := NewCodexIdentityDeriver(derivationSecret)
	if err != nil {
		return nil, err
	}
	return finalizeCodexAttemptWithDeriver(plan, input, deriver)
}

func finalizeCodexAttemptWithDeriver(plan *CodexRequestPlan, input CodexAttemptInput, deriver *CodexIdentityDeriver) (*CodexAttemptState, error) {
	if plan == nil || deriver == nil {
		return nil, errors.New("codex request plan and deriver are required")
	}
	profile, err := resolveCodexAttemptProfile(input, plan.inboundHeaders)
	if err != nil {
		return nil, err
	}
	if plan.transport == CodexTransportWS && !profile.Supports(CodexCapabilityWebSocket) {
		return nil, fmt.Errorf("codex profile %s does not support WebSocket egress", profile.ID)
	}
	if plan.operation == CodexOperationCompact && !profile.Supports(CodexCapabilityCompact) {
		return nil, fmt.Errorf("codex profile %s does not support Compact", profile.ID)
	}
	if plan.operation == CodexOperationResume && !profile.Supports(CodexCapabilityResume) {
		return nil, fmt.Errorf("codex profile %s does not support resume", profile.ID)
	}

	accountScope := strconv.FormatInt(input.AccountID, 10) + ":" + strings.TrimSpace(input.AccountVersion)
	credentialScope := strings.TrimSpace(input.CredentialVersion)
	deviceID := deriver.UUIDv4(codexNamespaceDevice, accountScope, credentialScope, profile.ID, strconv.Itoa(profile.Revision))
	installationPolicy, err := normalizeCodexInstallationPolicy(input.InstallationPolicy)
	if err != nil {
		return nil, err
	}
	if installationPolicy == CodexInstallationStableV1 {
		deviceID = codexStableInstallationID(deriver, input.AccountID, profile.ID)
	}
	mode := normalizeCodexFingerprintMode(input.FingerprintMode)
	clientRequestID := plan.clientRequestID
	if clientRequestID == "" {
		clientRequestID = deriver.UUIDv7(codexNamespaceClientRequest, plan.createdAt, plan.logicalRequestID, plan.conversationDigest)
	}
	identity := deriveCodexV2Identity(deriver, plan, mode, profile, deviceID, clientRequestID)
	poolSlot := codexDerivationSlot(deriver, plan.conversationDigest, codexFingerprintPoolSize(mode))
	internalAttemptID := deriver.UUIDv7(codexNamespaceInternalAttempt, plan.createdAt, plan.logicalRequestID, strconv.Itoa(input.AttemptNumber))
	transportKey := deriver.DigestHex(codexNamespaceTransport,
		accountScope,
		credentialScope,
		strings.TrimSpace(input.ProxyIdentity),
		profile.ID,
		strconv.Itoa(profile.Revision),
		profile.Transport.TLSProfileID,
		profile.Transport.HTTP2ProfileID,
		strings.TrimSpace(input.TransportConfigVersion),
		strings.TrimSpace(input.EgressRoute),
	)

	profileDigest, err := codexProfileSnapshotDigest(profile)
	if err != nil {
		return nil, err
	}
	transportKey = deriver.DigestHex("codex/transport/profile-snapshot/v1", transportKey, profileDigest, installationPolicy)
	finalBody, err := applyCodexFingerprintToRawBody(plan.body, identity)
	if err != nil {
		return nil, err
	}
	return &CodexAttemptState{
		policyVersion:      CodexIdentityPolicyV2,
		installationPolicy: installationPolicy,
		deriver:            deriver,
		attemptNumber:      input.AttemptNumber,
		accountID:          input.AccountID,
		poolSlot:           poolSlot,
		clientRequestID:    clientRequestID,
		internalAttemptID:  internalAttemptID,
		transportKey:       transportKey,
		profile:            profile,
		identity:           cloneCodexIdentitySnapshot(identity),
		finalHeaders:       buildCodexAttemptIdentityHeaders(profile, identity, plan.inboundHeaders),
		finalHTTPBody:      append([]byte(nil), finalBody...),
	}, nil
}

func normalizeCodexFingerprintMode(value string) codexFingerprintMode {
	mode := codexFingerprintMode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		return codexFingerprintSession
	}
	switch mode {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintWindow, codexFingerprintWindow40, codexFingerprintFull:
		return mode
	default:
		return codexFingerprintSession
	}
}

func codexFingerprintPoolSize(mode codexFingerprintMode) int {
	switch mode {
	case codexFingerprintWindow:
		return 24
	case codexFingerprintWindow40:
		return 40
	default:
		return 1
	}
}

func deriveCodexV2Identity(deriver *CodexIdentityDeriver, plan *CodexRequestPlan, mode codexFingerprintMode, profile CodexClientProfile, deviceID, clientRequestID string) *CodexIdentitySnapshot {
	if mode == codexFingerprintOff {
		return nil
	}
	conversation := plan.conversationDigest
	logicalRequest := plan.logicalRequestID
	poolSize := 1
	sessionScope := conversation
	threadScope := conversation
	switch mode {
	case codexFingerprintDevice:
		sessionScope = logicalRequest
		threadScope = logicalRequest
	case codexFingerprintSession:
		sessionScope = deviceID
	case codexFingerprintWindow:
		poolSize = 24
		sessionScope = strconv.Itoa(codexDerivationSlot(deriver, conversation, poolSize))
	case codexFingerprintWindow40:
		poolSize = 40
		sessionScope = strconv.Itoa(codexDerivationSlot(deriver, conversation, poolSize))
	case codexFingerprintFull:
		sessionScope = conversation
	}

	identityRoot := deriver.digest(codexNamespaceIdentityRoot, deviceID, string(mode), conversation)
	sessionDigest := codexIdentityChildDigest(identityRoot, codexNamespaceSession, sessionScope)
	sessionID := formatCodexDerivedUUID(sessionDigest[:16], 4)
	threadDigest := codexIdentityChildDigest(identityRoot, codexNamespaceThread, threadScope)
	threadID := formatCodexDerivedUUID(threadDigest[:16], 4)
	windowSlot := codexDerivationSlot(deriver, conversation, poolSize)
	windowDigest := codexIdentityChildDigest(identityRoot, codexNamespaceWindow, sessionID, strconv.Itoa(windowSlot))
	windowID := formatCodexDerivedUUID(windowDigest[:16], 4)
	turnID := clientRequestID
	return &CodexIdentitySnapshot{
		mode:                mode,
		installationID:      deviceID,
		sessionID:           sessionID,
		threadID:            threadID,
		windowID:            windowID,
		turnID:              turnID,
		turnStartedAtUnixMs: plan.createdAt.UnixMilli(),
		clientRequestID:     turnID,
		protocolProfile:     codexProtocolProfileName,
	}
}

func codexIdentityChildDigest(root [sha256.Size]byte, namespace string, parts ...string) [sha256.Size]byte {
	encodedSize := len(root) + 4 + len(namespace)
	for _, part := range parts {
		encodedSize += 4 + len(part)
	}
	var stack [512]byte
	encoded := stack[:0]
	if encodedSize > len(stack) {
		encoded = make([]byte, 0, encodedSize)
	}
	encoded = append(encoded, root[:]...)
	encoded = appendCodexDerivationPart(encoded, namespace)
	for _, part := range parts {
		encoded = appendCodexDerivationPart(encoded, part)
	}
	return sha256.Sum256(encoded)
}

func codexDerivationSlot(deriver *CodexIdentityDeriver, conversation string, size int) int {
	if size <= 1 {
		return 0
	}
	digest := deriver.digest(codexNamespaceWindow, conversation, strconv.Itoa(size))
	return int(binary.BigEndian.Uint64(digest[:8]) % uint64(size))
}

func cloneCodexIdentitySnapshot(source *CodexIdentitySnapshot) *CodexIdentitySnapshot {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}

func buildCodexAttemptIdentityHeaders(profile CodexClientProfile, identity *CodexIdentitySnapshot, inbound http.Header) []CodexOrderedHeader {
	values := cloneCodexHTTPHeader(inbound)
	if values == nil {
		values = make(http.Header)
	}
	if profile.ID != CodexProfilePassthrough {
		values.Set("User-Agent", profile.App.UserAgent)
		values.Set("originator", profile.App.Originator)
		if profile.App.Version != "" {
			values.Set("x-openai-client-version", profile.App.Version)
		}
		if profile.App.BetaFeatures != "" {
			values.Set("OpenAI-Beta", profile.App.BetaFeatures)
		}
	}
	if identity != nil {
		applyCodexFingerprintHeaders(values, identity)
	}
	ordered := make([]CodexOrderedHeader, 0, len(profile.Transport.HeaderOrder))
	for _, name := range profile.Transport.HeaderOrder {
		for _, value := range values.Values(name) {
			ordered = append(ordered, CodexOrderedHeader{Name: strings.ToLower(name), Value: value})
		}
	}
	return ordered
}

func (s *CodexIdentitySnapshot) InstallationID() string {
	if s == nil {
		return ""
	}
	return s.installationID
}

func (s *CodexIdentitySnapshot) SessionID() string {
	if s == nil {
		return ""
	}
	return s.sessionID
}

func (s *CodexIdentitySnapshot) ThreadID() string {
	if s == nil {
		return ""
	}
	return s.threadID
}

func (s *CodexIdentitySnapshot) WindowID() string {
	if s == nil {
		return ""
	}
	return s.windowID
}

func (s *CodexAttemptState) PoolSlot() int {
	if s == nil {
		return 0
	}
	return s.poolSlot
}

func (s *CodexIdentitySnapshot) TurnID() string {
	if s == nil {
		return ""
	}
	return s.turnID
}

func (s *CodexAttemptState) PolicyVersion() string {
	if s == nil {
		return ""
	}
	return s.policyVersion
}

func (s *CodexAttemptState) AttemptNumber() int {
	if s == nil {
		return 0
	}
	return s.attemptNumber
}

func (s *CodexAttemptState) AccountID() int64 {
	if s == nil {
		return 0
	}
	return s.accountID
}

func (s *CodexAttemptState) ClientRequestID() string {
	if s == nil {
		return ""
	}
	return s.clientRequestID
}

func (s *CodexAttemptState) InternalAttemptID() string {
	if s == nil {
		return ""
	}
	return s.internalAttemptID
}

func (s *CodexAttemptState) TransportKey() string {
	if s == nil {
		return ""
	}
	return s.transportKey
}

func (s *CodexAttemptState) Profile() CodexClientProfile {
	if s == nil {
		return CodexClientProfile{}
	}
	profile := s.profile
	profile.Transport.HeaderOrder = append([]string(nil), profile.Transport.HeaderOrder...)
	return profile
}

func (s *CodexAttemptState) Identity() *CodexIdentitySnapshot {
	if s == nil {
		return nil
	}
	return cloneCodexIdentitySnapshot(s.identity)
}

func (s *CodexAttemptState) FinalHeaders() []CodexOrderedHeader {
	if s == nil {
		return nil
	}
	return append([]CodexOrderedHeader(nil), s.finalHeaders...)
}

func (s *CodexAttemptState) FinalHTTPBody() []byte {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.finalHTTPBody...)
}

func (s *CodexAttemptState) FinalWSPayload() []byte {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.finalWSPayload...)
}

func (s *CodexAttemptState) WithFinalHTTPWire(headers []CodexOrderedHeader, body []byte) *CodexAttemptState {
	if s == nil {
		return nil
	}
	clone := *s
	clone.identity = cloneCodexIdentitySnapshot(s.identity)
	clone.profile = s.Profile()
	clone.finalHeaders = append([]CodexOrderedHeader(nil), headers...)
	clone.finalHTTPBody = append([]byte(nil), body...)
	return &clone
}

func (s *CodexAttemptState) WithFinalWSWire(headers []CodexOrderedHeader, payload []byte) *CodexAttemptState {
	if s == nil {
		return nil
	}
	clone := *s
	clone.identity = cloneCodexIdentitySnapshot(s.identity)
	clone.profile = s.Profile()
	clone.finalHeaders = append([]CodexOrderedHeader(nil), headers...)
	clone.finalWSPayload = append([]byte(nil), payload...)
	return &clone
}

type codexRequestPlanContextKey struct{}
type codexAttemptStateContextKey struct{}

func ContextWithCodexRequestPlan(ctx context.Context, plan *CodexRequestPlan) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, codexRequestPlanContextKey{}, plan)
}

func CodexRequestPlanFromContext(ctx context.Context) (*CodexRequestPlan, bool) {
	if ctx == nil {
		return nil, false
	}
	plan, ok := ctx.Value(codexRequestPlanContextKey{}).(*CodexRequestPlan)
	return plan, ok && plan != nil
}

func ContextWithCodexAttemptState(ctx context.Context, state *CodexAttemptState) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, codexAttemptStateContextKey{}, state)
}

func CodexAttemptStateFromContext(ctx context.Context) (*CodexAttemptState, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(codexAttemptStateContextKey{}).(*CodexAttemptState)
	return state, ok && state != nil
}
