package shadowmem

import (
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

// Baseline: Phase 2 behavior (always using Epoch for comparison).
// We'll simulate this by measuring epoch-only operations.

// BenchmarkVarState_SingleReader_Phase2_AlwaysEpoch benchmarks Phase 2 single-reader performance.
// Phase 2 always uses Epoch (no VectorClock), so this is the baseline.
func BenchmarkVarState_SingleReader_Phase2_AlwaysEpoch(b *testing.B) {
	vs := NewVarState()
	readEpoch := epoch.NewEpoch(5, 100)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate read: Check epoch, update epoch.
		if vs.GetReadEpoch() == readEpoch {
			continue
		}
		vs.SetReadEpoch(readEpoch)
	}
}

// BenchmarkVarState_SingleReader_Phase3_Adaptive benchmarks Phase 3 adaptive single-reader performance.
// Should be same or better than Phase 2 (fast path with IsPromoted() check).
func BenchmarkVarState_SingleReader_Phase3_Adaptive(b *testing.B) {
	vs := NewVarState()
	readEpoch := epoch.NewEpoch(5, 100)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate adaptive read: Check if promoted, then check epoch.
		if !vs.IsPromoted() {
			if vs.GetReadEpoch() == readEpoch {
				continue
			}
			vs.SetReadEpoch(readEpoch)
		}
	}
}

// BenchmarkVarState_MultipleReaders_Phase2_VectorClock benchmarks Phase 2 multi-reader with VectorClock.
// Phase 2 would always use VectorClock for read-shared variables.
func BenchmarkVarState_MultipleReaders_Phase2_VectorClock(b *testing.B) {
	// Simulate Phase 2: Always allocated VectorClock.
	vc1 := vectorclock.New()
	vc2 := vectorclock.New()
	vc1.Set(5, 100)
	vc2.Set(3, 50)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate read: Join VectorClocks (Phase 2 always does this for reads).
		vc1.Join(vc2)
	}
}

// BenchmarkVarState_MultipleReaders_Phase3_Promoted benchmarks Phase 3 promoted multi-reader.
// After promotion, this should be similar to Phase 2 VectorClock performance.
func BenchmarkVarState_MultipleReaders_Phase3_Promoted(b *testing.B) {
	vs := NewVarState()

	// Promote to VectorClock.
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	vc2 := vectorclock.New()
	vc2.Set(3, 50)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate read: Check promoted, then join.
		if vs.IsPromoted() {
			vs.GetReadClock().Join(vc2)
		}
	}
}

// BenchmarkVarState_Promotion_Overhead benchmarks the one-time cost of promotion.
// Target: <100ns per promotion.
func BenchmarkVarState_Promotion_Overhead(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		vs := NewVarState()
		vs.SetReadEpoch(epoch.NewEpoch(5, 100))
		vc := vectorclock.New()
		vc.Set(3, 50)
		b.StartTimer()

		// Measure only promotion cost.
		vs.PromoteToReadClock(vc)
	}
}

// BenchmarkVarState_IsPromoted checks the cost of IsPromoted() check (should be ~0ns).
func BenchmarkVarState_IsPromoted(b *testing.B) {
	vs := NewVarState()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.IsPromoted()
	}
}

// BenchmarkVarState_GetReadEpoch_Unpromoted benchmarks fast-path read epoch access.
func BenchmarkVarState_GetReadEpoch_Unpromoted(b *testing.B) {
	vs := NewVarState()
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.GetReadEpoch()
	}
}

// BenchmarkVarState_GetReadClock_Promoted benchmarks slow-path read clock access.
func BenchmarkVarState_GetReadClock_Promoted(b *testing.B) {
	vs := NewVarState()
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.GetReadClock()
	}
}

// BenchmarkVarState_Demotion benchmarks the cost of demotion (write clearing read state).
func BenchmarkVarState_Demotion(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		vs := NewVarState()
		vc := vectorclock.New()
		vc.Set(5, 100)
		vs.PromoteToReadClock(vc)
		b.StartTimer()

		// Measure demotion cost.
		vs.SetReadEpoch(0)
		vs.readClock = nil
	}
}

