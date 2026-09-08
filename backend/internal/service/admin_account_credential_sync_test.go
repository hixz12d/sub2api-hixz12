package service

import "testing"

func TestIsRecognizedAuthError(t *testing.T) {
	if !isRecognizedAuthError(StatusError, "oauth 401 unauthorized") {
		t.Fatal("expected auth error")
	}
	if isRecognizedAuthError(StatusError, "") {
		t.Fatal("empty message must not qualify")
	}
	if isRecognizedAuthError(StatusActive, "401") {
		t.Fatal("active status must not qualify")
	}
	if isRecognizedAuthError(StatusError, "rate limit exceeded") {
		t.Fatal("rate limit alone must not qualify as auth error")
	}
}

func TestFilterOAuthCredentialPatchKeepsRefresh(t *testing.T) {
	out := filterOAuthCredentialPatch(map[string]any{
		"access_token":  "a",
		"refresh_token": "",
		"group_ids":     []int{1},
		"priority":      9,
	})
	if _, ok := out["refresh_token"]; ok {
		t.Fatal("empty refresh_token must not wipe")
	}
	if out["access_token"] != "a" {
		t.Fatal("access_token missing")
	}
	if _, ok := out["priority"]; ok {
		t.Fatal("non-token fields must be rejected")
	}
}

func TestIdentityMatches(t *testing.T) {
	account := &Account{Credentials: map[string]any{"email": "a@example.com", "workspace_id": "ws-1"}}
	if err := identityMatches(account, map[string]any{"email": "a@example.com", "workspace_id": "ws-1"}); err != nil {
		t.Fatal(err)
	}
	if err := identityMatches(account, map[string]any{"email": "b@example.com"}); err == nil {
		t.Fatal("expected email mismatch")
	}
}
