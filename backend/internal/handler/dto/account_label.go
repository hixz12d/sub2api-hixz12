package dto

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	teamOwnerPattern = regexp.MustCompile(`(?i)^team\s*[(（]\s*([^()（）\r\n]+)\s*[)）]`)
	proOwnerPattern  = regexp.MustCompile(`(?i)^pro(?:\s+|\s*[(（]\s*)([^\s()（）]+)`)
)

// userAccountLabel exposes only the owner alias from the supported naming
// convention. Never fall back to the full name, credentials, notes or email.
func userAccountLabel(account *service.Account) string {
	if account == nil {
		return ""
	}
	name := strings.TrimSpace(account.Name)
	isTeam := true
	match := teamOwnerPattern.FindStringSubmatch(name)
	if match == nil {
		isTeam = false
		match = proOwnerPattern.FindStringSubmatch(name)
	}
	if match == nil {
		return ""
	}
	owner := strings.TrimSpace(match[1])
	// An email may be used as the alias, but its domain is never public.
	owner, _, _ = strings.Cut(owner, "@")
	if owner == "" || strings.ContainsFunc(owner, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' && r != '-'
	}) {
		return ""
	}
	runes := []rune(owner)
	if !isTeam {
		if len(runes) > 6 {
			runes = runes[:6]
		}
		return string(runes)
	}
	if len(runes) <= 5 {
		return string(runes[:1]) + "**"
	}
	return string(runes[:3]) + "**" + string(runes[len(runes)-2:])
}
