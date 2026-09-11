package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsHaveUniqueIDs(t *testing.T) {
	seen := make(map[string]bool, len(DefaultModels))
	for _, model := range DefaultModels {
		require.NotEmpty(t, model.ID)
		require.False(t, seen[model.ID], "duplicate default model: %s", model.ID)
		seen[model.ID] = true
	}
}
