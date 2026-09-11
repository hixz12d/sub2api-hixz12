package service

import "strings"

func normalizeGroupModelsListConfig(cfg GroupModelsListConfig) GroupModelsListConfig {
	out := GroupModelsListConfig{Enabled: cfg.Enabled}
	if len(cfg.Models) == 0 {
		return out
	}

	seen := make(map[string]struct{}, len(cfg.Models))
	out.Models = make([]string, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out.Models = append(out.Models, model)
	}
	if len(out.Models) == 0 {
		out.Models = nil
	}
	return out
}

func (g *Group) CustomModelsListEnabled() bool {
	return g != nil && g.ModelsListConfig.Enabled && len(g.ModelsListConfig.Models) > 0
}

// ModelListingFilter combines display preferences with admission restrictions only
// for discovery responses. Request admission must continue to use ModelAllowlist.
func (g *Group) ModelListingFilter() GroupModelAllowlist {
	if g == nil {
		return GroupModelAllowlist{}
	}
	if !g.CustomModelsListEnabled() {
		return g.ModelAllowlist
	}
	models := make([]string, 0, len(g.ModelsListConfig.Models))
	for _, model := range g.ModelsListConfig.Models {
		if g.ModelAllowlist.Allows(model) {
			models = append(models, model)
		}
	}
	return GroupModelAllowlist{Enabled: true, Models: models}
}
