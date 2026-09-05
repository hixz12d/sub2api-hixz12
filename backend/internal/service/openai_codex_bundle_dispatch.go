package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
)

const (
	CodexProfilePiBundle       = "pi-0.57.1-oauth-sse-r1"
	CodexProfileOpenCodeBundle = "opencode-1.2.4-oauth-sse-r1"
)

func isCodexBundleProfile(id string) bool {
	return id == CodexProfilePiBundle || id == CodexProfileOpenCodeBundle
}

func resolveCodexBundleProfile(id string) (CodexClientProfile, error) {
	bundle, digest, err := clientprofile.LoadCandidate(id)
	if err != nil {
		return CodexClientProfile{}, err
	}
	return CodexClientProfile{
		ID: id, Revision: bundle.Revision, BundleID: id, BundleDigest: digest,
		App:          CodexAppIdentityProfile{UserAgent: bundle.UserAgent, Originator: bundle.Family},
		Transport:    CodexTransportProfile{TLSProfileID: "native-go", HTTP2ProfileID: "native-go", HeaderOrder: []string{"authorization", "content-type", "accept", "user-agent", "originator", "session_id"}},
		Capabilities: CodexCapabilityResponses | CodexCapabilityResume | CodexCapabilityHTTP,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "shared application bundle; native TLS/HTTP2 parity remains unverified",
	}, nil
}

func validateCodexBundleProfile(profile CodexClientProfile) error {
	expected, err := resolveCodexBundleProfile(profile.BundleID)
	if err != nil {
		return err
	}
	expected.FidelityNote = profile.FidelityNote // Descriptive text is not the wire contract.
	if !reflect.DeepEqual(profile, expected) {
		return errors.New("pinned client bundle differs from packaged artifact")
	}
	return nil
}

// This runs at the common HTTP dispatch boundary, after every request builder
// and before either plugin dispatch or the built-in sender. Never consume a
// non-replayable body or silently remove caller-owned metadata.
func applyCodexBundleAtDispatch(request *http.Request) error {
	state, ok := CodexAttemptStateFromContext(request.Context())
	if !ok || state.Profile().BundleID == "" {
		return nil
	}
	profile := state.Profile()
	if err := validateCodexBundleProfile(profile); err != nil {
		return err
	}
	if state.Identity() == nil {
		return errors.New("client bundle requires managed session identity")
	}
	plan, ok := CodexRequestPlanFromContext(request.Context())
	if !ok {
		return errors.New("client bundle requires original request plan")
	}
	if plan.Transport() != CodexTransportHTTP || plan.Operation() == CodexOperationCompact {
		return errors.New("client bundle supports Responses HTTP/SSE only")
	}
	bundle, _, err := clientprofile.LoadCandidate(profile.BundleID)
	if err != nil {
		return err
	}
	session := state.Identity().SessionID()
	// Validate the original input before removing metadata injected by the gateway.
	if _, _, err := bundle.AdaptResponses(plan.InboundHeaders(), plan.Body(), session); err != nil {
		return err
	}
	if request.GetBody == nil {
		return errors.New("client bundle requires replayable JSON body")
	}
	reader, err := request.GetBody()
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return errors.New("invalid final bundle body")
	}
	delete(object, "client_metadata")
	delete(object, "conversation_id")
	if bundle.Family == "opencode" {
		var original map[string]json.RawMessage
		if err := json.Unmarshal(plan.Body(), &original); err != nil {
			return err
		}
		if cacheKey, exists := original["prompt_cache_key"]; exists {
			object["prompt_cache_key"] = cacheKey
		} else {
			delete(object, "prompt_cache_key")
		}
	}
	body, err = json.Marshal(object)
	if err != nil {
		return err
	}
	headers, body, err := bundle.AdaptResponses(request.Header, body, session)
	if err != nil {
		return err
	}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "text/event-stream")
	headers.Del("Content-Length")
	if request.Body != nil {
		_ = request.Body.Close()
	}
	request.Header = headers
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	request.ContentLength = int64(len(body))
	request.TransferEncoding = nil
	state = state.WithFinalHTTPWire(buildCodexAttemptIdentityHeaders(profile, nil, headers), body)
	*request = *request.WithContext(ContextWithCodexAttemptState(request.Context(), state))
	return nil
}
