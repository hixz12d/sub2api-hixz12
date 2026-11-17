package securityaudit

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Upstream refusal handling: a request the guard let through (judged safe,
// pass-cache hit or fail_open) that the upstream Claude account then refused
// is archived as a review event, dropped from the pass cache and remembered in
// a per-process refused cache so identical retries are blocked locally.
const (
	// RefusedCacheTTL is how long an upstream refusal blocks identical retries.
	RefusedCacheTTL = 30 * time.Minute
	// RefusedCacheMaxEntries bounds the in-process refused cache.
	RefusedCacheMaxEntries = 10000

	// refusalHistoryTurns / refusalHistoryMaxRunes bound the "earlier
	// conversation" part of a refusal archive.
	refusalHistoryTurns    = 3
	refusalHistoryMaxRunes = 20000
	// refusalJudgeViewMaxRunes leaves room for the history inside full_prompt.
	refusalJudgeViewMaxRunes = DefaultFullPromptMaxRunes - refusalHistoryMaxRunes - 64

	AllowViaJudged   = "judged"
	AllowViaCacheHit = "cache_hit"
	AllowViaFailOpen = "fail_open"

	StageRefusedAfterAllow = "refused_after_allow"
	StageRefusedRepeat     = "refused_repeat"

	CategoryUpstreamRefusal       = "upstream_refusal"
	CategoryUpstreamRefusalRepeat = "upstream_refusal_repeat"

	ScannerBackendUpstreamRefusal = "upstream-refusal"
	ScannerBackendRefusalCache    = "refusal-cache"

	EvidenceKeyAllowVia        = "allow_via"
	EvidenceKeyRefusalKind     = "refusal_kind"
	EvidenceKeyClientRequestID = "client_request_id"
	EvidenceKeyConfigChanged   = "config_changed"

	refusalFullPromptJudgeHeader   = "【审核员看到的】"
	refusalFullPromptHistoryHeader = "【更早的对话】"
	refusalHistoryUserLabel        = "【用户】"
	refusalHistoryAILabel          = "【AI】"
	refusalHistoryEmpty            = "（无）"
)

// rawEvidenceKeys are stored verbatim (only length-capped): they hold IDs and
// enum values that the generic redaction would mangle.
var rawEvidenceKeys = map[string]struct{}{
	EvidenceKeyAllowVia: {}, EvidenceKeyRefusalKind: {}, EvidenceKeyClientRequestID: {}, EvidenceKeyConfigChanged: {},
}

// UpstreamRefusal describes one allowed request that the upstream refused.
type UpstreamRefusal struct {
	Request            Request
	AuditConfigVersion int64
	AllowVia           string
	RefusalKind        string
	// ClientRequestID matches usage_logs.request_id ("client:<id>" or
	// "local:<id>").
	ClientRequestID string
}

type upstreamRefusalHandler interface {
	HandleUpstreamRefusal(refusal UpstreamRefusal)
}

// HandleUpstreamRefusal forwards a refusal to the prompt engine when it
// supports refusal handling.
func (c *Coordinator) HandleUpstreamRefusal(refusal UpstreamRefusal) {
	if c == nil || c.prompt == nil {
		return
	}
	if handler, ok := c.prompt.(upstreamRefusalHandler); ok {
		handler.HandleUpstreamRefusal(refusal)
	}
}

// HandleUpstreamRefusal archives the refused request and, when the config is
// unchanged since the audit, moves its cache key from the pass cache to the
// refused cache. The archive is written in the background; failures are only
// logged because the response has already been sent.
func (s *PromptService) HandleUpstreamRefusal(refusal UpstreamRefusal) {
	if s == nil || s.config == nil || s.evaluator == nil || refusal.AllowVia == "" || refusal.RefusalKind == "" {
		return
	}
	fields := requestLogFields(refusal.Request)
	fields["client_request_id"] = refusal.ClientRequestID
	fields["allow_via"] = refusal.AllowVia
	fields["refusal_kind"] = refusal.RefusalKind
	fields["audit_config_version"] = refusal.AuditConfigVersion
	if s.metrics != nil {
		s.metrics.IncUpstreamRefusal()
	}
	cfg, ok := s.config.Active()
	if !ok {
		LogWarn(EventUpstreamRefusal, mergeLogFields(fields, map[string]any{"status": "config_unavailable"}))
		return
	}
	fields["config_version"] = cfg.ConfigVersion
	snapshot, err := ExtractBlockingPromptSnapshot(refusal.Request, cfg.BlockingLatestTurnOnly)
	if err != nil {
		LogWarn(EventUpstreamRefusal, mergeLogFields(fields, map[string]any{"status": "snapshot_failed"}))
		return
	}
	configChanged := cfg.ConfigVersion != refusal.AuditConfigVersion
	status := "recorded"
	if configChanged {
		// The cache key would no longer match the audited verdict: archive only.
		status = "recorded_config_changed"
	} else {
		s.evaluator.rememberUpstreamRefusal(cfg, snapshot)
	}
	LogWarn(EventUpstreamRefusal, mergeLogFields(fields, map[string]any{"status": status}))
	if s.repo == nil {
		return
	}
	configVersion := refusal.AuditConfigVersion
	if configVersion < 1 {
		configVersion = cfg.ConfigVersion
	}
	evidence := map[string]string{
		EvidenceKeyAllowVia: refusal.AllowVia, EvidenceKeyRefusalKind: refusal.RefusalKind,
		EvidenceKeyClientRequestID: refusal.ClientRequestID,
	}
	if configChanged {
		evidence[EvidenceKeyConfigChanged] = "true"
	}
	result := &NormalizedResult{
		Decision: EventFlag, RiskLevel: RiskMedium, Action: ActionWarn, Safety: "Controversial",
		Categories: []string{CategoryUpstreamRefusal}, MatchedScanners: []string{},
		ScannerScores: map[string]float64{}, ScannerEvidence: evidence,
		ScannerBackend: ScannerBackendUpstreamRefusal, ScannerVersion: refusal.RefusalKind,
		PolicyID: "priority", PolicyVersion: 1,
	}
	record := snapshot.Redacted()
	record.Stage = StageRefusedAfterAllow
	record.FullPrompt = buildRefusalFullPrompt(cfg, snapshot.ScanText, extractRefusalHistory(refusal.Request))
	s.lifecycleMu.Lock()
	background := s.background
	s.lifecycleMu.Unlock()
	if background == nil {
		background = context.Background()
	}
	s.enqueueWG.Add(1)
	go func() {
		defer s.enqueueWG.Done()
		ctx, cancel := context.WithTimeout(background, 5*time.Second)
		defer cancel()
		// Always stored: a flag event does not depend on store_pass_events.
		if _, recordErr := s.repo.RecordBlocking(ctx, record, configVersion, result, true); recordErr != nil {
			if s.metrics != nil {
				s.metrics.IncRecordFailed()
			}
			LogWarn(EventResultRecordFailed, mergeLogFields(fields, map[string]any{
				"error_code": "result_record_failed", "stage": StageRefusedAfterAllow, "status": "failed",
			}))
		}
	}()
}

// rememberUpstreamRefusal drops the pass-cache verdict for the refused scan
// text and blocks identical retries for RefusedCacheTTL.
func (g *GuardEvaluator) rememberUpstreamRefusal(cfg ActiveConfig, snapshot PromptSnapshot) {
	if g == nil || strings.TrimSpace(snapshot.ScanText) == "" {
		return
	}
	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		return
	}
	key := passCacheKey(cfg, endpoints, snapshot.ScanText)
	g.passCache.Delete(key)
	g.refusedCache.Store(key, g.clock.Now())
}

