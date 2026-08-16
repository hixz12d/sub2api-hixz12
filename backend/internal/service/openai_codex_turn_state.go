package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// openAICodexTurnStateHeader is an opaque turn-scoped token minted by the
// upstream. Codex echoes it on later requests in the same turn.
const openAICodexTurnStateHeader = "x-codex-turn-state"

type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

// openAICodexTurnStateSeed scopes provenance to one API key and the client's
// original session. Requests without a session remain pass-through.
func openAICodexTurnStateSeed(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	sessionID := extractClientSessionID(c.Request.Header)
	if sessionID == "" {
		return ""
	}
	return strconv.FormatInt(getAPIKeyIDFromContext(c), 10) + "\x00" + sessionID
}

// relayOpenAICodexTurnState commits the selected attempt's token downstream.
// A missing token clears any value left by an earlier failover attempt.
func (s *OpenAIGatewayService) relayOpenAICodexTurnState(c *gin.Context, account *Account, upstream http.Header) {
	if c == nil || c.Writer == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		c.Writer.Header().Del(canonical)
		return
	}
	c.Writer.Header().Set(canonical, state)
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

// stageOpenAICodexTurnState adds the token to delayed response headers. The
// provenance is recorded only after those headers become client-visible.
func stageOpenAICodexTurnState(dst *http.Header, upstream http.Header) {
	if dst == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		if *dst != nil {
			dst.Del(canonical)
		}
		return
	}
	if *dst == nil {
		*dst = http.Header{}
	}
	dst.Set(canonical, state)
}

func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if staged == nil || strings.TrimSpace(staged.Get(openAICodexTurnStateHeader)) == "" {
		return
	}
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

func extractOpenAICodexTurnState(upstream http.Header) string {
	if upstream == nil {
		return ""
	}
	return strings.TrimSpace(upstream.Get(openAICodexTurnStateHeader))
}

func (s *OpenAIGatewayService) noteOpenAICodexTurnStateProvenance(c *gin.Context, account *Account) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	s.openaiCodexTurnStateOrigins.Store(seed, openAICodexTurnStateOrigin{
		accountID: account.ID,
		expiresAt: time.Now().Add(s.openAIWSSessionStickyTTL()),
	})
	s.sweepOpenAICodexTurnStateOrigins()
}

// guardOpenAICodexTurnStateEcho strips a token known to have been minted by a
// different account. Same-account and unknown tokens remain untouched.
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	if s == nil || h == nil || account == nil || strings.TrimSpace(h.Get(openAICodexTurnStateHeader)) == "" {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	raw, ok := s.openaiCodexTurnStateOrigins.Load(seed)
	if !ok {
		return
	}
	origin, ok := raw.(openAICodexTurnStateOrigin)
	if !ok {
		s.openaiCodexTurnStateOrigins.Delete(seed)
		return
	}
	if !origin.expiresAt.IsZero() && time.Now().After(origin.expiresAt) {
		s.openaiCodexTurnStateOrigins.Delete(seed)
		return
	}
	if origin.accountID != account.ID {
		h.Del(openAICodexTurnStateHeader)
	}
}

// sweepOpenAICodexTurnStateOrigins bounds stale provenance with an
// opportunistic sweep every 256 writes.
func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexTurnStateOrigins.Range(func(key, value any) bool {
		origin, ok := value.(openAICodexTurnStateOrigin)
		if !ok || (!origin.expiresAt.IsZero() && now.After(origin.expiresAt)) {
			s.openaiCodexTurnStateOrigins.Delete(key)
		}
		return true
	})
}
