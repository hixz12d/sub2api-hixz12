package clientprofile

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This exercises the candidate constructor plus Go's normal HTTP sender, not
// the production Relay Kernel/uTLS/plugin dispatch or a real OAuth upstream.
func TestCandidateLocalHTTPReceiver(t *testing.T) {
	for _, id := range []string{"pi-0.57.1-oauth-sse-r1", "opencode-1.2.4-oauth-sse-r1"} {
		t.Run(id, func(t *testing.T) {
			b, _, err := LoadCandidate(id)
			if err != nil {
				t.Fatal(err)
			}
			type received struct {
				headers http.Header
				body    []byte
				err     error
			}
			wire := make(chan received, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				wire <- received{r.Header.Clone(), data, err}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
			}))
			defer server.Close()
			headers, body, err := b.AdaptResponses(http.Header{"Content-Type": {"application/json"}}, []byte(`{"input":[],"instructions":"local policy","tools":[],"stream":true}`), "account-A-conversation-1")
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header = headers
			client := server.Client()
			client.Timeout = 5 * time.Second
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, readErr := io.Copy(io.Discard, response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
				t.Fatal("local HTTP response failed")
			}
			got := <-wire
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.headers.Get("session_id") != "account-A-conversation-1" || got.headers.Get("session-id") != "" {
				t.Fatal("incorrect session header on wire")
			}
			if got.headers.Get("User-Agent") != b.UserAgent || got.headers.Get("originator") != b.Family {
				t.Fatal("incorrect provider tuple on wire")
			}
			if !bytes.Equal(got.body, body) {
				t.Fatal("HTTP sender changed adapter body")
			}
			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(got.body, &decoded); err != nil {
				t.Fatal(err)
			}
			if string(decoded["instructions"]) != `"local policy"` || string(decoded["tools"]) != `[]` {
				t.Fatal("caller semantics lost on wire")
			}
		})
	}
}
