//go:build unit

package service

import (
	"strings"
	"time"
)

func cnProviderResponseIndicatesInsufficientBalance(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	s := strings.ToLower(string(body))
	return strings.Contains(s, "\u4f59\u989d\u4e0d\u8db3") ||
		strings.Contains(s, "insufficient balance") ||
		strings.Contains(s, "insufficient_credit") ||
		strings.Contains(s, "balance is not enough") ||
		strings.Contains(s, "no enough balance")
}

func cnProviderQuotaSnapshotReset(account *Account, now time.Time) *time.Time {
	if account == nil || !account.IsCNProvider() || !account.IsCodingPlan() || len(account.Extra) == 0 {
		return nil
	}
	provider := account.Platform
	var earliest *time.Time
	for _, suffix := range []string{cnExtraSuffix5hReset, cnExtraSuffixWeeklyReset} {
		t := parseSchedulingResetAt(account.Extra[cnExtraKey(provider, suffix)])
		if t == nil || !t.After(now) {
			continue
		}
		if earliest == nil || t.Before(*earliest) {
			earliest = t
		}
	}
	return earliest
}
