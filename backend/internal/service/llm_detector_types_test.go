package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDetectorJobStateIsNotVerdict(t *testing.T) {
	for _, state := range []MonitorJobState{MonitorJobQueued, MonitorJobRunning, MonitorJobCancelling, "unknown"} {
		if state.Terminal() {
			t.Fatalf("%s is not terminal", state)
		}
	}
	for _, state := range []MonitorJobState{MonitorJobCompleted, MonitorJobFailed, MonitorJobCancelled, MonitorJobInterrupted, MonitorJobSkipped} {
		if !state.Terminal() {
			t.Fatalf("%s is terminal", state)
		}
	}
	var execution DetectorExecution
	if execution.Verdict == DetectorMatch || execution.Verdict == DetectorMismatch {
		t.Fatal("zero value is not an evaluated verdict")
	}
}

func TestDetectorSnapshotHasNoCredentialInput(t *testing.T) {
	var target DetectorTargetSpec
	if err := DecodeMonitorConfig([]byte(`{"source":"external_api","api_key":"fixture-only"}`), &target); err == nil {
		t.Fatal("persistent target accepted api_key")
	}
	raw, err := json.Marshal(MonitorJobSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"api_key":`, `"authorization":`, `"headers":`} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("snapshot contains credential field %s", key)
		}
	}
}
