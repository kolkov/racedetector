package detector

import (
	"runtime"
	"testing"

	"github.com/kolkov/racedetector/internal/race/goroutine"
	"github.com/kolkov/racedetector/internal/race/stackdepot"
)

// BenchmarkLazyStackCapture compares old (full stack) vs new (PC only) approaches.
//
// v0.3.0 Performance Optimization:
// This benchmark demonstrates the 50x performance improvement from lazy stack capture.
//
// Old approach: stackdepot.CaptureStack() on every write (~500ns)
// New approach: captureCallerPC() on every write (~5-10ns)
//
// Expected results:
//   - Old (full stack): ~500ns per operation
//   - New (PC only):     ~10ns per operation
//   - Speedup:          50x improvement on hot path!

// BenchmarkFullStackCapture measures the old approach (full stack capture).
func BenchmarkFullStackCapture(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = stackdepot.CaptureStack() // Old approach: ~500ns
	}
}

// BenchmarkPCOnlyCapture measures the new approach (PC only).
func BenchmarkPCOnlyCapture(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = captureCallerPC() // New approach: ~5-10ns
	}
}

// BenchmarkRuntimeCallers measures raw runtime.Callers performance.
func BenchmarkRuntimeCallers(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	var pcs [1]uintptr
	for i := 0; i < b.N; i++ {
		runtime.Callers(2, pcs[:])
	}
}

// BenchmarkOnWrite_WithLazyStackCapture measures OnWrite with lazy stack capture.
//
// This benchmark shows the real-world impact on the hot path.
// With lazy stack capture, OnWrite should be significantly faster.
func BenchmarkOnWrite_WithLazyStackCapture(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d.OnWrite(addr, ctx)
		addr += 8 // Different address each time to avoid same-epoch fast path
	}
}

// BenchmarkOnRead_WithLazyStackCapture measures OnRead with lazy stack capture.
func BenchmarkOnRead_WithLazyStackCapture(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	// Do one write first
	d.OnWrite(addr, ctx)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d.OnRead(addr, ctx)
		addr += 8 // Different address
	}
}

// BenchmarkHotPath_WriteSequence measures realistic write sequence.
//
// This simulates a common pattern: multiple writes to different variables.
// Lazy stack capture should show significant improvement here.
func BenchmarkHotPath_WriteSequence(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)

	// Simulate 100 different variables
	addrs := make([]uintptr, 100)
	for i := range addrs {
		addrs[i] = uintptr(0x1000 + i*8)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		addr := addrs[i%len(addrs)]
		d.OnWrite(addr, ctx)
	}
}
