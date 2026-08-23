package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	maxAccountCreateTemplates           = 50
	maxAccountCreateTemplateNameRunes   = 80
	maxAccountCreateTemplateGroups      = 64
	maxAccountCreateTemplateConcurrency = 10000
	maxAccountCreateTemplatePriority    = 10000
)

var (
	ErrAccountCreateTemplateNotFound = infraerrors.NotFound(
		"ACCOUNT_CREATE_TEMPLATE_NOT_FOUND",
		"account create template not found",
	)
	ErrAccountCreateTemplateLimit = infraerrors.BadRequest(
		"ACCOUNT_CREATE_TEMPLATE_LIMIT",
		"too many account create templates",
	)
	ErrAccountCreateTemplateNameDuplicate = infraerrors.BadRequest(
		"ACCOUNT_CREATE_TEMPLATE_NAME_DUPLICATE",
		"template name already exists for this platform and type",
	)
	ErrAccountCreateTemplateNameInvalid = infraerrors.BadRequest(
		"ACCOUNT_CREATE_TEMPLATE_NAME_INVALID",
		"template name is required",
	)
	ErrAccountCreateTemplateScopeInvalid = infraerrors.BadRequest(
		"ACCOUNT_CREATE_TEMPLATE_SCOPE_INVALID",
		"unsupported platform or account type",
	)
)

type accountCreateTemplateStore struct {
	Items []AccountCreateTemplate `json:"items"`
}

// AccountCreateTemplate is a named snapshot of account create/edit defaults.
type AccountCreateTemplate struct {
	ID            string                      `json:"id"`
	Name          string                      `json:"name"`
	Platform      string                      `json:"platform"`
	Type          string                      `json:"type"`
	IsDefault     bool                        `json:"is_default"`
	IncludeGroups bool                        `json:"include_groups"`
	Values        AccountCreateTemplateValues `json:"values"`
}

// AccountCreateTemplateValues holds the fields copied into the account form.
type AccountCreateTemplateValues struct {
	ProxyID                  *int64   `json:"proxy_id"`
	Concurrency              int      `json:"concurrency"`
	LoadFactor               *int     `json:"load_factor"`
	Priority                 int      `json:"priority"`
	RateMultiplier           float64  `json:"rate_multiplier"`
	GroupIDs                 []int64  `json:"group_ids"`
	QuotaLimit               *float64 `json:"quota_limit"`
	QuotaDailyLimit          *float64 `json:"quota_daily_limit"`
	QuotaWeeklyLimit         *float64 `json:"quota_weekly_limit"`
	AutoPauseOnExpired       bool     `json:"auto_pause_on_expired"`
	InterceptWarmup          bool     `json:"intercept_warmup"`
	OpenAIPassthrough        bool     `json:"openai_passthrough"`
	OpenAIFlattenNamespaces  bool     `json:"openai_flatten_namespaces"`
	OpenAILongContextBilling bool     `json:"openai_long_context_billing"`
	OpenAIWSMode             string   `json:"openai_ws_mode"`
	OpenAICompactMode        string   `json:"openai_compact_mode"`
	CodexCLIOnly             bool     `json:"codex_cli_only"`
	CodexCLIOnlyAppServer    bool     `json:"codex_cli_only_app_server"`
	CodexFingerprintMode     string   `json:"codex_fingerprint_mode"`
	TLSFingerprintEnabled    bool     `json:"tls_fingerprint_enabled"`
	TLSFingerprintProfileID  *int64   `json:"tls_fingerprint_profile_id"`
}

// AccountCreateTemplateInput is the writable payload for create/update.
type AccountCreateTemplateInput struct {
	Name          string
	Platform      string
	Type          string
	IsDefault     bool
	IncludeGroups bool
	Values        AccountCreateTemplateValues
}

var allowedAccountCreateTemplatePlatforms = map[string]struct{}{
	PlatformAnthropic:   {},
	PlatformOpenAI:      {},
	PlatformGemini:      {},
	PlatformAntigravity: {},
	PlatformGrok:        {},
	PlatformKimi:        {},
	PlatformZhipu:       {},
	PlatformDeepseek:    {},
}

var allowedAccountCreateTemplateTypes = map[string]struct{}{
	AccountTypeOAuth:          {},
	AccountTypeSetupToken:     {},
	AccountTypeAPIKey:         {},
	AccountTypeUpstream:       {},
	AccountTypeBedrock:        {},
	AccountTypeServiceAccount: {},
}

var allowedAccountCreateTemplateWSModes = map[string]struct{}{
	"off":         {},
	"ctx_pool":    {},
	"passthrough": {},
	"http_bridge": {},
}

var allowedAccountCreateTemplateFingerprintModes = map[string]struct{}{
	"off":      {},
	"device":   {},
	"session":  {},
	"window":   {},
	"window40": {},
	"full":     {},
}

var allowedAccountCreateTemplateCompactModes = map[string]struct{}{
	"auto":      {},
	"force_on":  {},
	"force_off": {},
}

