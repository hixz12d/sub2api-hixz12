package clientprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestCandidateArtifacts(t *testing.T) {
	for _, id := range []string{"pi-0.57.1-oauth-sse-r1", "opencode-1.2.4-oauth-sse-r1"} {
		t.Run(id, func(t *testing.T) {
			bundle, digest, err := LoadCandidate(id)
			if err != nil {
				t.Fatal(err)
			}
			data, err := candidates.ReadFile("profiles/" + id + ".json")
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if digest != hex.EncodeToString(hash[:]) {
				t.Fatal("digest is not artifact SHA-256")
			}
			bundle.UserAgent = "mutated"
			fresh, _, err := LoadCandidate(id)
			if err != nil || fresh.UserAgent == bundle.UserAgent {
				t.Fatal("catalog was mutated")
			}
		})
	}
	for _, id := range []string{"pi", "latest", "../../secrets", "pi-0.84.3-oauth-sse-r1"} {
		if _, _, err := LoadCandidate(id); err == nil {
			t.Fatalf("accepted unknown id %q", id)
		}
	}
}

func TestDecodeRejectsInvalidBundles(t *testing.T) {
	data, err := candidates.ReadFile("profiles/pi-0.57.1-oauth-sse-r1.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"unknown":       bytes.Replace(data, []byte(`"family":`), []byte(`"execute": "anything", "family":`), 1),
		"duplicate":     bytes.Replace(data, []byte(`"family":`), []byte(`"family": "pi", "family":`), 1),
		"trailing":      append(bytes.Clone(data), []byte(` {}`)...),
		"size":          bytes.Repeat([]byte(" "), maxBundleBytes+1),
		"deep":          []byte(strings.Repeat("[", 10) + strings.Repeat("]", 10)),
		"crlf":          bytes.Replace(data, []byte(`pi (win32`), []byte(`pi\r\n (win32`), 1),
		"family":        bytes.Replace(data, []byte(`"family": "pi"`), []byte(`"family": "codex_exec"`), 1),
		"approval":      bytes.Replace(data, []byte(`"candidate"`), []byte(`"approved"`), 1),
		"wrong-version": bytes.Replace(data, []byte(`"app_version": "0.57.1"`), []byte(`"app_version": "0.84.3"`), 1),
		"wrong-header":  bytes.Replace(data, []byte(`"session_id"`), []byte(`"session-id"`), 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Decode(input); err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}

func TestCandidateResponsesPreserveCallerSemantics(t *testing.T) {
	body := []byte(`{"instructions":"caller policy","tools":[{"type":"function","name":"local_tool","parameters":{"type":"object"}}],"input":[{"type":"function_call","call_id":"call_1","name":"local_tool","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}],"text":{"verbosity":"high","format":{"type":"json_object"}},"reasoning":{"effort":"low"},"previous_response_id":"owned_response","prompt_cache_key":"caller-cache","store":false,"stream":true}`)
	headers := http.Header{"session-id": {"inbound"}, "session_id": {"inbound"}, "x-codex-installation-id": {"not-for-pi"}, "user-agent": {"old"}, "originator": {"old"}, "Authorization": {"Bearer local-test"}, "X-Client-Request-Id": {"old"}, "Thread-Id": {"old"}, "Version": {"old"}}
	beforeHeaders := headers.Clone()
	beforeBody := bytes.Clone(body)
	var original map[string]json.RawMessage
	if err := json.Unmarshal(body, &original); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pi-0.57.1-oauth-sse-r1", "opencode-1.2.4-oauth-sse-r1"} {
		t.Run(id, func(t *testing.T) {
			b, _, err := LoadCandidate(id)
			if err != nil {
				t.Fatal(err)
			}
			out, wire, err := b.AdaptResponses(headers, body, "scoped-session")
			if err != nil {
				t.Fatal(err)
			}
			if out.Get("session_id") != "scoped-session" || out.Get("User-Agent") != b.UserAgent || out.Get("originator") != b.Family {
				t.Fatal("incorrect application headers")
			}
			for name := range out {
				lower := strings.ToLower(name)
				if strings.HasPrefix(lower, "x-codex-") || lower == "session-id" || lower == "thread-id" || lower == "x-client-request-id" || lower == "version" {
					t.Fatalf("unexpected header %s", name)
				}
			}
			if _, duplicate := out["user-agent"]; duplicate {
				t.Fatal("noncanonical duplicate UA survived")
			}
			if out.Get("Authorization") != headers.Get("Authorization") {
				t.Fatal("authentication mutated")
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(wire, &result); err != nil {
				t.Fatal(err)
			}
			for key, value := range original {
				if key == "prompt_cache_key" && b.Family == "pi" {
					continue
				}
				var want, got any
				if err := json.Unmarshal(value, &want); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(result[key], &got); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("caller field changed: %s", key)
				}
			}
			if b.Family == "pi" && string(result["prompt_cache_key"]) != `"scoped-session"` {
				t.Fatal("Pi cache/session mismatch")
			}
			if _, invented := result["client_metadata"]; invented {
				t.Fatal("Codex metadata injected")
			}
		})
	}
	if !reflect.DeepEqual(headers, beforeHeaders) || !bytes.Equal(body, beforeBody) {
		t.Fatal("input was mutated")
	}
}

func TestPiDefaultAndCandidateRejections(t *testing.T) {
	b, _, err := LoadCandidate("pi-0.57.1-oauth-sse-r1")
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := b.AdaptResponses(nil, []byte(`{"input":[]}`), "session")
	if err != nil || !bytes.Contains(body, []byte(`"verbosity":"medium"`)) {
		t.Fatalf("missing version-specific default: %s %v", body, err)
	}
	for _, input := range []string{`null`, `[]`, `{`, `{"text":null}`, `{"text":"low"}`, `{"client_metadata":{}}`, `{"conversation_id":"foreign"}`, `{"input":[],"input":[1]}`} {
		if _, _, err := b.AdaptResponses(nil, []byte(input), "session"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	for _, session := range []string{"", " ", "a\r\nb", "a\x00b"} {
		if _, _, err := b.AdaptResponses(nil, []byte(`{}`), session); err == nil {
			t.Fatal("invalid scoped session accepted")
		}
	}
	for _, h := range []http.Header{{"Session-Id": {"a"}, "Session_id": {"b"}}, {"SESSION-ID": {"a", "b"}}, {"session_id": {""}}} {
		if _, _, err := b.AdaptResponses(h, []byte(`{}`), "session"); err == nil {
			t.Fatal("ambiguous inbound session accepted")
		}
	}
}