// BenchmarkVarState_PromotionDemotion_Cycle benchmarks full promotion/demotion cycle.
// This simulates alternating concurrent reads and writes (realistic workload).
func BenchmarkVarState_PromotionDemotion_Cycle(b *testing.B) {
	vs := NewVarState()
	vc := vectorclock.New()
	vc.Set(5, 100)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Promotion (concurrent reads).
		vs.SetReadEpoch(epoch.NewEpoch(3, 50))
		vs.PromoteToReadClock(vc)

		// Demotion (write).
		vs.SetReadEpoch(0)
		vs.readClock = nil
	}
}

// BenchmarkVarState_FastPath_Read_Same_Epoch benchmarks the fastest case (same epoch read).
// This should be the absolute minimum overhead.
func BenchmarkVarState_FastPath_Read_Same_Epoch(b *testing.B) {
	vs := NewVarState()
	readEpoch := epoch.NewEpoch(5, 100)
	vs.SetReadEpoch(readEpoch)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Fast path: Same epoch check (should be ~1-2ns).
		if !vs.IsPromoted() && vs.GetReadEpoch().Same(readEpoch) {
			continue
		}
	}
}

// BenchmarkVarState_FastPath_Read_Different_Epoch benchmarks fast path with epoch update.
func BenchmarkVarState_FastPath_Read_Different_Epoch(b *testing.B) {
	vs := NewVarState()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Fast path: Different epoch (update required).
		if !vs.IsPromoted() {
			readEpoch := epoch.NewEpoch(5, uint64(i))
			vs.SetReadEpoch(readEpoch)
		}
	}
}

// BenchmarkVarState_SlowPath_Read_VectorClock benchmarks slow path read (promoted).
func BenchmarkVarState_SlowPath_Read_VectorClock(b *testing.B) {
	vs := NewVarState()
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	vc2 := vectorclock.New()
	vc2.Set(3, 50)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Slow path: VectorClock join (should be ~300-500ns).
		if vs.IsPromoted() {
			vs.GetReadClock().Join(vc2)
		}
	}
}

// BenchmarkVarState_Write_Demote_FastPath benchmarks write to unpromoted variable.
func BenchmarkVarState_Write_Demote_FastPath(b *testing.B) {
	vs := NewVarState()
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))
	writeEpoch := epoch.NewEpoch(3, 200)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Write: Update W, clear read state (unpromoted).
		vs.SetW(writeEpoch)
		vs.SetReadEpoch(0)
	}
}

// BenchmarkVarState_Write_Demote_SlowPath benchmarks write to promoted variable (with demotion).
func BenchmarkVarState_Write_Demote_SlowPath(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		vs := NewVarState()
		vc := vectorclock.New()
		vc.Set(5, 100)
		vs.PromoteToReadClock(vc)
		writeEpoch := epoch.NewEpoch(3, 200)
		b.StartTimer()

		// Write: Update W, clear read state, demote.
		vs.SetW(writeEpoch)
		vs.SetReadEpoch(0)
		vs.readClock = nil
	}
}

// BenchmarkVarState_String_Unpromoted benchmarks String() for unpromoted state.
func BenchmarkVarState_String_Unpromoted(b *testing.B) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.String()
	}
}

// BenchmarkVarState_String_Promoted benchmarks String() for promoted state.
func BenchmarkVarState_String_Promoted(b *testing.B) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vc := vectorclock.New()
	vc.Set(3, 50)
	vc.Set(7, 60)
	vs.PromoteToReadClock(vc)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = vs.String()
	}
}

// BenchmarkVarState_Reset_Unpromoted benchmarks Reset() on unpromoted state.
func BenchmarkVarState_Reset_Unpromoted(b *testing.B) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vs.Reset()
		// Re-initialize for next iteration.
		vs.SetW(epoch.NewEpoch(5, 100))
		vs.SetReadEpoch(epoch.NewEpoch(3, 50))
	}
}

// BenchmarkVarState_Reset_Promoted benchmarks Reset() on promoted state.
func BenchmarkVarState_Reset_Promoted(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		vs := NewVarState()
		vc := vectorclock.New()
		vc.Set(5, 100)
		vs.PromoteToReadClock(vc)
		b.StartTimer()

		vs.Reset()
	}
}
