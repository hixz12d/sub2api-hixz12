package service

// CaptureMonitorUsage copies only independently validated observations. An
// invalid monitor sample must never fail or alter the customer's billing write.
func CaptureMonitorUsage(observation *VisibleOutputObservation) UsageLog {
	var fields UsageLog
	if observation == nil || !observation.Origin.Valid() {
		return fields
	}
	origin := string(observation.Origin)
	fields.RequestOrigin = &origin
	if observation.Version != VisibleStreamObservationVersion || observation.Method != VisibleStreamObservationMethod {
		return fields
	}
	version := observation.Version
	fields.MonitorObservationVersion = &version
	if observation.InputTokensTotal != nil && *observation.InputTokensTotal >= 0 {
		value := *observation.InputTokensTotal
		fields.MonitorInputTokensTotal = &value
		if observation.CacheReadTokens != nil && *observation.CacheReadTokens >= 0 && *observation.CacheReadTokens <= value {
			cached := *observation.CacheReadTokens
			fields.MonitorCacheReadTokens = &cached
		}
	}
	if !observation.OutputComplete {
		return fields
	}
	fields.MonitorFirstVisibleMs = observation.FirstVisibleOutputMs()
	rate, err := observation.OutputTPSMilli()
	if err != nil || rate == nil {
		return fields
	}
	visible := *observation.VisibleOutputTokens
	generation := observation.LastVisibleTextAt.Sub(*observation.FirstVisibleTextAt).Milliseconds()
	method := observation.Method
	fields.MonitorVisibleOutputTokens = &visible
	fields.MonitorGenerationMs = &generation
	fields.MonitorOutputTPSMilli = rate
	fields.MonitorTPSMethod = &method
	return fields
}
