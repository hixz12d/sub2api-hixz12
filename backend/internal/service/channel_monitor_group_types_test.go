package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func testInactiveGroupPolicy() ChannelMonitorGroupPolicy {
	return ChannelMonitorGroupPolicy{GroupID: 1, PrimaryModel: "model-a", ProbeConfig: DefaultGroupProbeConfig(), CapabilityConfig: DefaultGroupCapabilityConfig()}
}

func TestMonitorGroupDefaultsAndNormalization(t *testing.T) {
	p := testInactiveGroupPolicy()
	p.PrimaryModel = " model-a "
	p.ExtraModels = []string{"model-a", " model-b ", "model-b", "MODEL-B"}
	if err := p.Normalize(); err != nil {
		t.Fatal(err)
	}
	if p.Enabled || p.ProbeConfig.Enabled || p.CapabilityConfig.Enabled || p.ProbeConfig.IncludeExtraModels {
		t.Fatal("defaults enabled an operation")
	}
	if len(p.ExtraModels) != 2 || p.ExtraModels[0] != "model-b" || p.ExtraModels[1] != "MODEL-B" {
		t.Fatalf("bad normalization: %v", p.ExtraModels)
	}
}

func TestDecodeMonitorConfigStrictAndMissingFields(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "[]", `{"enabled":true,"unknown":1}`, `{"interval_seconds":1.5}`, `{"interval_seconds":"60"}`, `{"interval_seconds":null}`, `{"enabled":false,"enabled":true}`, `{} {}`} {
		t.Run(raw, func(t *testing.T) {
			cfg := DefaultGroupProbeConfig()
			if err := DecodeMonitorConfig([]byte(raw), &cfg); err == nil {
				t.Fatalf("accepted invalid config %s", raw)
			}
		})
	}
	cfg := DefaultGroupProbeConfig()
	if err := DecodeMonitorConfig([]byte(`{"interval_seconds":120}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.IncludeExtraModels || cfg.IntervalSeconds != 120 || cfg.JitterSeconds != 15 {
		t.Fatalf("lost defaults: %+v", cfg)
	}
}

func TestMonitorGroupConfigValidation(t *testing.T) {
	tests := map[string]func(*ChannelMonitorGroupPolicy){
		"missing group":      func(p *ChannelMonitorGroupPolicy) { p.GroupID = 0 },
		"probe jitter":       func(p *ChannelMonitorGroupPolicy) { p.ProbeConfig.JitterSeconds = 60 },
		"random fixed list":  func(p *ChannelMonitorGroupPolicy) { p.ProbeConfig.FixedAccountIDs = []int64{1} },
		"fixed missing list": func(p *ChannelMonitorGroupPolicy) { p.ProbeConfig.SelectionMode = MonitorSelectionFixed },
		"duplicate accounts": func(p *ChannelMonitorGroupPolicy) {
			p.ProbeConfig.SelectionMode = MonitorSelectionFixed
			p.ProbeConfig.FixedAccountIDs = []int64{1, 1}
			p.ProbeConfig.SampleSize = 2
		},
		"sample limit":              func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.SampleSize = 4 },
		"capability short interval": func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.IntervalSeconds = 60 },
		"capability TTL":            func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.ResultTTLSeconds = 3600 },
		"unknown tier":              func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.Tier = "unlimited" },
		"enabled no target":         func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.Enabled = true },
		"negative budget":           func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.DailyRequestLimit = -1 },
		"extra retests":             func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.RetestLimitPerExecution = 2 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p := testInactiveGroupPolicy()
			mutate(&p)
			if err := p.Normalize(); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	p := testInactiveGroupPolicy()
	p.CapabilityConfig.Enabled = true
	p.CapabilityConfig.Targets = []DetectorTarget{{RequestModel: "model-a", ClaimedModel: "candidate", BenchmarkID: "fixture", BenchmarkVersion: "1", BenchmarkSHA256: strings.Repeat("a", 64)}}
	if err := p.Normalize(); err != nil {
		t.Fatal(err)
	}
	p.CapabilityConfig.Targets[0].ClaimedModel = "reference-only:other"
	if err := p.Normalize(); err == nil {
		t.Fatal("accepted reference-only candidate")
	}
}

func TestMonitorGroupRevisionIgnoresPresentationAndOrder(t *testing.T) {
	p := testInactiveGroupPolicy()
	p.CapabilityConfig.SelectionMode = MonitorSelectionFixed
	p.CapabilityConfig.FixedAccountIDs = []int64{2, 1}
	p.CapabilityConfig.SampleSize = 2
	a, err := p.CapabilityRevision()
	if err != nil {
		t.Fatal(err)
	}
	p.DisplayName = "renamed"
	p.Version = 20
	p.ProbeConfig.IntervalSeconds = 120
	p.CapabilityConfig.FixedAccountIDs = []int64{1, 2}
	b, _ := p.CapabilityRevision()
	if a != b {
		t.Fatal("presentation/order invalidated evidence")
	}
	p.GroupID = 2
	c, _ := p.CapabilityRevision()
	if a == c {
		t.Fatal("group change did not invalidate evidence")
	}
}

func TestMonitorGroupDefaultsJSONRoundTrip(t *testing.T) {
	// JSON roundtrip also catches accidental secret-bearing or untagged defaults.
	p := DefaultGroupProbeConfig()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GroupProbeConfig
	if err := DecodeMonitorConfig(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if decoded.Enabled {
		t.Fatal("enabled by roundtrip")
	}
}
