//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFilterCodexInput_StripsFunctionCallItemID_WhenPreservingReferences
// verifies that function_call items with non-fc id (e.g. item_*) have their
// id stripped even when PreserveReferences is true. OpenAI upstream requires
// function_call ids to begin with "fc" and rejects item_* with 400:
// "Expected an ID that begins with 'fc'." (#3785)
func TestFilterCodexInput_StripsFunctionCallItemID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call",
			"id":      "item_A9v0SNfS3VaLrfX0j3y4xhyK",
			"call_id": "fc_abc123",
			"name":    "bash",
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "fc_abc123",
			"output":  "done",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 2)

	fc, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", fc["type"])
	_, hasID := fc["id"]
	require.False(t, hasID, "item_* id should be stripped from function_call")
	require.Equal(t, "fc_abc123", fc["call_id"], "call_id must be preserved")
	require.Equal(t, "bash", fc["name"])
}

// TestFilterCodexInput_KeepsFcID_WhenPreservingReferences
// verifies that function_call items with a valid fc* id are kept when
// PreserveReferences is true.
func TestFilterCodexInput_KeepsFcID_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call",
			"id":      "fc_validID123",
			"call_id": "fc_validID123",
			"name":    "bash",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	fc, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fc_validID123", fc["id"], "valid fc* id must be preserved")
}

// TestFilterCodexInput_StripsFcIDFromToolSearchCall_WhenPreservingReferences
// verifies that tool_search_call items keep tsc* ids but drop replayed fc_* ids.
// OpenAI upstream requires tool_search_call ids to begin with "tsc" and rejects
// fc_* with 400: "Expected an ID that begins with 'tsc'."
func TestFilterCodexInput_StripsFcIDFromToolSearchCall_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "tool_search_call",
			"id":      "fc_0bc1462075ce15c2016a87cb89e73887d085f05d00654737da",
			"call_id": "fc_search",
			"arguments": map[string]any{
				"query": "subagent",
			},
		},
		map[string]any{
			"type":    "tool_search_call",
			"id":      "tsc_validID123",
			"call_id": "fc_search2",
			"arguments": map[string]any{
				"query": "github",
			},
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 2)

	bad, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_search_call", bad["type"])
	_, hasID := bad["id"]
	require.False(t, hasID, "fc_* id should be stripped from tool_search_call")
	require.Equal(t, "fc_search", bad["call_id"], "call_id must be preserved")

	good, ok := filtered[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tsc_validID123", good["id"], "valid tsc* id must be preserved")
	require.Equal(t, "fc_search2", good["call_id"])
}

// TestFilterCodexInput_StripsFcIDFromCustomToolCall_WhenPreservingReferences
// verifies that custom_tool_call items keep ctc* ids but drop replayed fc_* ids.
// OpenAI upstream requires custom_tool_call ids to begin with "ctc" and rejects
// fc_* with 400: "Expected an ID that begins with 'ctc'."
func TestFilterCodexInput_StripsFcIDFromCustomToolCall_WhenPreservingReferences(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "custom_tool_call",
			"id":      "fc_04da631661f928f7016a87b53bdba887d09b0939bcf3231f90",
			"call_id": "fc_custom",
			"name":    "exec",
			"input":   "dir",
		},
		map[string]any{
			"type":    "custom_tool_call",
			"id":      "ctc_validID123",
			"call_id": "fc_custom2",
			"name":    "exec",
			"input":   "pwd",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 2)

	bad, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "custom_tool_call", bad["type"])
	_, hasID := bad["id"]
	require.False(t, hasID, "fc_* id should be stripped from custom_tool_call")
	require.Equal(t, "fc_custom", bad["call_id"], "call_id must be preserved")
	require.Equal(t, "exec", bad["name"])
	require.Equal(t, "dir", bad["input"])

	good, ok := filtered[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ctc_validID123", good["id"], "valid ctc* id must be preserved")
	require.Equal(t, "fc_custom2", good["call_id"])
}

func TestFilterCodexInput_PreservesNativeCustomAndToolSearchIDs(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "id": "ctc_valid", "call_id": "call_custom", "name": "apply_patch"},
		map[string]any{"type": "tool_search_call", "id": "tsc_valid", "call_id": "call_search"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "ctc_valid", filtered[0].(map[string]any)["id"])
	require.Equal(t, "tsc_valid", filtered[1].(map[string]any)["id"])
}

func TestFilterCodexInput_StripsWrongCustomAndToolSearchIDs(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "id": "fc_wrong", "call_id": "call_custom", "name": "apply_patch"},
		map[string]any{"type": "tool_search_call", "id": "fc_wrong", "call_id": "call_search"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.NotContains(t, filtered[0].(map[string]any), "id")
	require.NotContains(t, filtered[1].(map[string]any), "id")
}

