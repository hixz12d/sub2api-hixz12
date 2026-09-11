package domain

// GroupModelsListConfig controls the optional custom /v1/models response list.
// It is independent of the opt-in request admission allowlist.
type GroupModelsListConfig struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}
