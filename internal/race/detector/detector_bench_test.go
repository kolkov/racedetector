package detector

import (
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/goroutine"
)

// BenchmarkOnWrite_NoRace benchmarks OnWrite in the common case (no race).
//
// This represents the typical path where writes happen in proper order
// with happens-before relationships.
//
// Target: <100ns per operation for MVP, <50ns ideal.
func BenchmarkOnWrite_NoRace(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	// Setup: First write to initialize shadow cell.
	d.OnWrite(addr, ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.OnWrite(addr, ctx)
	}
}

// BenchmarkOnWrite_NoRace_NewAddress benchmarks OnWrite for addresses
// that haven't been accessed before (cold path with allocation).
//
// This measures the overhead of creating new shadow cells.
func BenchmarkOnWrite_NoRace_NewAddress(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	baseAddr := uintptr(0x10000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Each iteration writes to a new address (cold path).
		addr := baseAddr + uintptr(i*8)
		d.OnWrite(addr, ctx)
	}
}

// BenchmarkOnWrite_SameEpoch benchmarks the same-epoch fast path.
//
// This is the CRITICAL optimization path that handles 71% of writes
// according to the FastTrack paper.
//
// Target: <20ns per operation (just a comparison and early return).
func BenchmarkOnWrite_SameEpoch(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x2000)

	// Setup: Write once to create shadow cell.
	d.OnWrite(addr, ctx)

	// Get shadow cell and manually set it to current epoch.
	vs := d.shadowMemory.Get(addr)
	vs.SetW(ctx.GetEpoch())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// This should hit the same-epoch fast path every time.
		// Note: We need to manually maintain vs.GetW() == currentEpoch
		// because IncrementClock advances the epoch.
		currentEpoch := ctx.GetEpoch()
		vs.SetW(currentEpoch)
		d.OnWrite(addr, ctx)
	}
}

// BenchmarkOnWrite_WithRace benchmarks OnWrite when races are detected.
//
// This measures the overhead of race reporting (stderr output).
// Note: This will produce race reports during benchmarking.
func BenchmarkOnWrite_WithRace(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x3000)

	// Setup to always trigger write-write race.
	vs := d.shadowMemory.GetOrCreate(addr)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Set previous write to future epoch to trigger race.
		vs.SetW(epoch.NewEpoch(1, 1000000))
		ctx.C.Set(1, uint32(i))
		ctx.Epoch = epoch.NewEpoch(1, uint64(i))

		d.OnWrite(addr, ctx)
	}
}

// BenchmarkOnWrite_MultipleAddresses benchmarks writes to different addresses.
//
// This tests the overhead when shadow memory needs to handle many different cells.
func BenchmarkOnWrite_MultipleAddresses(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	const numAddresses = 1000
	baseAddr := uintptr(0x100000)

	// Pre-populate shadow memory.
	for i := 0; i < numAddresses; i++ {
		addr := baseAddr + uintptr(i*8)
		d.OnWrite(addr, ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Round-robin through addresses.
		addr := baseAddr + uintptr((i%numAddresses)*8)
		d.OnWrite(addr, ctx)
	}
}

// BenchmarkHappensBeforeWrite benchmarks the happens-before check.
//
// This is a critical operation called on every write to check for races.
// Target: <10ns per operation.
func BenchmarkHappensBeforeWrite(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	ctx.C.Set(1, 100)

	prevWrite := epoch.NewEpoch(1, 50) // No race (50 <= 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = d.happensBeforeWrite(prevWrite, ctx)
	}
}

// BenchmarkHappensBeforeRead benchmarks the read happens-before check.
//
// Similar to write happens-before, this is on the critical path.
// Target: <10ns per operation.
func BenchmarkHappensBeforeRead(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	ctx.C.Set(1, 100)

	prevRead := epoch.NewEpoch(1, 50)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = d.happensBeforeRead(prevRead, ctx)
	}
}

// BenchmarkShadowMemoryGetOrCreate benchmarks shadow memory access.
//
// This is called on every memory access to retrieve the VarState cell.
// Performance is critical for overall detector throughput.
func BenchmarkShadowMemoryGetOrCreate(b *testing.B) {
	d := NewDetector()
	addr := uintptr(0x5000)

	// Pre-create the shadow cell (hot path).
	d.shadowMemory.GetOrCreate(addr)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = d.shadowMemory.GetOrCreate(addr)
	}
}

// BenchmarkParallelOnWrite benchmarks OnWrite under concurrent load.
//
// This tests the thread-safety overhead of concurrent writes.
func BenchmarkParallelOnWrite(b *testing.B) {
	d := NewDetector()
	baseAddr := uintptr(0x200000)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own context
		ctx := goroutine.Alloc(1) // In real usage, would have unique TIDs
		i := 0
		for pb.Next() {
			// Each goroutine writes to its own address space.
			addr := baseAddr + uintptr(i*8)
			d.OnWrite(addr, ctx)
			i++
		}
	})
}

// BenchmarkReportRace benchmarks the race reporting function.
//
// This is NOT on the hot path (only called when races are found),
// but we benchmark it to understand the overhead.
func BenchmarkReportRace(b *testing.B) {
	d := NewDetector()
	addr := uintptr(0xDEADBEEF)
	prevEpoch := epoch.NewEpoch(1, 100)
	currEpoch := epoch.NewEpoch(1, 200)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.reportRace("benchmark-race", addr, prevEpoch, currEpoch)
	}
}