// blockRefusalRepeat answers a refused-cache hit like a guard block, without
// calling any guard endpoint. The event keeps only the prompt hash; the first
// refusal archive holds the text.
func (g *GuardEvaluator) blockRefusalRepeat(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot, baseFields map[string]any, start time.Time) *PromptDecision {
	latency := g.clock.Now().Sub(start)
	result := &NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe",
		Categories: []string{CategoryUpstreamRefusalRepeat}, MatchedScanners: []string{},
		ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		ScannerBackend: ScannerBackendRefusalCache, ScannerVersion: "refused_cache",
		PolicyID: "priority", PolicyVersion: 1, LatencyMS: int(latency.Milliseconds()),
	}
	if g.metrics != nil {
		g.metrics.IncRefusalRepeatBlock()
		g.metrics.Observe(DecisionBlock, latency)
	}
	if g.repo != nil {
		record := snapshot.Redacted()
		record.Stage = StageRefusedRepeat
		record.FullPrompt = ""
		if _, recordErr := g.repo.RecordBlocking(ctx, record, cfg.ConfigVersion, result, cfg.StorePassEvents); recordErr != nil {
			if g.metrics != nil {
				g.metrics.IncRecordFailed()
			}
			LogWarn(EventResultRecordFailed, mergeLogFields(baseFields, map[string]any{
				"decision": DecisionBlock, "error_code": "result_record_failed", "stage": StageRefusedRepeat,
				"status": "failed",
			}))
		}
	}
	LogInfo(EventRefusalRepeatBlocked, mergeLogFields(baseFields, map[string]any{
		"decision": DecisionBlock, "risk_level": RiskCritical, "action": ActionBlock,
		"latency_ms": latency.Milliseconds(), "status": "blocked", "error_code": ErrorCodeBlocked,
		"upstream_dispatched": false, "billing_preconsumed": false, "scanner_backend": ScannerBackendRefusalCache,
	}))
	return &PromptDecision{Kind: DecisionBlock, Result: result, ErrorCode: ErrorCodeBlocked, AllowNextStage: false}
}