func TestFilterCodexInput_MapsItemReferencesToNativeToolCallPair(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "id": "fc_custom", "call_id": "call_custom", "name": "apply_patch"},
		map[string]any{"type": "custom_tool_call_output", "call_id": "fc_custom", "output": "done"},
		map[string]any{"type": "item_reference", "id": "call_custom"},
		map[string]any{"type": "tool_search_call", "id": "fc_search", "call_id": "call_search"},
		map[string]any{"type": "tool_search_output", "call_id": "fc_search", "output": "result"},
		map[string]any{"type": "item_reference", "id": "call_search"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "ctc_custom", filtered[0].(map[string]any)["call_id"])
	require.Equal(t, "ctc_custom", filtered[1].(map[string]any)["call_id"])
	require.Equal(t, "ctc_custom", filtered[2].(map[string]any)["id"])
	require.Equal(t, "tsc_search", filtered[3].(map[string]any)["call_id"])
	require.Equal(t, "tsc_search", filtered[4].(map[string]any)["call_id"])
	require.Equal(t, "tsc_search", filtered[5].(map[string]any)["id"])
}

func TestFilterCodexInput_PreservesAmbiguousItemReference(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "call_id": "call_shared", "name": "apply_patch"},
		map[string]any{"type": "tool_search_call", "call_id": "call_shared"},
		map[string]any{"type": "item_reference", "id": "call_shared"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "ctc_shared", filtered[0].(map[string]any)["call_id"])
	require.Equal(t, "tsc_shared", filtered[1].(map[string]any)["call_id"])
	require.Equal(t, "call_shared", filtered[2].(map[string]any)["id"])
}

func TestFilterCodexInput_PreservesNativeItemIDReferenceIndependentlyFromCallID(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "id": "ctc_item", "call_id": "call_custom", "name": "apply_patch"},
		map[string]any{"type": "item_reference", "id": "ctc_item"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "ctc_item", filtered[0].(map[string]any)["id"])
	require.Equal(t, "ctc_custom", filtered[0].(map[string]any)["call_id"])
	require.Equal(t, "ctc_item", filtered[1].(map[string]any)["id"])
}

func TestFilterCodexInput_ExistingItemIDWinsOverLegacyCallIDMapping(t *testing.T) {
	input := []any{
		map[string]any{"type": "custom_tool_call", "call_id": "call_shared", "name": "apply_patch"},
		map[string]any{"type": "function_call_output", "id": "call_shared", "call_id": "call_other", "output": "done"},
		map[string]any{"type": "item_reference", "id": "call_shared"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "ctc_shared", filtered[0].(map[string]any)["call_id"])
	require.Equal(t, "call_shared", filtered[1].(map[string]any)["id"])
	require.Equal(t, "call_shared", filtered[2].(map[string]any)["id"])
}

func TestFilterCodexInput_NormalizesCrossTurnLegacyCallReference(t *testing.T) {
	input := []any{
		map[string]any{"type": "item_reference", "id": "call_previous_turn"},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

	require.Equal(t, "fc_previous_turn", filtered[0].(map[string]any)["id"])
}

func TestFilterCodexInput_PreservesNativeRemoteItemReferences(t *testing.T) {
	for _, id := range []string{"fc_remote", "ctc_remote", "tsc_remote", "msg_remote", "rs_remote", "vendor_remote"} {
		t.Run(id, func(t *testing.T) {
			input := []any{map[string]any{"type": "item_reference", "id": id}}

			filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true})

			require.Equal(t, id, filtered[0].(map[string]any)["id"])
		})
	}
}

// TestFilterCodexInput_StripsItemIDFromAllToolCallInputTypes verifies that
// item_* ids are stripped from all call-input types (not output types).
func TestFilterCodexInput_StripsItemIDFromAllToolCallInputTypes(t *testing.T) {
	types := []string{"function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call"}

	for _, typ := range types {
		input := []any{
			map[string]any{
				"type":    typ,
				"id":      "item_xyz",
				"call_id": "fc_001",
				"name":    "tool",
			},
		}
		filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
			PreserveReferences: true,
		})
		require.Len(t, filtered, 1)
		item, ok := filtered[0].(map[string]any)
		require.True(t, ok)
		_, hasID := item["id"]
		require.False(t, hasID, "item_* id should be stripped from %s", typ)
	}
}

// TestFilterCodexInput_OutputTypeKeepsItemID ensures tool-output items
// (e.g. function_call_output) keep their id — only call-input types have
// the fc* constraint.
func TestFilterCodexInput_OutputTypeKeepsItemID(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "function_call_output",
			"id":      "o1",
			"call_id": "fc_abc",
			"output":  "done",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	out, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "o1", out["id"], "output item id should be preserved")
}

// TestFilterCodexInput_NonToolCallItemKeepsID ensures items subject to neither
// the fc* (call-input) nor the msg* (message) prefix rule still keep their id
// when PreserveReferences is true.
// message is covered separately in openai_codex_message_item_id_test.go (#3981).
func TestFilterCodexInput_NonToolCallItemKeepsID(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "web_search_call",
			"id":   "ws_001",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
	})

	require.Len(t, filtered, 1)
	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ws_001", item["id"], "unconstrained items keep their id in preserve mode")
}
