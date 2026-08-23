package service

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const codexPlatformPersonaSeedPrefix = "sub2api:codex-platform-persona:v1:"

// codexPlatformPersona is one official-looking Codex client shell. OS, arch and
// terminal are bound together so we never emit iTerm on Windows or
// WindowsTerminal on macOS. The compiled Ubuntu/xterm default remains the
// no-account fallback; OAuth accounts without an explicit UA pick one of these
// personas from a durable account seed.
type codexPlatformPersona struct {
	Originator string
	Platform   string
	Terminal   string
	Trailer    bool
}

func (p codexPlatformPersona) UserAgent(version string) string {
	version = strings.TrimSpace(version)
	if p.Originator == "" || p.Platform == "" || p.Terminal == "" || version == "" {
		return ""
	}
	ua := p.Originator + "/" + version + " (" + p.Platform + ") " + p.Terminal
	if p.Trailer {
		ua += " (" + p.Originator + "; " + version + ")"
	}
	return ua
}

var codexPlatformPersonaPool = buildCodexPlatformPersonaPool()

func buildCodexPlatformPersonaPool() []codexPlatformPersona {
	desktopSkins := []codexPlatformPersona{
		{Originator: openai.CodexCLIOriginator, Terminal: "electron", Trailer: false},
		{Originator: "codex_vscode", Terminal: "vscode", Trailer: true},
	}

	windowsPlatforms := []string{
		"Windows NT 10.0; Win64; x64",
		"Windows NT 10.0; x86_64",
		"Windows NT 10.0; ARM64",
		"Windows 10; x86_64",
		"Windows 11; x86_64",
		"Windows 11; Win64; x64",
		"Windows 11; arm64",
	}
	macVersions := []string{
		"13.6.7", "13.6.9", "14.0", "14.4.1", "14.5", "14.6.1",
		"14.7.1", "15.0", "15.1.0", "15.2", "15.3.1", "15.4",
	}
	macArches := []string{"arm64", "x86_64"}
	linuxPlatforms := []string{
		"Ubuntu 20.4.0; x86_64",
		"Ubuntu 22.4.0; x86_64",
		"Ubuntu 22.4.0; aarch64",
		"Ubuntu 22.10.0; x86_64",
		"Ubuntu 23.4.0; x86_64",
		"Ubuntu 23.10.0; x86_64",
		"Ubuntu 24.4.0; x86_64",
		"Ubuntu 24.4.0; aarch64",
		"Ubuntu 24.10.0; x86_64",
	}

	pool := make([]codexPlatformPersona, 0, 96)
	add := func(originator, platform, terminal string, trailer bool) {
		pool = append(pool, codexPlatformPersona{
			Originator: originator,
			Platform:   platform,
			Terminal:   terminal,
			Trailer:    trailer,
		})
	}
	addSkins := func(platform string, skins []codexPlatformPersona) {
		for _, skin := range skins {
			add(skin.Originator, platform, skin.Terminal, skin.Trailer)
		}
	}

	for _, platform := range windowsPlatforms {
		addSkins(platform, desktopSkins)
	}
	for _, version := range macVersions {
		for _, arch := range macArches {
			addSkins("Mac OS X "+version+"; "+arch, desktopSkins)
		}
	}
	for _, platform := range linuxPlatforms {
		addSkins(platform, desktopSkins)
	}

	// A smaller official TUI set keeps some accounts from looking like every
	// other Electron/VS Code install, without inventing cross-OS terminals.
	add("codex-tui", "Windows NT 10.0; Win64; x64", "WindowsTerminal", true)
	add("codex-tui", "Windows 11; x86_64", "WindowsTerminal", true)
	add("codex-tui", "Mac OS X 14.0; arm64", "iTerm", true)
	add("codex-tui", "Mac OS X 14.6.1; arm64", "iTerm", true)
	add("codex-tui", "Mac OS X 15.1.0; arm64", "iTerm.app", true)
	add("codex-tui", "Mac OS X 15.1.0; x86_64", "iTerm.app", true)
	add("codex-tui", "Ubuntu 22.4.0; x86_64", "xterm-256color", true)
	add("codex-tui", "Ubuntu 24.4.0; x86_64", "xterm-256color", true)
	add("codex-tui", "Ubuntu 24.4.0; aarch64", "xterm-256color", true)
	return pool
}

func selectAccountStableCodexPersona(account *Account) (codexPlatformPersona, bool) {
	if account == nil || !account.IsOpenAIOAuth() || len(codexPlatformPersonaPool) == 0 {
		return codexPlatformPersona{}, false
	}
	seed, _ := fingerprintDerivationSeed(account)
	if strings.TrimSpace(seed) == "" {
		return codexPlatformPersona{}, false
	}
	sum := sha256.Sum256([]byte(codexPlatformPersonaSeedPrefix + seed))
	idx := binary.BigEndian.Uint64(sum[:8]) % uint64(len(codexPlatformPersonaPool))
	return codexPlatformPersonaPool[idx], true
}

func buildAccountStableCodexUserAgent(account *Account, version string) string {
	persona, ok := selectAccountStableCodexPersona(account)
	if !ok {
		return ""
	}
	version = NormalizeCodexClientVersion(version)
	if version == "" {
		version = codexCLIVersion
	}
	ua := persona.UserAgent(version)
	if _, _, ok := openai.PairCodexClientIdentity(ua); !ok {
		return ""
	}
	return ua
}