// BenchmarkReset benchmarks the detector reset operation.
//
// This is used in testing and is NOT on the hot path.
func BenchmarkReset(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)

	// Populate with some data.
	for i := 0; i < 100; i++ {
		addr := uintptr(0x10000 + i*8)
		d.OnWrite(addr, ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.Reset()

		// Re-populate after reset to keep benchmark consistent.
		for j := 0; j < 100; j++ {
			addr := uintptr(0x10000 + j*8)
			d.OnWrite(addr, ctx)
		}
	}
}

// BenchmarkOnRead_NoRace benchmarks OnRead in the common case (no race).
//
// This represents the typical path where reads happen with proper
// happens-before relationships.
//
// Target: <100ns per operation for MVP, <50ns ideal.
func BenchmarkOnRead_NoRace(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	// Setup: First read to initialize shadow cell.
	d.OnRead(addr, ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.OnRead(addr, ctx)
	}
}

// BenchmarkOnRead_NoRace_NewAddress benchmarks OnRead for addresses
// that haven't been accessed before (cold path with allocation).
//
// This measures the overhead of creating new shadow cells.
func BenchmarkOnRead_NoRace_NewAddress(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	baseAddr := uintptr(0x10000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Each iteration reads from a new address (cold path).
		addr := baseAddr + uintptr(i*8)
		d.OnRead(addr, ctx)
	}
}

// BenchmarkOnRead_SameEpoch benchmarks the same-epoch fast path for reads.
//
// This is the CRITICAL optimization path that handles 63% of reads
// according to the FastTrack paper.
//
// Target: <20ns per operation (just a comparison and early return).
func BenchmarkOnRead_SameEpoch(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x2000)

	// Setup: Read once to create shadow cell.
	d.OnRead(addr, ctx)

	// Get shadow cell and manually set it to current epoch.
	vs := d.shadowMemory.Get(addr)
	vs.SetReadEpoch(ctx.GetEpoch())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// This should hit the same-epoch fast path every time.
		// Note: We need to manually maintain vs.GetReadEpoch() == currentEpoch
		// because IncrementClock advances the epoch.
		currentEpoch := ctx.GetEpoch()
		vs.SetReadEpoch(currentEpoch)
		d.OnRead(addr, ctx)
	}
}

// BenchmarkOnRead_WithRace benchmarks OnRead when races are detected.
//
// This measures the overhead of race reporting (stderr output).
// Note: This will produce race reports during benchmarking.
func BenchmarkOnRead_WithRace(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x3000)

	// Setup to always trigger write-read race.
	vs := d.shadowMemory.GetOrCreate(addr)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Set previous write to future epoch to trigger race.
		vs.SetW(epoch.NewEpoch(1, 1000000))
		ctx.C.Set(1, uint32(i))
		ctx.Epoch = epoch.NewEpoch(1, uint64(i))

		d.OnRead(addr, ctx)
	}
}

// BenchmarkOnRead_MultipleAddresses benchmarks reads from different addresses.
//
// This tests the overhead when shadow memory needs to handle many different cells.
func BenchmarkOnRead_MultipleAddresses(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	const numAddresses = 1000
	baseAddr := uintptr(0x100000)

	// Pre-populate shadow memory.
	for i := 0; i < numAddresses; i++ {
		addr := baseAddr + uintptr(i*8)
		d.OnRead(addr, ctx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Round-robin through addresses.
		addr := baseAddr + uintptr((i%numAddresses)*8)
		d.OnRead(addr, ctx)
	}
}

// BenchmarkOnRead_AfterWrite benchmarks read after write (common pattern).
//
// This simulates the typical pattern where a variable is written then read.
func BenchmarkOnRead_AfterWrite(b *testing.B) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x4000)

	// Initial write to set up shadow memory.
	d.OnWrite(addr, ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		d.OnRead(addr, ctx)
	}
}

// BenchmarkParallelOnRead benchmarks OnRead under concurrent load.
//
// This tests the thread-safety overhead of concurrent reads.
func BenchmarkParallelOnRead(b *testing.B) {
	d := NewDetector()
	baseAddr := uintptr(0x200000)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own context
		ctx := goroutine.Alloc(1) // In real usage, would have unique TIDs
		i := 0
		for pb.Next() {
			// Each goroutine reads from its own address space.
			addr := baseAddr + uintptr(i*8)
			d.OnRead(addr, ctx)
			i++
		}
	})
}

// BenchmarkParallelReadWrite benchmarks mixed reads and writes under load.
//
// This simulates real-world concurrent access patterns.
func BenchmarkParallelReadWrite(b *testing.B) {
	d := NewDetector()
	baseAddr := uintptr(0x300000)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own context
		ctx := goroutine.Alloc(1) // In real usage, would have unique TIDs
		i := 0
		for pb.Next() {
			addr := baseAddr + uintptr(i*8)
			// Alternate between reads and writes.
			if i%2 == 0 {
				d.OnRead(addr, ctx)
			} else {
				d.OnWrite(addr, ctx)
			}
			i++
		}
	})
}

// BenchmarkOnReadOnWrite_Comparison directly compares OnRead vs OnWrite performance.
//
// This helps verify that OnRead is as fast or faster than OnWrite.
func BenchmarkOnReadOnWrite_Comparison(b *testing.B) {
	b.Run("OnRead", func(b *testing.B) {
		d := NewDetector()
		ctx := goroutine.Alloc(1)
		addr := uintptr(0x5000)
		d.OnRead(addr, ctx) // Setup

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			d.OnRead(addr, ctx)
		}
	})

	b.Run("OnWrite", func(b *testing.B) {
		d := NewDetector()
		ctx := goroutine.Alloc(1)
		addr := uintptr(0x6000)
		d.OnWrite(addr, ctx) // Setup

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			d.OnWrite(addr, ctx)
		}
	})
}
