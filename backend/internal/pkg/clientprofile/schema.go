package clientprofile

import (
	"bytes"
	"encoding/json"
	"errors"
)

func validateBundleShape(data []byte) error {
	object, err := requireObjectFields(data, []string{"schema_version", "bundle_id", "family", "app_version", "revision", "status", "source", "user_agent", "session_header", "default_verbosity", "evidence"})
	if err != nil {
		return err
	}
	if _, err := requireObjectFields(object["source"], []string{"repository", "ref", "path", "observed_blob", "resolved_commit"}); err != nil {
		return err
	}
	_, err = requireObjectFields(object["evidence"], []string{"application", "wire_capture", "native_transport"})
	return err
}

func requireObjectFields(data []byte, fields []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || len(object) != len(fields) {
		return nil, errors.New("bundle object fields do not match schema")
	}
	for _, name := range fields {
		raw, exists := object[name]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, errors.New("bundle fields must be present, non-null and exactly named")
		}
	}
	return object, nil
}
