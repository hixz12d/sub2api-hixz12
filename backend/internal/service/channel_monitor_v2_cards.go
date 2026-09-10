package service

import (
	"context"
	"fmt"
	"time"
)

const ChannelMonitorV2CardBucket = 20 * time.Minute

type ChannelMonitorV2StatusCardSettings struct {
	TTFTP90WarningMs  *int64 `json:"ttft_p90_warning_ms"`
	TTFTP90CriticalMs *int64 `json:"ttft_p90_critical_ms"`
}

func (s ChannelMonitorV2StatusCardSettings) Validate() error {
	if s.TTFTP90WarningMs == nil && s.TTFTP90CriticalMs == nil {
		return nil
	}
	if s.TTFTP90WarningMs == nil || s.TTFTP90CriticalMs == nil || *s.TTFTP90WarningMs <= 0 || *s.TTFTP90WarningMs >= *s.TTFTP90CriticalMs {
		return fmt.Errorf("%w: TTFT P90 thresholds must both be null or 0 < warning < critical", ErrChannelMonitorV2InvalidConfig)
	}
	return nil
}

type ChannelMonitorV2CardIdentity struct {
	Platform string `json:"platform"`
	GroupID  int64  `json:"group_id"`
	Model    string `json:"model"`
}

type ChannelMonitorV2CardDisplay struct {
	PlatformLabel string `json:"platform_label"`
	GroupLabel    string `json:"group_label"`
	ModelLabel    string `json:"model_label"`
}

type ChannelMonitorV2CardCurrent struct {
	RequestState     string `json:"request_state"`
	PerformanceState string `json:"performance_state"`
	WindowSeconds    int    `json:"window_seconds"`
}

type ChannelMonitorV2ObservationEvidence struct {
	TPS         string `json:"tps"`
	VisibleTTFT string `json:"visible_ttft"`
	Cache       string `json:"cache"`
}

type ChannelMonitorV2CardWindow struct {
	ObservationEvidence    *ChannelMonitorV2ObservationEvidence `json:"observation_evidence,omitempty"`
	ObservedCacheReadRatio *float64                             `json:"observed_cache_read_ratio,omitempty"`
	ObservedCacheReason    string                               `json:"observed_cache_reason,omitempty"`
	SuccessRate            *float64                             `json:"success_rate"`
	TTFTP50Ms              *int64                               `json:"ttft_p50_ms"`
	TTFTP90Ms              *int64                               `json:"ttft_p90_ms"`
	DurationP50Ms          *int64                               `json:"duration_p50_ms"`
	CacheReadRatio         *float64                             `json:"cache_read_ratio"`
	ObservedTTFTP90Ms      *int64                               `json:"observed_ttft_p90_ms,omitempty"`
	OutputTpsP50Milli      *int64                               `json:"output_tps_p50_milli,omitempty"`
	OutputTpsReason        string                               `json:"output_tps_reason,omitempty"`
	Evidence               ChannelMonitorV2Measurement          `json:"evidence"`
	CoveredStart           *time.Time                           `json:"covered_start"`
	CoveredEnd             *time.Time                           `json:"covered_end"`
	CoverageComplete       bool                                 `json:"coverage_complete"`
}

type ChannelMonitorV2CardWindows struct {
	H24 ChannelMonitorV2CardWindow `json:"h24"`
	D7  ChannelMonitorV2CardWindow `json:"d7"`
}

type ChannelMonitorV2CardSlot struct {
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	SuccessRate *float64  `json:"success_rate"`
	TTFTP90Ms   *int64    `json:"ttft_p90_ms"`
	State       string    `json:"state"`
}

type ChannelMonitorV2StatusCard struct {
	Identity ChannelMonitorV2CardIdentity `json:"identity"`
	Display  ChannelMonitorV2CardDisplay  `json:"display"`
	Source   string                       `json:"source"`
	Current  ChannelMonitorV2CardCurrent  `json:"current"`
	Windows  ChannelMonitorV2CardWindows  `json:"windows"`
	Timeline []ChannelMonitorV2CardSlot   `json:"timeline"`
	// Capability remains null until the independently authorized module exists.
	Capability *struct{} `json:"capability"`
}

type ChannelMonitorV2AdminCardMetrics struct {
	H24     ChannelMonitorV2Metric `json:"h24"`
	D7      ChannelMonitorV2Metric `json:"d7"`
	Current ChannelMonitorV2Metric `json:"current"`
}

type ChannelMonitorV2AdminStatusCard struct {
	ChannelMonitorV2StatusCard
	Metrics ChannelMonitorV2AdminCardMetrics `json:"metrics"`
}

type ChannelMonitorV2AdminCards struct {
	*ChannelMonitorV2Cards
	Items []ChannelMonitorV2AdminStatusCard `json:"items"`
}

