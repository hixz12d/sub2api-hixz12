//go:build unit

package service

import (
	"testing"
	"unicode/utf8"
)

func TestGenerateChallenge_ShortRandomPrompt(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 40; i++ {
		ch := generateChallenge()
		if ch.Expected == "" {
			t.Fatalf("challenge %d: empty expected answer", i)
		}
		if utf8.RuneCountInString(ch.Prompt) > 16 {
			t.Fatalf("challenge prompt too long (%q)", ch.Prompt)
		}
		got, ok := parseChallengeExpression(ch.Prompt)
		if !ok {
			t.Fatalf("challenge %d: cannot parse expression from %q", i, ch.Prompt)
		}
		if got != ch.Expected {
			t.Fatalf("challenge %d: expected %s, parsed %s from %q", i, ch.Expected, got, ch.Prompt)
		}
		if !validateChallenge(ch.Expected, ch.Expected) {
			t.Fatalf("challenge %d: expected answer should validate", i)
		}
		seen[ch.Prompt] = struct{}{}
	}
	if len(seen) < 8 {
		t.Fatalf("expected prompt variety, got %d unique prompts: %v", len(seen), seen)
	}
}

func TestParseChallengeExpression(t *testing.T) {
	got, ok := parseChallengeExpression("12+7")
	if !ok || got != "19" {
		t.Fatalf("12+7: got %q ok=%v", got, ok)
	}
	got, ok = parseChallengeExpression("9 - 4 = ?")
	if !ok || got != "5" {
		t.Fatalf("9 - 4 = ?: got %q ok=%v", got, ok)
	}
	if _, ok := parseChallengeExpression("no math here"); ok {
		t.Fatal("expected parse failure")
	}
}

func TestValidateInterval_Allows9600(t *testing.T) {
	if err := validateInterval(9600); err != nil {
		t.Fatalf("9600 should be valid, got %v", err)
	}
	if err := validateInterval(15); err != nil {
		t.Fatalf("15 should be valid, got %v", err)
	}
	if err := validateInterval(9601); err == nil {
		t.Fatal("9601 should be invalid")
	}
	if err := validateInterval(14); err == nil {
		t.Fatal("14 should be invalid")
	}
}

func TestValidateJitter_Allows200WithLongInterval(t *testing.T) {
	if err := validateJitter(200, 9600); err != nil {
		t.Fatalf("jitter 200 with interval 9600 should be valid, got %v", err)
	}
	if err := validateJitter(200, 60); err == nil {
		t.Fatal("jitter 200 with interval 60 should be invalid")
	}
}
