package repository

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (c *githubReleaseClientError) FetchClientProfileRelease(context.Context, string, string) (service.ClientReleaseObservation, error) {
	return service.ClientReleaseObservation{}, c.err
}

// All locations originate in the compiled contract. Neither release asset URLs
// nor redirects can choose a new host; no downloaded source is executed.
func (c *githubReleaseClient) clientProfileJSON(ctx context.Context, path, etag string, limit int64, value any) (string, bool, error) {
	if len(etag) > 512 || strings.ContainsAny(etag, "\r\n") {
		return "", false, errors.New("invalid release validator")
	}
	req, err := c.newAPIRequest(ctx, "https://api.github.com/repos/"+path)
	if err != nil {
		return "", false, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotModified && etag != "" {
		return etag, true, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("client release source returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return "", false, errors.New("client release source exceeds size limit or cannot be read")
	}
	if err := json.Unmarshal(body, value); err != nil {
		return "", false, errors.New("invalid client release source JSON")
	}
	validator := response.Header.Get("ETag")
	if len(validator) > 512 || strings.ContainsAny(validator, "\r\n") {
		validator = ""
	}
	return validator, false, nil
}

func (c *githubReleaseClient) clientProfileBlob(ctx context.Context, repo, sha string) ([]byte, error) {
	if !clientprofile.ValidGitObjectID(sha) {
		return nil, errors.New("missing client manifest blob")
	}
	var blob struct{ SHA, Encoding, Content string }
	if _, _, err := c.clientProfileJSON(ctx, repo+"/git/blobs/"+sha, "", 256*1024, &blob); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(blob.Content)
	if err != nil || blob.Encoding != "base64" || blob.SHA != sha || len(data) > 128*1024 {
		return nil, errors.New("invalid client manifest blob")
	}
	hash := sha1.New() // Git object identity, not a signature or trust boundary.
	fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	if hex.EncodeToString(hash.Sum(nil)) != sha {
		return nil, errors.New("client manifest blob digest mismatch")
	}
	return data, nil
}

func (c *githubReleaseClient) FetchClientProfileRelease(ctx context.Context, family, etag string) (service.ClientReleaseObservation, error) {
	result := service.ClientReleaseObservation{}
	contract, err := clientprofile.ClientReleaseContract(family)
	if err != nil {
		return result, err
	}
	var latest service.GitHubRelease
	result.ETag, result.NotModified, err = c.clientProfileJSON(ctx, contract.Repository+"/releases/latest", etag, 2*1024*1024, &latest)
	if err != nil || result.NotModified {
		return result, err
	}
	version := strings.TrimPrefix(latest.TagName, "v")
	if latest.Draft || latest.Prerelease || latest.TagName != "v"+version || !clientprofile.StableClientVersion(version) {
		return result, errors.New("client release is not a stable product version")
	}
	result.LatestVersion = version
	var commit struct{ SHA string }
	if _, _, err = c.clientProfileJSON(ctx, contract.Repository+"/commits/"+latest.TagName, "", 2*1024*1024, &commit); err != nil {
		return result, err
	}
	if !clientprofile.ValidGitObjectID(commit.SHA) {
		return result, errors.New("invalid immutable release commit")
	}
	var tree struct {
		Truncated bool
		Tree      []struct{ Path, SHA, Type, Mode string }
	}
	if _, _, err = c.clientProfileJSON(ctx, contract.Repository+"/git/trees/"+commit.SHA+"?recursive=1", "", 4*1024*1024, &tree); err != nil {
		return result, err
	}
	if tree.Truncated || len(tree.Tree) == 0 {
		return result, errors.New("incomplete client source tree")
	}
	found := make(map[string]string)
	for _, entry := range tree.Tree {
		if !clientprofile.ValidGitObjectID(entry.SHA) {
			return result, errors.New("invalid source object digest")
		}
		if entry.Type == "tree" && entry.Mode == "040000" || entry.Type == "blob" && entry.Mode == "100644" {
			if _, duplicate := found[entry.Path]; duplicate {
				return result, errors.New("duplicate source path")
			}
			found[entry.Path] = entry.SHA
		}
	}
	for path, expected := range contract.Trees {
		if found[path] != expected {
			result.Reason = "source_changed"
			return result, nil
		}
	}
	manifest, err := c.clientProfileBlob(ctx, contract.Repository, found[contract.Manifest])
	if err != nil {
		return result, err
	}
	root, err := c.clientProfileBlob(ctx, contract.Repository, found["package.json"])
	if err != nil {
		return result, err
	}
	var pkg struct{ Version string }
	if json.Unmarshal(manifest, &pkg) != nil || pkg.Version != version {
		result.Reason = "package_version_mismatch"
		return result, nil
	}
	digest, err := clientprofile.ClientDependencyDigest(manifest, root)
	if err != nil {
		return result, err
	}
	if digest != contract.DependencyDigest {
		result.Reason = "dependencies_changed"
		return result, nil
	}
	release := &clientprofile.CompatibleRelease{Family: family, Version: version, Tag: latest.TagName, Commit: commit.SHA, ContractDigest: contract.Digest()}
	if err := release.Validate(); err != nil {
		return result, err
	}
	result.Release = release
	return result, nil
}
