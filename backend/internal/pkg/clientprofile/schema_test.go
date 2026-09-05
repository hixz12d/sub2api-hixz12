package clientprofile

import (
	"bytes"
	"testing"
)

func TestCandidateSchemaRequiresExactNonNullFields(t *testing.T) {
	data, err := candidates.ReadFile("profiles/opencode-1.2.4-oauth-sse-r1.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string][]byte{
		"missing":        bytes.Replace(data, []byte(`"default_verbosity": "",`), nil, 1),
		"null":           bytes.Replace(data, []byte(`"default_verbosity": ""`), []byte(`"default_verbosity": null`), 1),
		"case":           bytes.Replace(data, []byte(`"family"`), []byte(`"FAMILY"`), 1),
		"case-duplicate": bytes.Replace(data, []byte(`"family":`), []byte(`"FAMILY": "pi", "family":`), 1),
		"source-null":    bytes.Replace(data, []byte(`"resolved_commit": ""`), []byte(`"resolved_commit": null`), 1),
		"source-missing": bytes.Replace(data, []byte(`"observed_blob": "56931b2ed62cd4f7d077ddea74cb406bef0d8b72",`), nil, 1),
		"evidence-case":  bytes.Replace(data, []byte(`"application"`), []byte(`"Application"`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Decode(input); err == nil {
				t.Fatal("ambiguous schema accepted")
			}
		})
	}
}