// ListAccountCreateTemplates returns stored templates, optionally filtered.
func (s *SettingService) ListAccountCreateTemplates(ctx context.Context, platform, accountType string) ([]AccountCreateTemplate, error) {
	items, err := s.loadAccountCreateTemplates(ctx)
	if err != nil {
		return nil, err
	}
	platform = strings.TrimSpace(platform)
	accountType = strings.TrimSpace(accountType)
	if platform == "" && accountType == "" {
		return items, nil
	}
	filtered := make([]AccountCreateTemplate, 0, len(items))
	for _, item := range items {
		if platform != "" && item.Platform != platform {
			continue
		}
		if accountType != "" && item.Type != accountType {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// CreateAccountCreateTemplate appends a normalized template to the store.
func (s *SettingService) CreateAccountCreateTemplate(ctx context.Context, input AccountCreateTemplateInput) (*AccountCreateTemplate, error) {
	items, err := s.loadAccountCreateTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) >= maxAccountCreateTemplates {
		return nil, ErrAccountCreateTemplateLimit
	}
	normalized, err := normalizeAccountCreateTemplate(AccountCreateTemplate{
		ID:            uuid.NewString(),
		Name:          input.Name,
		Platform:      input.Platform,
		Type:          input.Type,
		IsDefault:     input.IsDefault,
		IncludeGroups: input.IncludeGroups,
		Values:        input.Values,
	})
	if err != nil {
		return nil, err
	}
	if accountCreateTemplateNameExists(items, normalized.Platform, normalized.Type, normalized.Name, "") {
		return nil, ErrAccountCreateTemplateNameDuplicate
	}
	if normalized.IsDefault {
		clearAccountCreateTemplateDefaults(items, normalized.Platform, normalized.Type)
	}
	items = append(items, normalized)
	if err := s.saveAccountCreateTemplates(ctx, items); err != nil {
		return nil, err
	}
	return &normalized, nil
}

// UpdateAccountCreateTemplate replaces one stored template.
func (s *SettingService) UpdateAccountCreateTemplate(ctx context.Context, id string, input AccountCreateTemplateInput) (*AccountCreateTemplate, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrAccountCreateTemplateNotFound
	}
	items, err := s.loadAccountCreateTemplates(ctx)
	if err != nil {
		return nil, err
	}
	index := indexAccountCreateTemplate(items, id)
	if index < 0 {
		return nil, ErrAccountCreateTemplateNotFound
	}
	normalized, err := normalizeAccountCreateTemplate(AccountCreateTemplate{
		ID:            id,
		Name:          input.Name,
		Platform:      input.Platform,
		Type:          input.Type,
		IsDefault:     input.IsDefault,
		IncludeGroups: input.IncludeGroups,
		Values:        input.Values,
	})
	if err != nil {
		return nil, err
	}
	if accountCreateTemplateNameExists(items, normalized.Platform, normalized.Type, normalized.Name, id) {
		return nil, ErrAccountCreateTemplateNameDuplicate
	}
	if normalized.IsDefault {
		clearAccountCreateTemplateDefaults(items, normalized.Platform, normalized.Type)
	}
	items[index] = normalized
	if err := s.saveAccountCreateTemplates(ctx, items); err != nil {
		return nil, err
	}
	return &normalized, nil
}

// DeleteAccountCreateTemplate removes one stored template.
func (s *SettingService) DeleteAccountCreateTemplate(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrAccountCreateTemplateNotFound
	}
	items, err := s.loadAccountCreateTemplates(ctx)
	if err != nil {
		return err
	}
	index := indexAccountCreateTemplate(items, id)
	if index < 0 {
		return ErrAccountCreateTemplateNotFound
	}
	items = append(items[:index], items[index+1:]...)
	return s.saveAccountCreateTemplates(ctx, items)
}

func (s *SettingService) loadAccountCreateTemplates(ctx context.Context) ([]AccountCreateTemplate, error) {
	if s == nil || s.settingRepo == nil {
		return []AccountCreateTemplate{}, nil
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAccountCreateTemplates)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []AccountCreateTemplate{}, nil
		}
		return nil, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return []AccountCreateTemplate{}, nil
	}
	var store accountCreateTemplateStore
	if err := json.Unmarshal([]byte(value), &store); err != nil {
		var legacy []AccountCreateTemplate
		if legacyErr := json.Unmarshal([]byte(value), &legacy); legacyErr != nil {
			return []AccountCreateTemplate{}, nil
		}
		store.Items = legacy
	}
	normalized := make([]AccountCreateTemplate, 0, len(store.Items))
	seenIDs := make(map[string]struct{}, len(store.Items))
	for _, item := range store.Items {
		item, err := normalizeAccountCreateTemplate(item)
		if err != nil {
			continue
		}
		if _, exists := seenIDs[item.ID]; exists {
			continue
		}
		if accountCreateTemplateNameExists(normalized, item.Platform, item.Type, item.Name, item.ID) {
			continue
		}
		if item.IsDefault {
			clearAccountCreateTemplateDefaults(normalized, item.Platform, item.Type)
		}
		seenIDs[item.ID] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func (s *SettingService) saveAccountCreateTemplates(ctx context.Context, items []AccountCreateTemplate) error {
	if s == nil || s.settingRepo == nil {
		return infraerrors.InternalServer("SETTING_REPO_UNAVAILABLE", "setting repository is not configured")
	}
	payload, err := json.Marshal(accountCreateTemplateStore{Items: items})
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyAccountCreateTemplates, string(payload))
}

