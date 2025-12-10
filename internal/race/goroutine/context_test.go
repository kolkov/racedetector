package goroutine

import (
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

// verifyVectorClockInit checks vector clock initialization with C[tid]=1 and others=0.
func verifyVectorClockInit(t *testing.T, ctx *RaceContext, tid uint16) {
	t.Helper()
	if ctx.C == nil {
		t.Fatal("Alloc() returned nil vector clock")
	}
	for i := 0; i < vectorclock.MaxThreads; i++ {
		expected := uint32(0)
		if uint16(i) == tid {
			expected = 1 // Own clock starts at 1
		}
		if ctx.C.Get(uint16(i)) != expected {
			t.Errorf("Alloc() C[%d] = %d, want %d", i, ctx.C.Get(uint16(i)), expected)
		}
	}
}

// verifyEpochInit checks that epoch cache is properly initialized.
func verifyEpochInit(t *testing.T, ctx *RaceContext, tid uint16) {
	t.Helper()
	// Verify epoch cache is initialized correctly (clock=1).
	wantEpoch := epoch.NewEpoch(tid, 1)
	if ctx.Epoch != wantEpoch {
		t.Errorf("Alloc(%d).Epoch = 0x%X, want 0x%X", tid, ctx.Epoch, wantEpoch)
	}

	// Verify epoch cache matches C[TID] (invariant).
	tidClock := ctx.C.Get(ctx.TID)
	epochFromVC := epoch.NewEpoch(ctx.TID, uint64(tidClock))
	if ctx.Epoch != epochFromVC {
		t.Errorf("Epoch cache out of sync: Epoch=0x%X, NewEpoch(TID=%d, C[%d]=%d)=0x%X",
			ctx.Epoch, ctx.TID, ctx.TID, tidClock, epochFromVC)
	}
}

// TestAlloc tests RaceContext allocation and initialization.
func TestAlloc(t *testing.T) {
	tests := []struct {
		name    string
		tid     uint16
		wantTID uint16
	}{
		{"zero tid", 0, 0},
		{"small tid", 5, 5},
		{"mid tid", 128, 128},
		{"max tid", 255, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Alloc(tt.tid)

			if ctx.TID != tt.wantTID {
				t.Errorf("Alloc(%d).TID = %d, want %d", tt.tid, ctx.TID, tt.wantTID)
			}

			verifyVectorClockInit(t, ctx, tt.tid)
			verifyEpochInit(t, ctx, tt.tid)
		})
	}
}

// verifyClockValue checks that the clock value matches expected.
func verifyClockValue(t *testing.T, ctx *RaceContext, wantClock uint32, increments int) {
	t.Helper()
	gotClock := ctx.C.Get(ctx.TID)
	if gotClock != wantClock {
		t.Errorf("After %d increments, C[%d] = %d, want %d",
			increments, ctx.TID, gotClock, wantClock)
	}
}

// verifyEpochCache checks that epoch cache is synchronized with C[TID].
func verifyEpochCache(t *testing.T, ctx *RaceContext, tid uint16, wantClock uint32, increments int) {
	t.Helper()
	wantEpoch := epoch.NewEpoch(tid, uint64(wantClock))
	if ctx.Epoch != wantEpoch {
		t.Errorf("After %d increments, Epoch = 0x%X, want 0x%X",
			increments, ctx.Epoch, wantEpoch)
	}

	gotTID, gotEpochClock := ctx.Epoch.Decode()
	if gotTID != tid {
		t.Errorf("Epoch.Decode() tid = %d, want %d", gotTID, tid)
	}
	if gotEpochClock != uint64(wantClock) {
		t.Errorf("Epoch.Decode() clock = %d, want %d", gotEpochClock, wantClock)
	}
}

// verifyThreadIsolation checks that other threads' clocks are unchanged (still 0).
func verifyThreadIsolation(t *testing.T, ctx *RaceContext) {
	t.Helper()
	for i := 0; i < vectorclock.MaxThreads; i++ {
		if uint16(i) == ctx.TID {
			continue // Skip own TID - it has a non-zero clock
		}
		if ctx.C.Get(uint16(i)) != 0 {
			t.Errorf("IncrementClock() affected other thread: C[%d] = %d, want 0",
				i, ctx.C.Get(uint16(i)))
		}
	}
}

