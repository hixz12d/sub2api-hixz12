package service

import (
	"strings"
	"testing"
)

func TestMonitorConfigRejectsAmbiguousKeysAndOversizedInput(t *testing.T) {
	cases := map[string]string{
		"uppercase alias":    `{"Enabled":true}`,
		"case duplicate":     `{"enabled":false,"ENABLED":true}`,
		"escaped alias":      `{"\u0045nabled":true}`,
		"unicode case alias": `{"\u017fample_size":2}`,
		"invalid utf8":       "{\"selection_mode\":\"" + string([]byte{0xff}) + "\"}",
		"oversized":          `{}` + strings.Repeat(" ", 64*1024),
		"deep nesting":       `{"fixed_account_ids":` + strings.Repeat("[", 40) + "1" + strings.Repeat("]", 40) + "}",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultGroupProbeConfig()
			if err := DecodeMonitorConfig([]byte(raw), &cfg); err == nil {
				t.Fatal("accepted ambiguous or unbounded input")
			}
			if cfg.Enabled {
				t.Fatal("rejected input enabled the config")
			}
		})
	}
	cfg := DefaultGroupProbeConfig()
	if err := DecodeMonitorConfig([]byte(`{"\u0065nabled":true}`), &cfg); err != nil || !cfg.Enabled {
		t.Fatalf("valid escaped lowercase key rejected: %v", err)
	}
}

func TestMonitorIdentifierStorageBounds(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*ChannelMonitorGroupPolicy)
	}{
		{"invalid model utf8", func(p *ChannelMonitorGroupPolicy) { p.PrimaryModel = string([]byte{0xff}) }},
		{"model newline", func(p *ChannelMonitorGroupPolicy) { p.PrimaryModel = "model\na" }},
		{"extra model nul", func(p *ChannelMonitorGroupPolicy) { p.ExtraModels = []string{"model\x00b"} }},
		{"display control", func(p *ChannelMonitorGroupPolicy) { p.DisplayName = "display\x00name" }},
		{"padded reference candidate", func(p *ChannelMonitorGroupPolicy) {
			p.CapabilityConfig.Targets[0].ClaimedModel = " reference-only:other "
		}},
		{"oversized claimed model", func(p *ChannelMonitorGroupPolicy) {
			p.CapabilityConfig.Targets[0].ClaimedModel = strings.Repeat("a", 201)
		}},
		{"oversized benchmark", func(p *ChannelMonitorGroupPolicy) {
			p.CapabilityConfig.Targets[0].BenchmarkID = strings.Repeat("a", 201)
		}},
		{"oversized version", func(p *ChannelMonitorGroupPolicy) {
			p.CapabilityConfig.Targets[0].BenchmarkVersion = strings.Repeat("a", 101)
		}},
		{"benchmark control", func(p *ChannelMonitorGroupPolicy) { p.CapabilityConfig.Targets[0].BenchmarkID = "baseline\t1" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := testInactiveGroupPolicy()
			p.CapabilityConfig.Targets = []DetectorTarget{{RequestModel: "model-a", ClaimedModel: "candidate", BenchmarkID: "fixture", BenchmarkVersion: "1", BenchmarkSHA256: strings.Repeat("a", 64)}}
			test.mutate(&p)
			if err := p.Normalize(); err == nil {
				t.Fatal("accepted identifier outside the storage contract")
			}
		})
	}
	p := testInactiveGroupPolicy()
	p.CapabilityConfig.Targets = []DetectorTarget{{RequestModel: "model-a", ClaimedModel: strings.Repeat("a", 200), BenchmarkID: strings.Repeat("b", 200), BenchmarkVersion: strings.Repeat("c", 100), BenchmarkSHA256: strings.Repeat("a", 64)}}
	if err := p.Normalize(); err != nil {
		t.Fatalf("exact storage bounds rejected: %v", err)
	}
}
