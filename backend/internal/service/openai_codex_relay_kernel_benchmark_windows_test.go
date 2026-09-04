//go:build windows

package service

import (
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var (
	codexBenchmarkKernel32                      = syscall.NewLazyDLL("kernel32.dll")
	codexBenchmarkQueryPerformanceCounterAddr   = codexBenchmarkKernel32.NewProc("QueryPerformanceCounter").Addr()
	codexBenchmarkQueryPerformanceFrequencyAddr = codexBenchmarkKernel32.NewProc("QueryPerformanceFrequency").Addr()
)

func codexBenchmarkPerformanceCounter(procAddr uintptr) int64 {
	var value int64
	ok, _, callErr := syscall.SyscallN(procAddr, uintptr(unsafe.Pointer(&value)))
	if ok == 0 {
		panic(callErr)
	}
	return value
}

func BenchmarkCodexIdentityDerivationP99(b *testing.B) {
	deriver, err := NewCodexIdentityDeriver(testCodexRelaySecret)
	if err != nil {
		b.Fatal(err)
	}
	plan := &CodexRequestPlan{
		logicalRequestID:   "benchmark-logical-request",
		conversationDigest: strings.Repeat("a", 64),
		createdAt:          time.Unix(1_800_000_000, 0),
	}
	profile, err := ResolveCodexClientProfile(CodexProfileExec)
	if err != nil {
		b.Fatal(err)
	}
	deviceID := deriver.UUIDv4(codexNamespaceDevice, "44:account-v3", "credential-v5", profile.ID, strconv.Itoa(profile.Revision))
	plan.clientRequestID = deriver.UUIDv7(codexNamespaceClientRequest, plan.createdAt, plan.logicalRequestID, plan.conversationDigest)
	frequency := codexBenchmarkPerformanceCounter(codexBenchmarkQueryPerformanceFrequencyAddr)
	samples := make([]int64, b.N)
	clockOverhead := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		overheadStarted := codexBenchmarkPerformanceCounter(codexBenchmarkQueryPerformanceCounterAddr)
		clockOverhead[i] = codexBenchmarkPerformanceCounter(codexBenchmarkQueryPerformanceCounterAddr) - overheadStarted
		started := codexBenchmarkPerformanceCounter(codexBenchmarkQueryPerformanceCounterAddr)
		benchmarkCodexIdentity = deriveCodexV2Identity(deriver, plan, codexFingerprintSession, profile, deviceID, plan.clientRequestID)
		samples[i] = codexBenchmarkPerformanceCounter(codexBenchmarkQueryPerformanceCounterAddr) - started
	}
	b.StopTimer()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	sort.Slice(clockOverhead, func(i, j int) bool { return clockOverhead[i] < clockOverhead[j] })
	p99Index := (len(samples)*99 + 99) / 100
	if p99Index > 0 {
		p99Index--
	}
	overheadTicks := clockOverhead[len(clockOverhead)/2]
	adjustedP99Ticks := samples[p99Index] - overheadTicks
	if adjustedP99Ticks < 0 {
		adjustedP99Ticks = 0
	}
	nanosPerTick := float64(time.Second) / float64(frequency)
	b.ReportMetric(float64(adjustedP99Ticks)*nanosPerTick, "p99_ns")
	b.ReportMetric(float64(samples[p99Index])*nanosPerTick, "raw_p99_ns")
}
