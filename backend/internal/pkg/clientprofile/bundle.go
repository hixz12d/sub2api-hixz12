// Package clientprofile contains offline candidate wire contracts. It is not
// connected to the production Relay Kernel until conversation bundle pinning exists.
package clientprofile

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

//go:embed profiles/*.json
var candidates embed.FS

const maxBundleBytes = 16 * 1024

type Source struct {
	Repository     string `json:"repository"`
	Ref            string `json:"ref"`
	Path           string `json:"path"`
	ObservedBlob   string `json:"observed_blob"`
	ResolvedCommit string `json:"resolved_commit"`
}

type Evidence struct {
	Application     string `json:"application"`
	WireCapture     string `json:"wire_capture"`
	NativeTransport string `json:"native_transport"`
}

type Bundle struct {
	SchemaVersion    int      `json:"schema_version"`
	ID               string   `json:"bundle_id"`
	Family           string   `json:"family"`
	AppVersion       string   `json:"app_version"`
	Revision         int      `json:"revision"`
	Status           string   `json:"status"`
	Source           Source   `json:"source"`
	UserAgent        string   `json:"user_agent"`
	SessionHeader    string   `json:"session_header"`
	DefaultVerbosity string   `json:"default_verbosity"`
	Evidence         Evidence `json:"evidence"`
}

// LoadCandidate accepts only packaged names, never paths or remote URLs. Each
// call returns a value copy and the SHA-256 of the exact embedded artifact bytes.
func LoadCandidate(id string) (Bundle, string, error) {
	switch id {
	case "pi-0.57.1-oauth-sse-r1", "opencode-1.2.4-oauth-sse-r1":
	default:
		return Bundle{}, "", errors.New("unknown candidate bundle")
	}
	data, err := candidates.ReadFile("profiles/" + id + ".json")
	if err != nil {
		return Bundle{}, "", err
	}
	bundle, digest, err := Decode(data)
	if err == nil && bundle.ID != id {
		return Bundle{}, "", errors.New("bundle filename and id differ")
	}
	return bundle, digest, err
}

func Decode(data []byte) (Bundle, string, error) {
	if len(data) == 0 || len(data) > maxBundleBytes {
		return Bundle{}, "", errors.New("invalid bundle size")
	}
	if err := validateJSON(data, 8); err != nil {
		return Bundle{}, "", err
	}
	if err := validateBundleShape(data); err != nil {
		return Bundle{}, "", err
	}
	var b Bundle
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		return Bundle{}, "", errors.New("invalid bundle fields")
	}
	if err := b.Validate(); err != nil {
		return Bundle{}, "", err
	}
	digest := sha256.Sum256(data)
	return b, hex.EncodeToString(digest[:]), nil
}

func (b Bundle) Validate() error {
	if b.SchemaVersion != 1 || b.Revision != 1 || b.Status != "candidate" {
		return errors.New("unsupported bundle schema, revision or approval state")
	}
	if b.Evidence.Application != "blueprint_source_reference" || b.Evidence.WireCapture != "pending" || b.Evidence.NativeTransport != "unverified" || b.Source.ResolvedCommit != "" {
		return errors.New("candidate evidence must not claim unverified approval")
	}
	expected := Bundle{}
	switch b.ID {
	case "pi-0.57.1-oauth-sse-r1":
		expected.Family, expected.AppVersion = "pi", "0.57.1"
		expected.UserAgent, expected.DefaultVerbosity = "pi (win32 10.0.26200; x64)", "medium"
		expected.Source = Source{"earendil-works/pi", "v0.57.1", "packages/ai/src/providers/openai-codex-responses.ts", "8c9b8aae5f66ce9ac4e96afc0ca84dcc1075adc3", ""}
	case "opencode-1.2.4-oauth-sse-r1":
		expected.Family, expected.AppVersion = "opencode", "1.2.4"
		expected.UserAgent = "opencode/1.2.4 (win32 10.0.26200; x64)"
		expected.Source = Source{"anomalyco/opencode", "v1.2.4", "packages/opencode/src/plugin/codex.ts", "56931b2ed62cd4f7d077ddea74cb406bef0d8b72", ""}
	default:
		return errors.New("unknown candidate bundle")
	}
	if b.Family != expected.Family || b.AppVersion != expected.AppVersion || b.UserAgent != expected.UserAgent || b.Source != expected.Source || b.DefaultVerbosity != expected.DefaultVerbosity || b.SessionHeader != "session_id" {
		return errors.New("bundle differs from its reviewed candidate contract")
	}
	return nil
}

// AdaptResponses constructs only the candidate OAuth HTTP/SSE application layer.
// The caller supplies an authenticated, account/conversation-scoped session, not
// a raw public header. Transport/authentication remain the caller's responsibility.
func (b Bundle) AdaptResponses(headers http.Header, body []byte, session string) (http.Header, []byte, error) {
	if err := b.Validate(); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(session) == "" || strings.ContainsAny(session, "\r\n\x00") {
		return nil, nil, errors.New("invalid scoped session")
	}
	var existing string
	for name, values := range headers {
		if !strings.EqualFold(name, "session-id") && !strings.EqualFold(name, "session_id") {
			continue
		}
		for _, value := range values {
			if existing != "" && value != existing {
				return nil, nil, errors.New("session_header_conflict")
			}
			if value == "" {
				return nil, nil, errors.New("empty session header")
			}
			existing = value
		}
	}
	if err := validateJSON(body, 64); err != nil {
		return nil, nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return nil, nil, errors.New("request body must be an object")
	}
	// Refuse Codex-only metadata instead of deleting user data silently.
	for _, key := range []string{"client_metadata", "conversation_id"} {
		if _, exists := object[key]; exists {
			return nil, nil, fmt.Errorf("unsupported candidate body field: %s", key)
		}
	}
	if b.Family == "pi" {
		object["prompt_cache_key"], _ = json.Marshal(session)
	}
	if b.DefaultVerbosity != "" {
		text := map[string]json.RawMessage{}
		if raw, ok := object["text"]; ok {
			if err := json.Unmarshal(raw, &text); err != nil || text == nil {
				return nil, nil, errors.New("text must be an object")
			}
		}
		if _, explicit := text["verbosity"]; !explicit {
			text["verbosity"], _ = json.Marshal(b.DefaultVerbosity)
		}
		object["text"], _ = json.Marshal(text)
	}
	result, err := json.Marshal(object)
	if err != nil {
		return nil, nil, err
	}
	out := headers.Clone()
	if out == nil {
		out = make(http.Header)
	}
	for name := range out {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-codex-") || lower == "user-agent" || lower == "originator" || lower == "session-id" || lower == "session_id" || lower == "thread-id" || lower == "conversation_id" || lower == "x-client-request-id" || lower == "x-openai-client-version" || lower == "version" || lower == "openai-beta" {
			delete(out, name)
		}
	}
	out.Set("User-Agent", b.UserAgent)
	out.Set("originator", b.Family)
	out.Set(b.SessionHeader, session)
	return out, result, nil
}

// Walk tokens before decoding so duplicate keys and deeply nested unknown
// values cannot silently disappear through encoding/json's last-value behavior.
func validateJSON(data []byte, maxDepth int) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > maxDepth {
			return errors.New("JSON nesting limit exceeded")
		}
		token, err := d.Token()
		if err != nil {
			return errors.New("invalid JSON")
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return errors.New("invalid JSON object")
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate or invalid JSON key")
				}
				seen[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}
