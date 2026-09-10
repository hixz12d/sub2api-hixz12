package service

import (
	"testing"
	"time"
)

func TestVisibleOutputObservationSeparatesOriginAndCompleteness(t *testing.T) {
	started := time.Unix(100, 0)
	first := started.Add(2 * time.Second)
	last := first.Add(4 * time.Second)
	tokens := int64(80)
	base := VisibleOutputObservation{Origin: RequestOriginBusiness, RequestStartedAt: started, FirstVisibleTextAt: &first, LastVisibleTextAt: &last, VisibleOutputTokens: &tokens, OutputComplete: true}
	base.TextDeltaCount = 2
	base.Version = VisibleStreamObservationVersion
	base.Method = VisibleStreamObservationMethod
	rate, err := base.VisibleOutputRate()
	if err != nil || rate == nil || *rate != 20 {
		t.Fatalf("unexpected rate: %v %v", rate, err)
	}
	if ms := base.FirstVisibleOutputMs(); ms == nil || *ms != 2000 {
		t.Fatalf("unexpected first visible latency: %v", ms)
	}
	for _, origin := range []RequestOrigin{RequestOriginAvailabilityProbe, RequestOriginCapabilityDetector} {
		base.Origin = origin
		rate, err = base.VisibleOutputRate()
		if err != nil || rate != nil {
			t.Fatalf("non-business traffic became TPS: %v %v", rate, err)
		}
	}
	base.Origin = RequestOriginBusiness
	base.OutputComplete = false
	rate, err = base.VisibleOutputRate()
	if err != nil || rate != nil {
		t.Fatalf("incomplete stream became TPS: %v %v", rate, err)
	}
}

func TestVisibleOutputObservationRejectsInvalidValues(t *testing.T) {
	first := time.Unix(10, 0)
	last := first.Add(time.Second)
	zero := int64(0)
	for _, observation := range []VisibleOutputObservation{
		{Origin: "unknown"},
		{Origin: RequestOriginBusiness, FirstVisibleTextAt: &first, LastVisibleTextAt: &last, VisibleOutputTokens: &zero, OutputComplete: true},
		{Origin: RequestOriginBusiness, FirstVisibleTextAt: &last, LastVisibleTextAt: &first, VisibleOutputTokens: &zero, OutputComplete: true},
	} {
		if _, err := observation.VisibleOutputRate(); err == nil {
			t.Fatal("accepted invalid observation")
		}
	}
	before := first.Add(-time.Second)
	observation := VisibleOutputObservation{Origin: RequestOriginBusiness, RequestStartedAt: first, FirstVisibleTextAt: &before}
	if observation.FirstVisibleOutputMs() != nil {
		t.Fatal("accepted negative first visible latency")
	}
}
