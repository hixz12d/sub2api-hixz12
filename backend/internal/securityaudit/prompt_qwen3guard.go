package securityaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type ScannerDefinition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LabelZH     string `json:"label_zh"`
	Description string `json:"description"`
}

var AllScannerIDs = []string{
	"violent",
	"non_violent_illegal_acts",
	"sexual_content_or_sexual_acts",
	"pii",
	"suicide_and_self_harm",
	"unethical_acts",
	"politically_sensitive_topics",
	"copyright_violation",
	"jailbreak",
	"cbrn_weapons",
	"cyber_attack",
	"child_safety",
}

var ScannerCatalog = map[string]ScannerDefinition{
	"violent":                       {ID: "violent", Label: "Violent", LabelZH: "暴力", Description: "Violence or threats of violence"},
	"non_violent_illegal_acts":      {ID: "non_violent_illegal_acts", Label: "Non-violent Illegal Acts", LabelZH: "非暴力违法行为", Description: "Non-violent illegal activity"},
	"sexual_content_or_sexual_acts": {ID: "sexual_content_or_sexual_acts", Label: "Sexual Content or Sexual Acts", LabelZH: "性内容或性行为", Description: "Sexual content or sexual acts"},
	"pii":                           {ID: "pii", Label: "PII", LabelZH: "个人敏感信息", Description: "Personal identifying information"},
	"suicide_and_self_harm":         {ID: "suicide_and_self_harm", Label: "Suicide & Self-Harm", LabelZH: "自杀与自残", Description: "Suicide or self-harm"},
	"unethical_acts":                {ID: "unethical_acts", Label: "Unethical Acts", LabelZH: "不道德行为", Description: "Unethical behavior"},
	"politically_sensitive_topics":  {ID: "politically_sensitive_topics", Label: "Politically Sensitive Topics", LabelZH: "政治敏感话题", Description: "Politically sensitive topics"},
	"copyright_violation":           {ID: "copyright_violation", Label: "Copyright Violation", LabelZH: "版权侵权", Description: "Copyright infringement"},
	"jailbreak":                     {ID: "jailbreak", Label: "Jailbreak", LabelZH: "越狱攻击", Description: "Prompt injection or jailbreak attempt"},
	"cbrn_weapons":                  {ID: "cbrn_weapons", Label: "CBRN Weapons", LabelZH: "生化放核武器", Description: "Chemical, biological, radiological or nuclear weapons uplift"},
	"cyber_attack":                  {ID: "cyber_attack", Label: "Cyber Attack", LabelZH: "网络攻击", Description: "Malware, intrusion or other cyber attack assistance"},
	"child_safety":                  {ID: "child_safety", Label: "Child Safety", LabelZH: "儿童安全", Description: "Sexualization, abuse or exploitation of minors"},
}

var categoryAliases = map[string]string{
	"violent": "violent", "violence": "violent",
	"non violent illegal acts": "non_violent_illegal_acts", "non-violent illegal acts": "non_violent_illegal_acts",
	"sexual content or sexual acts": "sexual_content_or_sexual_acts", "sexual": "sexual_content_or_sexual_acts",
	"pii": "pii", "personal identifying information": "pii", "personal identifiable information": "pii",
	"suicide self harm": "suicide_and_self_harm", "suicide and self harm": "suicide_and_self_harm", "suicide & self-harm": "suicide_and_self_harm",
	"unethical acts": "unethical_acts", "unethical": "unethical_acts",
	"politically sensitive topics": "politically_sensitive_topics", "political": "politically_sensitive_topics",
	"copyright violation": "copyright_violation", "copyright": "copyright_violation",
	"jailbreak": "jailbreak", "prompt injection": "jailbreak",
	"cbrn": "cbrn_weapons", "cbrn weapons": "cbrn_weapons", "weapons": "cbrn_weapons", "weapon": "cbrn_weapons",
	"chemical weapons": "cbrn_weapons", "biological weapons": "cbrn_weapons", "bioweapons": "cbrn_weapons",
	"nuclear weapons": "cbrn_weapons", "radiological weapons": "cbrn_weapons", "weapons of mass destruction": "cbrn_weapons",
	"cyber": "cyber_attack", "cyber attack": "cyber_attack", "cyberattack": "cyber_attack", "cyber attacks": "cyber_attack",
	"hacking": "cyber_attack", "malware": "cyber_attack", "cybercrime": "cyber_attack", "malicious code": "cyber_attack",
	"csam": "child_safety", "child safety": "child_safety", "child sexual content": "child_safety",
	"child sexual abuse": "child_safety", "child sexual abuse material": "child_safety", "child abuse": "child_safety",
	"child exploitation": "child_safety", "minors": "child_safety",
}

type GuardError struct {
	Code       string
	HTTPStatus int
	Retryable  bool
	Timeout    bool
	Cause      error
}