// TestIncrementClock tests logical clock advancement.
// Note: Clock starts at 1 (not 0) to enable race detection.
func TestIncrementClock(t *testing.T) {
	tests := []struct {
		name       string
		tid        uint16
		increments int
		wantClock  uint32 // Expected clock = 1 (initial) + increments
	}{
		{
			name:       "single increment",
			tid:        5,
			increments: 1,
			wantClock:  2, // 1 (initial) + 1 increment
		},
		{
			name:       "multiple increments",
			tid:        10,
			increments: 100,
			wantClock:  101, // 1 (initial) + 100 increments
		},
		{
			name:       "zero increments",
			tid:        0,
			increments: 0,
			wantClock:  1, // Initial clock value (no increments)
		},
		{
			name:       "max tid increments",
			tid:        255,
			increments: 42,
			wantClock:  43, // 1 (initial) + 42 increments
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Alloc(tt.tid)

			// Perform increments.
			for i := 0; i < tt.increments; i++ {
				ctx.IncrementClock()
			}

			// Verify all invariants using helper functions.
			verifyClockValue(t, ctx, tt.wantClock, tt.increments)
			verifyEpochCache(t, ctx, tt.tid, tt.wantClock, tt.increments)
			verifyThreadIsolation(t, ctx)
		})
	}
}

// TestIncrementClockEpochSync tests epoch cache stays in sync with C[TID].
func TestIncrementClockEpochSync(t *testing.T) {
	ctx := Alloc(42)

	// Perform many increments and verify sync after each.
	// Initial clock is 1, so after i increments, clock = 1 + i
	for i := 1; i <= 1000; i++ {
		ctx.IncrementClock()

		// Verify epoch cache matches C[TID].
		expectedEpoch := epoch.NewEpoch(ctx.TID, uint64(ctx.C.Get(ctx.TID)))
		if ctx.Epoch != expectedEpoch {
			t.Errorf("Iteration %d: Epoch cache out of sync: got 0x%X, want 0x%X",
				i, ctx.Epoch, expectedEpoch)
		}

		// Verify epoch decodes to correct clock value.
		// Clock = 1 (initial) + i (increments)
		_, clock := ctx.Epoch.Decode()
		wantClock := uint64(1 + i)
		if clock != wantClock {
			t.Errorf("Iteration %d: Epoch clock = %d, want %d", i, clock, wantClock)
		}
	}
}

// TestGetEpoch tests cached epoch retrieval.
func TestGetEpoch(t *testing.T) {
	tests := []struct {
		name       string
		tid        uint16
		increments int
	}{
		{
			name:       "initial epoch (clock=1)",
			tid:        5,
			increments: 0,
		},
		{
			name:       "after single increment",
			tid:        10,
			increments: 1,
		},
		{
			name:       "after many increments",
			tid:        42,
			increments: 1000,
		},
		{
			name:       "max tid",
			tid:        255,
			increments: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Alloc(tt.tid)

			// Perform increments.
			for i := 0; i < tt.increments; i++ {
				ctx.IncrementClock()
			}

			// Get epoch using GetEpoch().
			gotEpoch := ctx.GetEpoch()

			// Verify it matches the cached Epoch field.
			if gotEpoch != ctx.Epoch {
				t.Errorf("GetEpoch() = 0x%X, want 0x%X (ctx.Epoch)", gotEpoch, ctx.Epoch)
			}

			// Verify it matches the expected epoch from C[TID].
			wantEpoch := epoch.NewEpoch(tt.tid, uint64(ctx.C.Get(tt.tid)))
			if gotEpoch != wantEpoch {
				t.Errorf("GetEpoch() = 0x%X, want 0x%X (from C[%d])", gotEpoch, wantEpoch, tt.tid)
			}

			// Verify epoch decodes correctly.
			// Clock = 1 (initial) + increments
			gotTID, gotClock := gotEpoch.Decode()
			wantClock := uint64(1 + tt.increments)
			if gotTID != tt.tid {
				t.Errorf("GetEpoch().Decode() tid = %d, want %d", gotTID, tt.tid)
			}
			if gotClock != wantClock {
				t.Errorf("GetEpoch().Decode() clock = %d, want %d", gotClock, wantClock)
			}
		})
	}
}

