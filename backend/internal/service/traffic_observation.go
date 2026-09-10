package service

import (
	"errors"
	"math/bits"
	"time"
)

// RequestOrigin identifies why an upstream request was sent. It is deliberately
// separate from billing fields so probes/detector traffic cannot be inferred
// from or mixed into ordinary usage accounting.
type RequestOrigin string

const (
	RequestOriginBusiness           RequestOrigin = "real_traffic"
	RequestOriginAvailabilityProbe  RequestOrigin = "availability_probe"
	RequestOriginCapabilityDetector RequestOrigin = "capability_probe"
	RequestOriginUserDetector       RequestOrigin = "user_detector"
	RequestOriginLegacyUnknown      RequestOrigin = "legacy_unknown"
)

func (o RequestOrigin) Valid() bool {
	return o == RequestOriginBusiness || o == RequestOriginAvailabilityProbe || o == RequestOriginCapabilityDetector || o == RequestOriginUserDetector || o == RequestOriginLegacyUnknown
}

type VisibleOutputObservation struct {
	Origin              RequestOrigin
	RequestStartedAt    time.Time
	FirstVisibleTextAt  *time.Time
	LastVisibleTextAt   *time.Time
	VisibleOutputTokens *int64
	OutputComplete      bool
	TextDeltaCount      int
	InputTokensTotal    *int64
	CacheReadTokens     *int64
	Version             int
	Method              string
}

const (
	VisibleStreamObservationVersion = 1
	VisibleStreamObservationMethod  = "visible_stream_v1"
	visibleStreamMinTokens          = 16
	visibleStreamMinGenerationMs    = 200
)

// OutputTPSMilli excludes short, incomplete and non-business samples. No billing
// token count or duration-minus-TTFT estimate is used as a fallback.
func (o VisibleOutputObservation) OutputTPSMilli() (*int64, error) {
	if !o.Origin.Valid() {
		return nil, errors.New("invalid request origin")
	}
	if o.Origin != RequestOriginBusiness || !o.OutputComplete || o.FirstVisibleTextAt == nil || o.LastVisibleTextAt == nil || o.VisibleOutputTokens == nil {
		return nil, nil
	}
	if *o.VisibleOutputTokens <= 0 || o.RequestStartedAt.IsZero() || o.FirstVisibleTextAt.Before(o.RequestStartedAt) || o.LastVisibleTextAt.Before(*o.FirstVisibleTextAt) {
		return nil, errors.New("invalid visible output observation")
	}
	generationMs := o.LastVisibleTextAt.Sub(*o.FirstVisibleTextAt).Milliseconds()
	if o.Version != VisibleStreamObservationVersion || o.Method != VisibleStreamObservationMethod || o.TextDeltaCount < 2 || *o.VisibleOutputTokens < visibleStreamMinTokens || generationMs < visibleStreamMinGenerationMs {
		return nil, nil
	}
	hi, lo := bits.Mul64(uint64(*o.VisibleOutputTokens), 1000000)
	if hi >= uint64(generationMs) {
		return nil, errors.New("visible output rate overflow")
	}
	value, _ := bits.Div64(hi, lo, uint64(generationMs))
	if value > uint64(1<<63-1) {
		return nil, errors.New("visible output rate overflow")
	}
	rate := int64(value)
	return &rate, nil
}

func (o VisibleOutputObservation) VisibleOutputRate() (*float64, error) {
	milli, err := o.OutputTPSMilli()
	if err != nil || milli == nil {
		return nil, err
	}
	rate := float64(*milli) / 1000
	return &rate, nil
}

func (o VisibleOutputObservation) FirstVisibleOutputMs() *int64 {
	if o.FirstVisibleTextAt == nil || o.RequestStartedAt.IsZero() || o.FirstVisibleTextAt.Before(o.RequestStartedAt) {
		return nil
	}
	value := o.FirstVisibleTextAt.Sub(o.RequestStartedAt).Milliseconds()
	return &value
}
