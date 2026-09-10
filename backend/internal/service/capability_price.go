package service

import (
	"encoding/json"
	"math"
)

// Prices are integer microdollars per token and rounded upward by the caller.
// An input ceiling must be enforced by the provider for this bound to be hard.
type CapabilityPriceCeiling struct {
	InputTokenLimit      int64 `json:"input_token_limit"`
	InputMicrosPerToken  int64 `json:"input_micros_per_token"`
	OutputMicrosPerToken int64 `json:"output_micros_per_token"`
	FixedMicros          int64 `json:"fixed_micros"`
}

func (p CapabilityPriceCeiling) UpperBound(payload []byte) (int64, error) {
	if p.InputTokenLimit <= 0 || p.InputTokenLimit > 10000000 || p.InputMicrosPerToken <= 0 || p.OutputMicrosPerToken <= 0 || p.FixedMicros < 0 || len(payload) == 0 || len(payload) > 65536 {
		return 0, ErrMonitorOutboundDenied
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return 0, ErrMonitorOutboundDenied
	}
	var output int64
	found := 0
	for _, key := range []string{"max_output_tokens", "max_tokens", "max_completion_tokens"} {
		if raw, ok := fields[key]; ok {
			found++
			if json.Unmarshal(raw, &output) != nil {
				return 0, ErrMonitorOutboundDenied
			}
		}
	}
	if found != 1 || output <= 0 || output > 1000000 {
		return 0, ErrMonitorOutboundDenied
	}
	if p.InputMicrosPerToken > math.MaxInt64/p.InputTokenLimit || p.OutputMicrosPerToken > math.MaxInt64/output {
		return 0, ErrMonitorOutboundDenied
	}
	inputCost, outputCost := p.InputMicrosPerToken*p.InputTokenLimit, p.OutputMicrosPerToken*output
	if inputCost > math.MaxInt64-outputCost || p.FixedMicros > math.MaxInt64-inputCost-outputCost {
		return 0, ErrMonitorOutboundDenied
	}
	total := inputCost + outputCost + p.FixedMicros
	if total > 1000000000000 {
		return 0, ErrMonitorOutboundDenied
	}
	return total, nil
}