type ChannelMonitorV2Cards struct {
	adminItems            []ChannelMonitorV2AdminStatusCard
	Items                 []ChannelMonitorV2StatusCard `json:"items"`
	Page                  int                          `json:"page"`
	PageSize              int                          `json:"page_size"`
	HasMore               bool                         `json:"has_more"`
	ServerNow             time.Time                    `json:"server_now"`
	AsOf                  *time.Time                   `json:"as_of"`
	ComputedAt            *time.Time                   `json:"computed_at"`
	CoverageStart         *time.Time                   `json:"coverage_start"`
	AggregationLagSeconds *int64                       `json:"aggregation_lag_seconds"`
	CoverageComplete      bool                         `json:"coverage_complete"`
	Stale                 bool                         `json:"stale"`
}

func (c *ChannelMonitorV2Cards) AppendAdminCard(card ChannelMonitorV2StatusCard, metrics ChannelMonitorV2AdminCardMetrics) {
	c.adminItems = append(c.adminItems, ChannelMonitorV2AdminStatusCard{ChannelMonitorV2StatusCard: card, Metrics: metrics})
}

func (c *ChannelMonitorV2Cards) AdminResponse() ChannelMonitorV2AdminCards {
	items := c.adminItems
	if items == nil {
		items = []ChannelMonitorV2AdminStatusCard{}
	}
	return ChannelMonitorV2AdminCards{ChannelMonitorV2Cards: c, Items: items}
}

type ChannelMonitorV2CardsQuery struct {
	IncludeAdmin bool
	Filter       ChannelMonitorV2Filter
	Page         int
	PageSize     int
	AsOf         *time.Time
	ServerNow    time.Time
}

// A separate read port avoids coupling the passive aggregator to cards.
type ChannelMonitorV2CardsRepository interface {
	GetCards(context.Context, ChannelMonitorV2CardsQuery, ChannelMonitorV2Config) (*ChannelMonitorV2Cards, error)
}

func (s *ChannelMonitorV2Service) Cards(ctx context.Context, q ChannelMonitorV2CardsQuery) (*ChannelMonitorV2Cards, error) {
	if q.Page < 1 || q.Page > 1000000 || q.PageSize < 1 || q.PageSize > 100 {
		return nil, fmt.Errorf("%w: invalid cards pagination", ErrChannelMonitorV2InvalidRange)
	}
	cfg, err := s.getEnabledConfig(ctx)
	if err != nil {
		return nil, err
	}
	q.ServerNow = s.now().UTC()
	if q.AsOf != nil {
		_, offset := q.AsOf.Zone()
		if offset != 0 || !q.AsOf.Equal(q.AsOf.Truncate(5*time.Minute)) || q.AsOf.After(q.ServerNow) || q.AsOf.Before(q.ServerNow.Add(-24*time.Hour)) {
			return nil, fmt.Errorf("%w: invalid cards as_of", ErrChannelMonitorV2InvalidRange)
		}
	}
	repo, ok := s.repo.(ChannelMonitorV2CardsRepository)
	if !ok {
		return nil, fmt.Errorf("channel monitor cards repository unavailable")
	}
	return repo.GetCards(ctx, q, *cfg)
}

func ChannelMonitorV2CardRequestState(m ChannelMonitorV2Metric, thresholds ChannelMonitorV2HealthThresholds) string {
	e := ChannelMonitorV2MetricEvidence(m, thresholds.MinimumSample)
	if e.State != "valid" {
		return e.State
	}
	thresholds = NormalizeChannelMonitorV2HealthThresholds(thresholds)
	rawError := float64(m.ErrorRequests) / float64(m.RequestCount)
	if rawError >= thresholds.CriticalErrorRate {
		return "many_failures"
	}
	if rawError >= thresholds.WarningErrorRate {
		return "partial_failure"
	}
	return "normal"
}

func ChannelMonitorV2CardPerformanceState(m ChannelMonitorV2Metric, settings ChannelMonitorV2StatusCardSettings, minimumSample int64) string {
	if settings.TTFTP90WarningMs == nil || settings.TTFTP90CriticalMs == nil {
		return "not_configured"
	}
	if minimumSample <= 0 {
		minimumSample = DefaultChannelMonitorV2HealthThresholds().MinimumSample
	}
	if m.TTFT.P90Ms == nil || m.TTFT.SampleCount < minimumSample {
		return "unknown"
	}
	if *m.TTFT.P90Ms >= *settings.TTFTP90CriticalMs {
		return "very_slow"
	}
	if *m.TTFT.P90Ms >= *settings.TTFTP90WarningMs {
		return "slow"
	}
	return "normal"
}