// TestGetEpochMultipleCalls tests that GetEpoch() is idempotent.
func TestGetEpochMultipleCalls(t *testing.T) {
	ctx := Alloc(7)
	ctx.IncrementClock()
	ctx.IncrementClock()
	ctx.IncrementClock()

	// Call GetEpoch() multiple times.
	e1 := ctx.GetEpoch()
	e2 := ctx.GetEpoch()
	e3 := ctx.GetEpoch()

	// All should return the same epoch.
	if e1 != e2 || e2 != e3 {
		t.Errorf("GetEpoch() not idempotent: e1=0x%X, e2=0x%X, e3=0x%X", e1, e2, e3)
	}

	// Verify the epoch is correct.
	// Initial clock = 1, after 3 increments = 4
	wantEpoch := epoch.NewEpoch(7, 4) // 1 + 3 = 4
	if e1 != wantEpoch {
		t.Errorf("GetEpoch() = 0x%X, want 0x%X", e1, wantEpoch)
	}
}

// TestTIDRange tests all valid TID values (0-255).
func TestTIDRange(t *testing.T) {
	for tid := 0; tid <= 255; tid++ {
		t.Run("tid_"+string(rune(tid)), func(t *testing.T) {
			ctx := Alloc(uint16(tid))

			// Verify TID is stored correctly.
			if ctx.TID != uint16(tid) {
				t.Errorf("Alloc(%d).TID = %d, want %d", tid, ctx.TID, tid)
			}

			// Increment and verify epoch cache.
			// Initial clock is 1, after one increment it should be 2.
			ctx.IncrementClock()
			wantEpoch := epoch.NewEpoch(uint16(tid), 2) // 1 (initial) + 1 increment = 2
			if ctx.Epoch != wantEpoch {
				t.Errorf("TID %d: Epoch = 0x%X, want 0x%X", tid, ctx.Epoch, wantEpoch)
			}

			// Verify epoch decodes correctly.
			gotTID, gotClock := ctx.Epoch.Decode()
			if gotTID != uint16(tid) {
				t.Errorf("TID %d: Epoch.Decode() tid = %d, want %d", tid, gotTID, tid)
			}
			if gotClock != 2 { // Clock should be 2 after one increment
				t.Errorf("TID %d: Epoch.Decode() clock = %d, want 2", tid, gotClock)
			}
		})
	}
}

// TestEpochCacheInvariant tests the critical invariant: Epoch == NewEpoch(TID, C[TID]).
func TestEpochCacheInvariant(t *testing.T) {
	ctx := Alloc(99)

	// Perform random increments and verify invariant holds.
	increments := []int{1, 5, 10, 50, 100, 1, 1, 1}
	for _, inc := range increments {
		for i := 0; i < inc; i++ {
			ctx.IncrementClock()
		}

		// Check invariant: ctx.Epoch == epoch.NewEpoch(ctx.TID, uint64(ctx.C.Get(ctx.TID)))
		expectedEpoch := epoch.NewEpoch(ctx.TID, uint64(ctx.C.Get(ctx.TID)))
		if ctx.Epoch != expectedEpoch {
			t.Errorf("Invariant violated: Epoch = 0x%X, NewEpoch(TID=%d, C[%d]=%d) = 0x%X",
				ctx.Epoch, ctx.TID, ctx.TID, ctx.C.Get(ctx.TID), expectedEpoch)
		}
	}
}

