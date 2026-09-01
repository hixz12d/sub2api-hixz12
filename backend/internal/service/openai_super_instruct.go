package service

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// Account-level Super-Instruct (bridge.md) injection.
//
// Default is off. Enable per account with extra.super_instruct=true.
// Optional extra.super_instruct_mode: "prepend" (default) | "replace".
// Bridge text is loaded from gateway.super_instruct_bridge_file with mtime hot-reload.
const (
	SuperInstructExtraKey     = "super_instruct"
	SuperInstructModeExtraKey = "super_instruct_mode"
	SuperInstructModePrepend  = "prepend"
	SuperInstructModeReplace  = "replace"

	// superInstructMarker is used to avoid double-injecting the same bridge.
	superInstructMarker = "[Super-Instruct"

	superInstructBridgeReloadMinInterval = 2 * time.Second
)

var (
	superInstructBridgeMu        sync.Mutex
	superInstructBridgePath      string
	superInstructBridgeText      string
	superInstructBridgeModTime   time.Time
	superInstructBridgeCheckedAt time.Time
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

// loadSuperInstructBridgeText reads bridge text from path with a short-interval
// mtime cache. On read errors it keeps the last good content (if any).
func loadSuperInstructBridgeText(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	now := time.Now()
	superInstructBridgeMu.Lock()
	defer superInstructBridgeMu.Unlock()

	if path == superInstructBridgePath &&
		!superInstructBridgeCheckedAt.IsZero() &&
		now.Sub(superInstructBridgeCheckedAt) < superInstructBridgeReloadMinInterval {
		return superInstructBridgeText
	}
	superInstructBridgeCheckedAt = now
	superInstructBridgePath = path

	info, err := os.Stat(path)
	if err != nil {
		return superInstructBridgeText
	}
	if !superInstructBridgeModTime.IsZero() &&
		info.ModTime().Equal(superInstructBridgeModTime) &&
		superInstructBridgeText != "" {
		return superInstructBridgeText
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return superInstructBridgeText
	}
	superInstructBridgeText = string(raw)
	superInstructBridgeModTime = info.ModTime()
	return superInstructBridgeText
}

// resetSuperInstructBridgeCacheForTest clears the package cache (unit tests only).
func resetSuperInstructBridgeCacheForTest() {
	superInstructBridgeMu.Lock()
	defer superInstructBridgeMu.Unlock()
	superInstructBridgePath = ""
	superInstructBridgeText = ""
	superInstructBridgeModTime = time.Time{}
	superInstructBridgeCheckedAt = time.Time{}
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
	// Already injected (same process path or client already carried the marker).
	if strings.Contains(existingTrimmed, superInstructMarker) {
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
// resolve bridge file from config (if any) and apply account whitelist injection.
func applyAccountSuperInstructBridgeFromConfig(reqBody map[string]any, account *Account, bridgeFile string) bool {
	return applyAccountSuperInstructBridge(reqBody, account, loadSuperInstructBridgeText(bridgeFile))
}

// resolveSuperInstructBridgeFile returns the configured bridge path from gateway cfg.
func resolveSuperInstructBridgeFile(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Gateway.SuperInstructBridgeFile)
}
