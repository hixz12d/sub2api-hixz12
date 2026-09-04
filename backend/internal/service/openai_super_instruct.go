package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// Account-level Super-Instruct (bridge.md) injection.
//
// Default is off. Enable per account with extra.super_instruct=true.
// Optional extra.super_instruct_mode: "prepend" (default) | "replace".
// Optional extra.super_instruct_profile: selects bridge file under profiles/.
//
//	empty | default | super-instruct → gateway.super_instruct_bridge_file
//	portable-kit | <name> → <bridgeDir>/profiles/<name>.md
//
// Bridge text is loaded with mtime hot-reload (per path).
const (
	SuperInstructExtraKey        = "super_instruct"
	SuperInstructModeExtraKey    = "super_instruct_mode"
	SuperInstructProfileExtraKey = "super_instruct_profile"
	SuperInstructModePrepend     = "prepend"
	SuperInstructModeReplace     = "replace"

	SuperInstructProfileDefault       = "default"
	SuperInstructProfileSuperInstruct = "super-instruct"
	SuperInstructProfilePortableKit   = "portable-kit"

	// Markers used to avoid double-injecting any known bridge profile.
	superInstructMarkerSI  = "[Super-Instruct"
	superInstructMarkerKit = "[Portable-Kit"

	superInstructBridgeReloadMinInterval = 2 * time.Second
)

var superInstructProfileNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type superInstructBridgeCacheEntry struct {
	text      string
	modTime   time.Time
	checkedAt time.Time
}

var (
	superInstructBridgeMu    sync.Mutex
	superInstructBridgeCache = map[string]*superInstructBridgeCacheEntry{}
)

// IsSuperInstructEnabled reports whether this account opted into bridge injection.
// Missing or non-bool extra values count as disabled (safe default).
func (a *Account) IsSuperInstructEnabled() bool {
	if a == nil {
		return false
	}
	enabled, present := extraBool(a.Extra, SuperInstructExtraKey)
	return present && enabled
}

// SuperInstructMode returns prepend|replace. Unknown/empty values fall back to prepend.
func (a *Account) SuperInstructMode() string {
	if a == nil {
		return SuperInstructModePrepend
	}
	mode := strings.ToLower(strings.TrimSpace(a.GetExtraString(SuperInstructModeExtraKey)))
	if mode == SuperInstructModeReplace {
		return SuperInstructModeReplace
	}
	return SuperInstructModePrepend
}

// SuperInstructProfile returns the sanitized profile name for bridge selection.
// Empty / default / super-instruct all mean the primary bridge.md.
func (a *Account) SuperInstructProfile() string {
	if a == nil {
		return SuperInstructProfileDefault
	}
	raw := strings.ToLower(strings.TrimSpace(a.GetExtraString(SuperInstructProfileExtraKey)))
	if raw == "" || raw == SuperInstructProfileDefault || raw == SuperInstructProfileSuperInstruct {
		return SuperInstructProfileDefault
	}
	if !superInstructProfileNameRe.MatchString(raw) {
		return SuperInstructProfileDefault
	}
	return raw
}

func instructionsAlreadyInjected(existing string) bool {
	return strings.Contains(existing, superInstructMarkerSI) ||
		strings.Contains(existing, superInstructMarkerKit)
}

// loadSuperInstructBridgeText reads bridge text from path with a short-interval
// mtime cache (per path). On read errors it keeps the last good content (if any).
func loadSuperInstructBridgeText(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	now := time.Now()
	superInstructBridgeMu.Lock()
	defer superInstructBridgeMu.Unlock()

	entry := superInstructBridgeCache[path]
	if entry != nil &&
		!entry.checkedAt.IsZero() &&
		now.Sub(entry.checkedAt) < superInstructBridgeReloadMinInterval {
		return entry.text
	}
	if entry == nil {
		entry = &superInstructBridgeCacheEntry{}
		superInstructBridgeCache[path] = entry
	}
	entry.checkedAt = now

	info, err := os.Stat(path)
	if err != nil {
		return entry.text
	}
	if !entry.modTime.IsZero() &&
		info.ModTime().Equal(entry.modTime) &&
		entry.text != "" {
		return entry.text
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return entry.text
	}
	entry.text = string(raw)
	entry.modTime = info.ModTime()
	return entry.text
}

// resetSuperInstructBridgeCacheForTest clears the package cache (unit tests only).
func resetSuperInstructBridgeCacheForTest() {
	superInstructBridgeMu.Lock()
	defer superInstructBridgeMu.Unlock()
	superInstructBridgeCache = map[string]*superInstructBridgeCacheEntry{}
}

// resolveAccountSuperInstructBridgePath picks bridge.md or profiles/<name>.md.
func resolveAccountSuperInstructBridgePath(account *Account, defaultBridgeFile string) string {
	defaultBridgeFile = strings.TrimSpace(defaultBridgeFile)
	if defaultBridgeFile == "" {
		return ""
	}
	profile := SuperInstructProfileDefault
	if account != nil {
		profile = account.SuperInstructProfile()
	}
	if profile == SuperInstructProfileDefault {
		return defaultBridgeFile
	}
	dir := filepath.Dir(defaultBridgeFile)
	return filepath.Join(dir, "profiles", profile+".md")
}

// applyAccountSuperInstructBridge mutates reqBody["instructions"] when the
// account whitelist is on and bridge text is non-empty.
// Returns true when instructions changed.
func applyAccountSuperInstructBridge(reqBody map[string]any, account *Account, bridgeText string) bool {
	if reqBody == nil || account == nil || !account.IsSuperInstructEnabled() {
		return false
	}
	bridgeText = strings.TrimSpace(bridgeText)
	if bridgeText == "" {
		return false
	}

	existing, _ := reqBody["instructions"].(string)
	existingTrimmed := strings.TrimSpace(existing)
	// Already injected (same process path or client already carried a known marker).
	if instructionsAlreadyInjected(existingTrimmed) {
		return false
	}

	var next string
	switch account.SuperInstructMode() {
	case SuperInstructModeReplace:
		next = bridgeText
	default:
		if existingTrimmed == "" {
			next = bridgeText
		} else {
			next = bridgeText + "\n\n" + existingTrimmed
		}
	}

	if strings.TrimSpace(next) == existingTrimmed {
		return false
	}
	reqBody["instructions"] = next
	return true
}

// applyAccountSuperInstructBridgeFromConfig is the gateway hot-path helper:
// resolve bridge file (default or per-account profile) and apply injection.
func applyAccountSuperInstructBridgeFromConfig(reqBody map[string]any, account *Account, bridgeFile string) bool {
	path := resolveAccountSuperInstructBridgePath(account, bridgeFile)
	return applyAccountSuperInstructBridge(reqBody, account, loadSuperInstructBridgeText(path))
}

// resolveSuperInstructBridgeFile returns the configured bridge path from gateway cfg.
func resolveSuperInstructBridgeFile(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Gateway.SuperInstructBridgeFile)
}