// buildRefusalFullPrompt renders the archive text:
//
//	【审核员看到的】
//	<上下文>…</上下文>        (when present)
//	<待审内容>…</待审内容>    (one per chunk)
//
//	【更早的对话】
//	【用户】/【AI】 blocks, or （无）
func buildRefusalFullPrompt(cfg ActiveConfig, scanText, history string) string {
	var judgeView string
	if cfg.JudgeMode() {
		judgeView = strings.Join(buildJudgeChunks(scanText, minimumInputLimit(cfg.EnabledEndpoints())), "\n\n")
	} else {
		judgeView = strings.ReplaceAll(scanText, promptAuditPrioritySeparator, "\n\n")
	}
	judgeView = TrimRunes(strings.TrimSpace(strings.ReplaceAll(judgeView, "\x00", "")), refusalJudgeViewMaxRunes)
	if strings.TrimSpace(history) == "" {
		history = refusalHistoryEmpty
	}
	return BuildFullPrompt(refusalFullPromptJudgeHeader+"\n"+judgeView+"\n\n"+refusalFullPromptHistoryHeader+"\n"+history, DefaultFullPromptMaxRunes)
}

// extractRefusalHistory returns the user and assistant text of the last
// refusalHistoryTurns turns before the latest user turn, from the full request
// rather than the latest-turn-only snapshot, cut to its last
// refusalHistoryMaxRunes runes.
func extractRefusalHistory(req Request) string {
	var document any
	if json.Unmarshal(req.Body, &document) != nil {
		return ""
	}
	normalized, _ := dropClientSystemReminders(normalizedPromptSegments(extractProtocolSegments(req.Protocol, document)))
	end := latestUserSegmentStart(normalized)
	if end < 0 {
		end = len(normalized)
	}
	start := end
	userTurns := 0
	for index := end - 1; index >= 0; index-- {
		segment := normalized[index]
		if !isUserSegment(segment) && !isAssistantOutputSegment(segment) {
			continue
		}
		start = index
		if isUserSegment(segment) && (index == 0 || !isUserSegment(normalized[index-1])) {
			userTurns++
			if userTurns >= refusalHistoryTurns {
				break
			}
		}
	}
	var builder strings.Builder
	lastLabel := ""
	for _, segment := range normalized[start:end] {
		label := ""
		switch {
		case isUserSegment(segment):
			label = refusalHistoryUserLabel
		case isAssistantOutputSegment(segment):
			label = refusalHistoryAILabel
		default:
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		if label != lastLabel {
			builder.WriteString(label + "\n")
			lastLabel = label
		}
		builder.WriteString(segment.text)
	}
	history := strings.TrimSpace(strings.ReplaceAll(builder.String(), "\x00", ""))
	if runes := []rune(history); len(runes) > refusalHistoryMaxRunes {
		history = "…" + strings.TrimSpace(string(runes[len(runes)-refusalHistoryMaxRunes:]))
	}
	return history
}

// storedEvidenceValue redacts free-text evidence but keeps IDs and enum values
// listed in rawEvidenceKeys verbatim.
func storedEvidenceValue(key, value string) string {
	if _, raw := rawEvidenceKeys[key]; raw {
		return TrimRunes(strings.TrimSpace(value), 160)
	}
	return RedactPreview(value, 160)
}
