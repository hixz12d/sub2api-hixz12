package service

import (
	"context"
	"time"
)

// A short per-instance cache bounds cross-instance convergence. Read errors and
// corrupt/missing rows retain the last valid snapshot instead of downgrading it.
func (s *SettingService) ClientProfileUpdates(ctx context.Context) map[string]ClientProfileUpdate {
	result := map[string]ClientProfileUpdate{"pi": defaultClientProfileUpdate(), "opencode": defaultClientProfileUpdate()}
	if s == nil || s.settingRepo == nil {
		return result
	}
	s.clientProfileMu.Lock()
	defer s.clientProfileMu.Unlock()
	if s.clientProfileCache == nil || time.Now().After(s.clientProfileExpires) {
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		values, err := s.settingRepo.GetMultiple(readCtx, []string{clientProfileUpdateKey("pi"), clientProfileUpdateKey("opencode")})
		cancel()
		if s.clientProfileCache == nil {
			s.clientProfileCache = make(map[string]ClientProfileUpdate)
		}
		for family := range result {
			if value, exists := values[clientProfileUpdateKey(family)]; err == nil && exists {
				if state, decodeErr := decodeClientProfileUpdate(family, value); decodeErr == nil {
					s.clientProfileCache[family] = state
					continue
				}
			} else if err == nil {
				if _, previouslyLoaded := s.clientProfileCache[family]; !previouslyLoaded {
					continue
				}
			}
			state, exists := s.clientProfileCache[family]
			if !exists {
				state = defaultClientProfileUpdate()
			}
			state.Status, state.Reason = "storage_unavailable", "last_good_retained"
			s.clientProfileCache[family] = state
		}
		s.clientProfileExpires = time.Now().Add(30 * time.Second)
	}
	for family, state := range s.clientProfileCache {
		result[family] = cloneClientProfileUpdate(state)
	}
	return result
}

func (s *SettingService) rememberClientProfileUpdate(family string, state ClientProfileUpdate) {
	if s == nil {
		return
	}
	s.clientProfileMu.Lock()
	defer s.clientProfileMu.Unlock()
	if s.clientProfileCache == nil {
		s.clientProfileCache = make(map[string]ClientProfileUpdate)
	}
	s.clientProfileCache[family] = cloneClientProfileUpdate(state)
	s.clientProfileExpires = time.Time{}
}

func (s *AccountTestService) ClientProfileUpdates(ctx context.Context) map[string]ClientProfileUpdate {
	if s == nil {
		return (*SettingService)(nil).ClientProfileUpdates(ctx)
	}
	return s.settingService.ClientProfileUpdates(ctx)
}

func (s *AccountTestService) SetClientProfileUpdatePolicy(ctx context.Context, family, action string) error {
	if s == nil {
		return (*SettingService)(nil).SetClientProfileUpdatePolicy(ctx, family, action)
	}
	return s.settingService.SetClientProfileUpdatePolicy(ctx, family, action)
}
