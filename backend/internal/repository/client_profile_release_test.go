package repository

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	"github.com/stretchr/testify/require"
)

type clientReleaseRoundTripper func(*http.Request) (*http.Response, error)

func (f clientReleaseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Public declarations captured from the reviewed v0.57.1 manifests. Only the
// package version changes in the simulated compatible release.
const piReleaseManifestFixture = `{"version":"0.57.2","dependencies":{"@anthropic-ai/sdk":"^0.73.0","@aws-sdk/client-bedrock-runtime":"^3.983.0","@google/genai":"^1.40.0","@mistralai/mistralai":"1.14.1","@sinclair/typebox":"^0.34.41","ajv":"^8.17.1","ajv-formats":"^3.0.1","chalk":"^5.6.2","openai":"6.26.0","partial-json":"^0.1.7","proxy-agent":"^6.5.0","undici":"^7.19.1","zod-to-json-schema":"^3.24.6"}}`
const piRootManifestFixture = `{"dependencies":{"@mariozechner/jiti":"^2.6.5","@mariozechner/pi-coding-agent":"^0.30.2","get-east-asian-width":"^1.4.0"},"overrides":{"fast-xml-parser":"5.3.8","gaxios":{"rimraf":"6.1.2"},"rimraf":"6.1.2"},"workspaces":["packages/*","packages/web-ui/example","packages/coding-agent/examples/extensions/with-deps","packages/coding-agent/examples/extensions/custom-provider-anthropic","packages/coding-agent/examples/extensions/custom-provider-gitlab-duo","packages/coding-agent/examples/extensions/custom-provider-qwen-cli"]}`

func TestClientReleaseSourceVerification(t *testing.T) {
	contract, err := clientprofile.ClientReleaseContract("pi")
	require.NoError(t, err)
	digest, err := clientprofile.ClientDependencyDigest([]byte(piReleaseManifestFixture), []byte(piRootManifestFixture))
	require.NoError(t, err)
	require.Equal(t, contract.DependencyDigest, digest, "reviewed dependency fingerprint must be reproducible")
	for _, scenario := range []string{"compatible", "source_changed", "dependencies_changed", "prerelease", "truncated", "304", "redirect", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := piReleaseManifestFixture
			if scenario == "dependencies_changed" {
				manifest = strings.ReplaceAll(manifest, "6.26.0", "6.27.0")
			}
			blobs := map[string]string{}
			blob := func(data string) string {
				h := sha1.New()
				fmt.Fprintf(h, "blob %d\x00", len(data))
				_, _ = h.Write([]byte(data))
				sha := hex.EncodeToString(h.Sum(nil))
				blobs[sha] = data
				return sha
			}
			tree := []map[string]string{
				{"path": contract.Manifest, "type": "blob", "mode": "100644", "sha": blob(manifest)},
				{"path": "package.json", "type": "blob", "mode": "100644", "sha": blob(piRootManifestFixture)},
			}
			for path, sha := range contract.Trees {
				if scenario == "source_changed" {
					sha = strings.Repeat("a", 40)
				}
				tree = append(tree, map[string]string{"path": path, "sha": sha, "type": "tree", "mode": "040000"})
			}
			calls := 0
			client := &githubReleaseClient{httpClient: &http.Client{Transport: clientReleaseRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "https", request.URL.Scheme)
				require.Equal(t, "api.github.com", request.URL.Host)
				require.True(t, strings.HasPrefix(request.URL.Path, "/repos/earendil-works/pi/"))
				status, headers := http.StatusOK, http.Header{}
				headers.Set("ETag", `"new"`)
				var value any
				switch {
				case strings.HasSuffix(request.URL.Path, "/releases/latest"):
					value = map[string]any{"tag_name": "v0.57.2", "draft": false, "prerelease": scenario == "prerelease"}
					if scenario == "304" {
						status = 304
						require.Equal(t, `"old"`, request.Header.Get("If-None-Match"))
					}
					if scenario == "redirect" {
						status = 302
						headers.Set("Location", "https://example.invalid/source")
					}
					if scenario == "oversized" {
						return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 2*1024*1024+1)))}, nil
					}
				case strings.Contains(request.URL.Path, "/commits/"):
					value = map[string]string{"sha": contract.BaselineCommit}
				case strings.Contains(request.URL.Path, "/git/trees/"):
					require.True(t, strings.HasSuffix(request.URL.Path, contract.BaselineCommit))
					value = map[string]any{"tree": tree, "truncated": scenario == "truncated"}
				case strings.Contains(request.URL.Path, "/git/blobs/"):
					sha := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
					value = map[string]string{"sha": sha, "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(blobs[sha]))}
				default:
					t.Fatalf("unexpected source URL: %s", request.URL)
				}
				data, err := json.Marshal(value)
				require.NoError(t, err)
				return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})}}
			etag := ""
			if scenario == "304" {
				etag = `"old"`
			}
			result, err := client.FetchClientProfileRelease(context.Background(), "pi", etag)
			switch scenario {
			case "compatible":
				require.NoError(t, err)
				require.NotNil(t, result.Release)
				require.Equal(t, `"new"`, result.ETag)
				require.Equal(t, "0.57.2", result.Release.Version)
				require.NoError(t, result.Release.Validate())
			case "source_changed", "dependencies_changed":
				require.NoError(t, err)
				require.Nil(t, result.Release)
				require.Equal(t, scenario, result.Reason)
			case "304":
				require.NoError(t, err)
				require.True(t, result.NotModified)
				require.Nil(t, result.Release)
				require.Equal(t, 1, calls)
			default:
				require.Error(t, err)
				require.Nil(t, result.Release)
			}
		})
	}
}
