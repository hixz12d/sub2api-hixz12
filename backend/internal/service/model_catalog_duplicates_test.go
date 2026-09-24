package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Duplicate model keys silently replace pricing when decoded into a map.
// Keep this guard for future upstream merges of the bundled catalog.
func TestBundledModelCatalogHasUniqueModels(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	require.True(t, json.Valid(data))
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), start)
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		model, ok := key.(string)
		require.True(t, ok)
		require.False(t, seen[model], "duplicate model pricing: %s", model)
		seen[model] = true
		var entry json.RawMessage
		require.NoError(t, decoder.Decode(&entry))
	}
}