func (e *GuardError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Code
}

func (e *GuardError) Unwrap() error { return e.Cause }

func NormalizeCategory(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", " ", "&", " and ", "/", " ", "-", " ", "–", " ", "—", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if canonical, ok := categoryAliases[normalized]; ok {
		return canonical
	}
	return strings.ReplaceAll(normalized, " ", "_")
}

// ScannerBackendJudge marks results produced by an endpoint running with judge
// instructions (system_prompt), so they can be told apart from Qwen3Guard.
const ScannerBackendJudge = "openai-judge"

// JudgeReasonEvidenceKey stores the judge's one-line reason in ScannerEvidence.
const JudgeReasonEvidenceKey = "judge_reason"

// judgeMaxCompletionTokens bounds the judge verdict (Safety/Categories/Reason).
const judgeMaxCompletionTokens = 400

func ParseQwen3Guard(content string, enabledScanners []string) (*NormalizedResult, error) {
	return parseGuardVerdict(content, enabledScanners, false)
}

// parseGuardVerdict parses the Safety/Categories verdict. In judge mode an
// optional Reason line is captured, light Markdown decoration is tolerated and
// Controversial results are never elevated to a block.
func parseGuardVerdict(content string, enabledScanners []string, judge bool) (*NormalizedResult, error) {
	var safety string
	var categoryLine string
	var reason string
	categorySeen := false
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if judge {
			line = strings.ReplaceAll(line, "**", "")
			line = strings.ReplaceAll(line, "：", ":")
			line = strings.Trim(strings.TrimSpace(line), "`*#->")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "safety:"):
			if safety != "" {
				return nil, &GuardError{Code: ErrorCodeInvalidResponse}
			}
			safety = strings.TrimSpace(line[len("safety:"):])
		case strings.HasPrefix(lower, "categories:"):
			if (judge && categorySeen) || categoryLine != "" {
				return nil, &GuardError{Code: ErrorCodeInvalidResponse}
			}
			categorySeen = true
			categoryLine = strings.TrimSpace(line[len("categories:"):])
		case judge && strings.HasPrefix(lower, "reason:"):
			if reason == "" {
				reason = strings.TrimSpace(line[len("reason:"):])
			}
		default:
			// Auxiliary Guard fields, such as Refusal, do not affect audit decisions.
		}
	}
	if judge {
		safety = strings.Trim(safety, " `*.")
	}
	switch strings.ToLower(safety) {
	case "safe":
		safety = "Safe"
	case "controversial":
		safety = "Controversial"
	case "unsafe":
		safety = "Unsafe"
	default:
		return nil, &GuardError{Code: ErrorCodeInvalidResponse}
	}
	if categoryLine == "" {
		// A judge that answers Safe without a Categories line has still given an
		// unambiguous verdict; only risky verdicts must name their categories.
		if !judge || safety != "Safe" {
			return nil, &GuardError{Code: ErrorCodeInvalidResponse}
		}
		categoryLine = "None"
	}
	enabled := make(map[string]struct{}, len(enabledScanners))
	for _, scanner := range enabledScanners {
		enabled[NormalizeCategory(scanner)] = struct{}{}
	}
	known := map[string]struct{}{}
	unknown := map[string]struct{}{}
	for _, raw := range strings.Split(categoryLine, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "n/a") {
			continue
		}
		category := NormalizeCategory(raw)
		if _, ok := ScannerCatalog[category]; ok {
			known[category] = struct{}{}
		} else {
			unknown[unknownCategoryID(category)] = struct{}{}
		}
	}
	knownList := orderedScannerKeys(known)
	unknownList := sortedKeys(unknown)
	matched := make([]string, 0, len(knownList))
	for _, category := range knownList {
		if _, ok := enabled[category]; ok {
			matched = append(matched, category)
		}
	}
	result := &NormalizedResult{
		Safety: safety, Categories: knownList, MatchedScanners: matched, UnknownCategories: unknownList,
		ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		ScannerBackend: "qwen3guard-openai", ScannerVersion: "qwen3guard",
		PolicyID: "priority", PolicyVersion: 1,
		Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow,
	}
	score := 0.0
	if safety == "Controversial" {
		score = 0.5
		result.Decision, result.RiskLevel, result.Action = EventFlag, RiskMedium, ActionWarn
	}
	if safety == "Unsafe" {
		score = 1
		if len(matched) > 0 || len(unknownList) > 0 || len(knownList) == 0 {
			result.Decision, result.RiskLevel, result.Action = EventCritical, RiskCritical, ActionBlock
		} else {
			result.Decision, result.RiskLevel, result.Action = EventFlag, RiskHigh, ActionWarn
		}
	}
	for _, category := range matched {
		result.ScannerScores[category] = score
		result.ScannerEvidence[category] = ScannerCatalog[category].Label
		// Judge mode deliberately keeps Controversial as a Flag: only an explicit
		// Unsafe verdict may block traffic.
		if !judge && safety == "Controversial" && isElevatedControversial(category) {
			result.Decision, result.RiskLevel, result.Action = EventCritical, RiskCritical, ActionBlock
		}
	}
	if judge {
		result.ScannerBackend = ScannerBackendJudge
		if reason != "" {
			result.ScannerEvidence[JudgeReasonEvidenceKey] = TrimRunes(reason, 160)
		}
	}
	return result, nil
}