func normalizeAccountCreateTemplate(item AccountCreateTemplate) (AccountCreateTemplate, error) {
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > maxAccountCreateTemplateNameRunes {
		return AccountCreateTemplate{}, ErrAccountCreateTemplateNameInvalid
	}
	item.Platform = strings.TrimSpace(item.Platform)
	item.Type = strings.TrimSpace(item.Type)
	if _, ok := allowedAccountCreateTemplatePlatforms[item.Platform]; !ok {
		return AccountCreateTemplate{}, ErrAccountCreateTemplateScopeInvalid
	}
	if _, ok := allowedAccountCreateTemplateTypes[item.Type]; !ok {
		return AccountCreateTemplate{}, ErrAccountCreateTemplateScopeInvalid
	}
	item.Values = normalizeAccountCreateTemplateValues(item.Values)
	if !item.IncludeGroups {
		item.Values.GroupIDs = []int64{}
	}
	return item, nil
}

func normalizeAccountCreateTemplateValues(values AccountCreateTemplateValues) AccountCreateTemplateValues {
	values.ProxyID = normalizeOptionalPositiveID(values.ProxyID)
	if values.Concurrency < 1 {
		values.Concurrency = 1
	}
	if values.Concurrency > maxAccountCreateTemplateConcurrency {
		values.Concurrency = maxAccountCreateTemplateConcurrency
	}
	if values.LoadFactor != nil {
		if *values.LoadFactor < 1 {
			values.LoadFactor = nil
		} else if *values.LoadFactor > maxAccountCreateTemplateConcurrency {
			capped := maxAccountCreateTemplateConcurrency
			values.LoadFactor = &capped
		}
	}
	if values.Priority < 1 {
		values.Priority = 1
	}
	if values.Priority > maxAccountCreateTemplatePriority {
		values.Priority = maxAccountCreateTemplatePriority
	}
	if values.RateMultiplier < 0 || !isFiniteNumber(values.RateMultiplier) {
		values.RateMultiplier = 1
	}
	values.GroupIDs = normalizePositiveIDs(values.GroupIDs, maxAccountCreateTemplateGroups)
	values.QuotaLimit = normalizeOptionalNonNegativeFloat(values.QuotaLimit)
	values.QuotaDailyLimit = normalizeOptionalNonNegativeFloat(values.QuotaDailyLimit)
	values.QuotaWeeklyLimit = normalizeOptionalNonNegativeFloat(values.QuotaWeeklyLimit)
	if _, ok := allowedAccountCreateTemplateWSModes[values.OpenAIWSMode]; !ok {
		values.OpenAIWSMode = "off"
	}
	if _, ok := allowedAccountCreateTemplateCompactModes[values.OpenAICompactMode]; !ok {
		values.OpenAICompactMode = "auto"
	}
	if _, ok := allowedAccountCreateTemplateFingerprintModes[values.CodexFingerprintMode]; !ok {
		values.CodexFingerprintMode = "off"
	}
	if values.TLSFingerprintProfileID != nil {
		if *values.TLSFingerprintProfileID == 0 {
			values.TLSFingerprintProfileID = nil
		} else if *values.TLSFingerprintProfileID < -1 {
			values.TLSFingerprintProfileID = nil
		}
	}
	if !values.TLSFingerprintEnabled {
		values.TLSFingerprintProfileID = nil
	}
	return values
}

func normalizeOptionalPositiveID(id *int64) *int64 {
	if id == nil || *id <= 0 {
		return nil
	}
	return id
}

func normalizeOptionalNonNegativeFloat(value *float64) *float64 {
	if value == nil || !isFiniteNumber(*value) || *value <= 0 {
		return nil
	}
	return value
}

func normalizePositiveIDs(ids []int64, limit int) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func accountCreateTemplateNameExists(items []AccountCreateTemplate, platform, accountType, name, exceptID string) bool {
	target := strings.ToLower(strings.TrimSpace(name))
	for _, item := range items {
		if item.ID == exceptID {
			continue
		}
		if item.Platform == platform && item.Type == accountType && strings.ToLower(item.Name) == target {
			return true
		}
	}
	return false
}

func clearAccountCreateTemplateDefaults(items []AccountCreateTemplate, platform, accountType string) {
	for i := range items {
		if items[i].Platform == platform && items[i].Type == accountType {
			items[i].IsDefault = false
		}
	}
}

func indexAccountCreateTemplate(items []AccountCreateTemplate, id string) int {
	for i, item := range items {
		if item.ID == id {
			return i
		}
	}
	return -1
}

func isFiniteNumber(value float64) bool {
	return !((value > 0 && value+1 == value) || (value < 0 && value-1 == value) || value != value)
}