// TestVectorClockIsolation tests that incrementing one context doesn't affect others.
func TestVectorClockIsolation(t *testing.T) {
	ctx1 := Alloc(1)
	ctx2 := Alloc(2)
	ctx3 := Alloc(3)

	// Increment each context different amounts.
	ctx1.IncrementClock()
	ctx1.IncrementClock()
	ctx1.IncrementClock()

	ctx2.IncrementClock()

	ctx3.IncrementClock()
	ctx3.IncrementClock()

	// Verify each context has correct clock.
	// Initial clock is 1, so: final = 1 + increments
	if ctx1.C.Get(1) != 4 { // 1 + 3 increments
		t.Errorf("ctx1.C[1] = %d, want 4", ctx1.C.Get(1))
	}
	if ctx2.C.Get(2) != 2 { // 1 + 1 increment
		t.Errorf("ctx2.C[2] = %d, want 2", ctx2.C.Get(2))
	}
	if ctx3.C.Get(3) != 3 { // 1 + 2 increments
		t.Errorf("ctx3.C[3] = %d, want 3", ctx3.C.Get(3))
	}

	// Verify each context doesn't affect others' TID clocks.
	if ctx1.C.Get(2) != 0 || ctx1.C.Get(3) != 0 {
		t.Error("ctx1 affected other threads' clocks")
	}
	if ctx2.C.Get(1) != 0 || ctx2.C.Get(3) != 0 {
		t.Error("ctx2 affected other threads' clocks")
	}
	if ctx3.C.Get(1) != 0 || ctx3.C.Get(2) != 0 {
		t.Error("ctx3 affected other threads' clocks")
	}
}

// TestEpochClockOverflow tests behavior when clock reaches 32-bit limit.
// Note: Epoch now supports 48-bit clocks, but VectorClock uses 32-bit internally.
func TestEpochClockOverflow(t *testing.T) {
	ctx := Alloc(5)

	// Set clock to near 32-bit max (0xFFFFFFFF = 4,294,967,295).
	// Use a smaller value to avoid actual overflow in test.
	maxClock := uint32(0xFFFFFFF0)
	ctx.C.Set(5, maxClock)
	ctx.Epoch = epoch.NewEpoch(5, uint64(maxClock))

	// Increment multiple times.
	for i := 0; i < 10; i++ {
		ctx.IncrementClock()
	}

	// Verify clock advanced to maxClock + 10.
	expectedClock := maxClock + 10
	if ctx.C.Get(5) != expectedClock {
		t.Errorf("C[5] = %d, want %d", ctx.C.Get(5), expectedClock)
	}

	// Verify epoch cache matches (48-bit ClockMask supports full 32-bit values).
	wantEpoch := epoch.NewEpoch(5, uint64(expectedClock))
	if ctx.Epoch != wantEpoch {
		t.Errorf("Epoch = 0x%X, want 0x%X", ctx.Epoch, wantEpoch)
	}

	// Verify epoch decodes correctly (no truncation within 48-bit range).
	_, epochClockDecoded := ctx.Epoch.Decode()
	if epochClockDecoded != uint64(expectedClock) {
		t.Errorf("Epoch decoded clock = %d, want %d", epochClockDecoded, expectedClock)
	}
}

// ========== BENCHMARKS ==========

// BenchmarkGetEpoch benchmarks the critical hot-path GetEpoch() operation.
// Target: <1ns/op, 0 allocs/op (just a field read).
func BenchmarkGetEpoch(b *testing.B) {
	ctx := Alloc(42)
	ctx.IncrementClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.GetEpoch()
	}
}

// BenchmarkIncrementClock benchmarks logical clock advancement.
// Target: <200ns/op, 0 allocs/op (VectorClock update + epoch creation).
func BenchmarkIncrementClock(b *testing.B) {
	ctx := Alloc(42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.IncrementClock()
	}
}

// BenchmarkAlloc benchmarks RaceContext allocation.
func BenchmarkAlloc(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Alloc(uint16(i % 256))
	}
}

// BenchmarkGetEpochWithIncrement benchmarks realistic usage pattern.
// This simulates the typical cycle: increment clock, then read epoch.
func BenchmarkGetEpochWithIncrement(b *testing.B) {
	ctx := Alloc(42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.IncrementClock()
		_ = ctx.GetEpoch()
	}
}

// BenchmarkMultipleContexts benchmarks isolation with multiple contexts.
func BenchmarkMultipleContexts(b *testing.B) {
	contexts := make([]*RaceContext, 10)
	for i := range contexts {
		contexts[i] = Alloc(uint16(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := contexts[i%10]
		ctx.IncrementClock()
		_ = ctx.GetEpoch()
	}
}