func unknownCategoryID(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return fmt.Sprintf("unknown:%x", digest[:8])
}

func isElevatedControversial(category string) bool {
	return category == "jailbreak" || category == "pii" || category == "suicide_and_self_harm"
}

type OpenAICompatibleScanner struct {
	clients sync.Map
}

func NewOpenAICompatibleScanner() *OpenAICompatibleScanner { return &OpenAICompatibleScanner{} }

func (s *OpenAICompatibleScanner) Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error) {
	client, err := s.clientFor(endpoint)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}
	requestURL, err := ChatCompletionsURL(endpoint.BaseURL)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}
	systemPrompt := strings.TrimSpace(endpoint.SystemPrompt)
	judge := systemPrompt != ""
	var payload map[string]any
	if judge {
		// GPT-5 class reasoning models reject temperature, seed and max_tokens.
		payload = map[string]any{
			"model": endpoint.Model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": chunk},
			},
			"max_completion_tokens": judgeMaxCompletionTokens,
		}
		if effort := strings.TrimSpace(endpoint.ReasoningEffort); effort != "" {
			payload["reasoning_effort"] = effort
		}
	} else {
		payload = map[string]any{
			"model":       endpoint.Model,
			"messages":    []map[string]string{{"role": "user", "content": chunk}},
			"temperature": 0,
			"max_tokens":  64,
			"seed":        42,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if endpoint.Token != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		timeout := errors.Is(err, context.DeadlineExceeded)
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			timeout = true
		}
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Timeout: timeout, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, &GuardError{Code: ErrorCodeUnavailable, HTTPStatus: resp.StatusCode, Retryable: retryable}
	}
	limited := io.LimitReader(resp.Body, maxGuardResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: err}
	}
	if int64(len(responseBody)) > maxGuardResponseBytes {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse}
	}
	content, finishReason, err := extractOpenAIChoice(responseBody)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}
	if judge && strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: errors.New("prompt judge response truncated")}
	}
	result, err := parseGuardVerdict(content, enabledScanners, judge)
	if err != nil {
		return nil, err
	}
	result.GuardEndpointID = endpoint.ID
	result.ScannerVersion = endpoint.Model
	return result, nil
}

func (s *OpenAICompatibleScanner) clientFor(endpoint ActiveEndpoint) (*http.Client, error) {
	key := fmt.Sprintf("%s|%s|%d", endpoint.ID, endpoint.BaseURL, endpoint.TimeoutMS)
	if cached, ok := s.clients.Load(key); ok {
		client, valid := cached.(*http.Client)
		if !valid {
			s.clients.Delete(key)
			return nil, errors.New("prompt guard client cache invalid")
		}
		return client, nil
	}
	client, err := NewSecureHTTPClient(endpoint)
	if err != nil {
		return nil, err
	}
	actual, _ := s.clients.LoadOrStore(key, client)
	actualClient, ok := actual.(*http.Client)
	if !ok {
		s.clients.Delete(key)
		return nil, errors.New("prompt guard client cache invalid")
	}
	return actualClient, nil
}

func extractOpenAIContent(body []byte) (string, error) {
	content, _, err := extractOpenAIChoice(body)
	return content, err
}

// extractOpenAIChoice returns the first choice's text content and its
// finish_reason (empty when the upstream omits it).
func extractOpenAIChoice(body []byte) (string, string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil || len(response.Choices) == 0 {
		return "", "", errors.New("prompt guard response envelope invalid")
	}
	content, err := openAIMessageText(response.Choices[0].Message.Content)
	return content, response.Choices[0].FinishReason, err
}

func openAIMessageText(content any) (string, error) {
	switch typed := content.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", errors.New("prompt guard response content empty")
		}
		return typed, nil
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := object["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return "", errors.New("prompt guard response content empty")
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", errors.New("prompt guard response content invalid")
	}
}

func ScannerDefinitions() []ScannerDefinition {
	result := make([]ScannerDefinition, 0, len(AllScannerIDs))
	for _, id := range AllScannerIDs {
		result = append(result, ScannerCatalog[id])
	}
	sort.SliceStable(result, func(i, j int) bool { return i < j })
	return result
}
