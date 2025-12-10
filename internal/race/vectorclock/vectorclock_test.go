package vectorclock

import (
	"testing"
)

// TestVectorClockNew tests zero initialization.
func TestVectorClockNew(t *testing.T) {
	vc := New()

	// Verify all clocks are zero (check a sample, not all 65536).
	for i := 0; i < 100; i++ {
		if vc.Get(uint16(i)) != 0 {
			t.Errorf("New() Get(%d) = %d, want 0", i, vc.Get(uint16(i)))
		}
	}

	// v0.3.0: maxTID should be 0 for new clock.
	if vc.GetMaxTID() != 0 {
		t.Errorf("New() GetMaxTID() = %d, want 0", vc.GetMaxTID())
	}
}

// TestVectorClockClone tests deep copy independence.
func TestVectorClockClone(t *testing.T) {
	// Create original with some values.
	original := New()
	original.Set(0, 10)
	original.Set(5, 20)
	original.Set(65535, 30)

	// Clone it.
	clone := original.Clone()

	// Verify clone has same values.
	if clone.Get(0) != 10 {
		t.Errorf("Clone().Get(0) = %d, want 10", clone.Get(0))
	}
	if clone.Get(5) != 20 {
		t.Errorf("Clone().Get(5) = %d, want 20", clone.Get(5))
	}
	if clone.Get(65535) != 30 {
		t.Errorf("Clone().Get(65535) = %d, want 30", clone.Get(65535))
	}

	// Modify clone.
	clone.Set(0, 999)
	clone.Set(5, 888)

	// Verify original is unchanged (deep copy).
	if original.Get(0) != 10 {
		t.Errorf("Original modified after clone change: Get(0) = %d, want 10", original.Get(0))
	}
	if original.Get(5) != 20 {
		t.Errorf("Original modified after clone change: Get(5) = %d, want 20", original.Get(5))
	}
}

// TestVectorClockJoinCommutativity tests vc1⊔vc2 == vc2⊔vc1.
func TestVectorClockJoinCommutativity(t *testing.T) {
	// Create two vector clocks with different values.
	vc1 := New()
	vc1.Set(0, 10)
	vc1.Set(1, 30)
	vc1.Set(2, 20)

	vc2 := New()
	vc2.Set(0, 5)
	vc2.Set(1, 40)
	vc2.Set(2, 15)

	// Clone for second test.
	vc1Copy := vc1.Clone()
	vc2Copy := vc2.Clone()

	// Test: vc1 ⊔ vc2.
	vc1.Join(vc2)

	// Test: vc2 ⊔ vc1 (reversed order).
	vc2Copy.Join(vc1Copy)

	// Results should be identical (commutativity).
	// v0.3.0: Use sparse-aware iteration up to maxTID.
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	limit := uint32(vc1.maxTID)
	if uint32(vc2Copy.maxTID) > limit {
		limit = uint32(vc2Copy.maxTID)
	}
	for i := uint32(0); i <= limit; i++ {
		if vc1.clocks[i] != vc2Copy.clocks[i] {
			t.Errorf("Join not commutative at index %d: vc1⊔vc2[%d]=%d, vc2⊔vc1[%d]=%d",
				i, i, vc1.clocks[i], i, vc2Copy.clocks[i])
		}
	}

	// Verify expected maximums.
	expected := map[uint16]uint32{
		0: 10, // max(10, 5)
		1: 40, // max(30, 40)
		2: 20, // max(20, 15)
	}

	for tid, want := range expected {
		if vc1.Get(tid) != want {
			t.Errorf("Join result[%d] = %d, want %d", tid, vc1.Get(tid), want)
		}
	}
}

// TestVectorClockJoinIdempotent tests vc⊔vc == vc.
func TestVectorClockJoinIdempotent(t *testing.T) {
	vc := New()
	vc.Set(0, 10)
	vc.Set(1, 20)
	vc.Set(5, 30)

	// Clone to compare later.
	original := vc.Clone()

	// Join with itself.
	vc.Join(vc)

	// Should be unchanged.
	// v0.3.0: Use sparse-aware iteration up to maxTID.
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(vc.maxTID); i++ {
		if vc.clocks[i] != original.clocks[i] {
			t.Errorf("Join not idempotent at index %d: vc⊔vc[%d]=%d, original[%d]=%d",
				i, i, vc.clocks[i], i, original.clocks[i])
		}
	}
}

// TestVectorClockPartialOrder tests transitivity: vc1⊑vc2 and vc2⊑vc3 => vc1⊑vc3.
func TestVectorClockPartialOrder(t *testing.T) {
	// Create three vector clocks: vc1 ⊑ vc2 ⊑ vc3.
	vc1 := New()
	vc1.Set(0, 10)
	vc1.Set(1, 20)
	vc1.Set(2, 30)

	vc2 := New()
	vc2.Set(0, 15) // >= vc1[0]
	vc2.Set(1, 25) // >= vc1[1]
	vc2.Set(2, 35) // >= vc1[2]

	vc3 := New()
	vc3.Set(0, 20) // >= vc2[0]
	vc3.Set(1, 30) // >= vc2[1]
	vc3.Set(2, 40) // >= vc2[2]

	// Test vc1 ⊑ vc2.
	if !vc1.LessOrEqual(vc2) {
		t.Error("vc1 ⊑ vc2 should be true")
	}

	// Test vc2 ⊑ vc3.
	if !vc2.LessOrEqual(vc3) {
		t.Error("vc2 ⊑ vc3 should be true")
	}

	// Test transitivity: vc1 ⊑ vc3.
	if !vc1.LessOrEqual(vc3) {
		t.Error("Transitivity failed: vc1 ⊑ vc2 and vc2 ⊑ vc3, but vc1 ⊑ vc3 is false")
	}

	// Test reflexivity: vc1 ⊑ vc1.
	if !vc1.LessOrEqual(vc1) {
		t.Error("Reflexivity failed: vc1 ⊑ vc1 should be true")
	}

	// Test non-comparable clocks (concurrent).
	vc4 := New()
	vc4.Set(0, 5)  // < vc1[0]
	vc4.Set(1, 25) // > vc1[1]

	if vc4.LessOrEqual(vc1) {
		t.Error("vc4 ⊑ vc1 should be false (vc4[1] > vc1[1])")
	}
	if vc1.LessOrEqual(vc4) {
		t.Error("vc1 ⊑ vc4 should be false (vc1[0] > vc4[0])")
	}
}

// TestVectorClockGetSet tests Get/Set operations.
func TestVectorClockGetSet(t *testing.T) {
	vc := New()

	// Test setting and getting various thread IDs.
	tests := []struct {
		tid   uint16
		clock uint32
	}{
		{0, 100},
		{1, 200},
		{127, 300},
		{255, 400},
	}

	for _, tt := range tests {
		vc.Set(tt.tid, tt.clock)
		got := vc.Get(tt.tid)
		if got != tt.clock {
			t.Errorf("Set(%d, %d) then Get(%d) = %d, want %d",
				tt.tid, tt.clock, tt.tid, got, tt.clock)
		}
	}

	// Test that other threads remain 0.
	if vc.Get(5) != 0 {
		t.Errorf("Untouched thread Get(5) = %d, want 0", vc.Get(5))
	}
}

// TestVectorClockIncrement tests Increment operation.
func TestVectorClockIncrement(t *testing.T) {
	vc := New()

	// Increment thread 0 multiple times.
	for i := 1; i <= 10; i++ {
		vc.Increment(0)
		got := vc.Get(0)
		if got != uint32(i) {
			t.Errorf("After %d increments, Get(0) = %d, want %d", i, got, i)
		}
	}

	// Increment thread 5.
	vc.Increment(5)
	if vc.Get(5) != 1 {
		t.Errorf("Increment(5) then Get(5) = %d, want 1", vc.Get(5))
	}

	// Verify thread 0 is unchanged.
	if vc.Get(0) != 10 {
		t.Errorf("Thread 0 changed after incrementing thread 5: Get(0) = %d, want 10", vc.Get(0))
	}

	// Test increment from non-zero value.
	vc.Set(10, 99)
	vc.Increment(10)
	if vc.Get(10) != 100 {
		t.Errorf("Increment from 99: Get(10) = %d, want 100", vc.Get(10))
	}
}

// TestVectorClockString tests debug output.
func TestVectorClockString(t *testing.T) {
	tests := []struct {
		name string
		set  map[uint16]uint32
		want string
	}{
		{
			name: "empty",
			set:  map[uint16]uint32{},
			want: "{}",
		},
		{
			name: "single thread",
			set:  map[uint16]uint32{0: 42},
			want: "{0:42}",
		},
		{
			name: "multiple threads",
			set:  map[uint16]uint32{0: 10, 5: 20, 65535: 30},
			want: "{0:10, 5:20, 65535:30}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc := New()
			for tid, clock := range tt.set {
				vc.Set(tid, clock)
			}
			got := vc.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestVectorClockJoinEdgeCases tests edge cases for Join.
func TestVectorClockJoinEdgeCases(t *testing.T) {
	t.Run("join with zero", func(t *testing.T) {
		vc1 := New()
		vc1.Set(0, 10)
		vc1.Set(1, 20)

		vc2 := New() // All zeros.

		vc1.Join(vc2)

		// vc1 should be unchanged (max with 0).
		if vc1.Get(0) != 10 || vc1.Get(1) != 20 {
			t.Errorf("Join with zero changed vc1: {0:%d, 1:%d}, want {0:10, 1:20}",
				vc1.Get(0), vc1.Get(1))
		}
	})

	t.Run("join zero with non-zero", func(t *testing.T) {
		vc1 := New() // All zeros.

		vc2 := New()
		vc2.Set(0, 10)
		vc2.Set(1, 20)

		vc1.Join(vc2)

		// vc1 should now equal vc2.
		if vc1.Get(0) != 10 || vc1.Get(1) != 20 {
			t.Errorf("Join zero with non-zero: {0:%d, 1:%d}, want {0:10, 1:20}",
				vc1.Get(0), vc1.Get(1))
		}
	})

	t.Run("join with max uint32", func(t *testing.T) {
		vc1 := New()
		vc1.Set(0, 100)

		vc2 := New()
		vc2.Set(0, 0xFFFFFFFF) // Max uint32.

		vc1.Join(vc2)

		if vc1.Get(0) != 0xFFFFFFFF {
			t.Errorf("Join with max uint32: Get(0) = %d, want %d", vc1.Get(0), 0xFFFFFFFF)
		}
	})
}

// TestVectorClockLessOrEqualEdgeCases tests edge cases for LessOrEqual.
func TestVectorClockLessOrEqualEdgeCases(t *testing.T) {
	t.Run("zero less or equal zero", func(t *testing.T) {
		vc1 := New()
		vc2 := New()

		if !vc1.LessOrEqual(vc2) {
			t.Error("Zero ⊑ Zero should be true")
		}
	})

	t.Run("zero less or equal non-zero", func(t *testing.T) {
		vc1 := New()
		vc2 := New()
		vc2.Set(0, 10)

		if !vc1.LessOrEqual(vc2) {
			t.Error("Zero ⊑ Non-Zero should be true")
		}
	})

	t.Run("non-zero not less or equal zero", func(t *testing.T) {
		vc1 := New()
		vc1.Set(0, 10)
		vc2 := New()

		if vc1.LessOrEqual(vc2) {
			t.Error("Non-Zero ⊑ Zero should be false")
		}
	})

	t.Run("equal clocks", func(t *testing.T) {
		vc1 := New()
		vc1.Set(0, 10)
		vc1.Set(1, 20)

		vc2 := New()
		vc2.Set(0, 10)
		vc2.Set(1, 20)

		if !vc1.LessOrEqual(vc2) {
			t.Error("Equal ⊑ Equal should be true")
		}
	})
}

// ========== BENCHMARKS ==========

// BenchmarkVectorClockJoin benchmarks the Join operation.
// Target: < 500ns/op, 0 allocs/op.
func BenchmarkVectorClockJoin(b *testing.B) {
	vc1 := New()
	vc2 := New()

	// Set up some realistic values.
	for i := 0; i < 10; i++ {
		vc1.Set(uint16(i), uint32(i*10))
		vc2.Set(uint16(i), uint32(i*15))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc1.Join(vc2)
	}
}

// BenchmarkVectorClockLessOrEqual benchmarks the LessOrEqual operation.
// Target: < 300ns/op, 0 allocs/op.
func BenchmarkVectorClockLessOrEqual(b *testing.B) {
	vc1 := New()
	vc2 := New()

	// Set up partial order: vc1 ⊑ vc2.
	for i := 0; i < 10; i++ {
		vc1.Set(uint16(i), uint32(i*10))
		vc2.Set(uint16(i), uint32(i*20))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vc1.LessOrEqual(vc2)
	}
}

// BenchmarkVectorClockClone benchmarks the Clone operation.
// Target: < 200ns/op, 1 alloc/op (for the new VC pointer).
func BenchmarkVectorClockClone(b *testing.B) {
	vc := New()

	// Set up some values.
	for i := 0; i < 10; i++ {
		vc.Set(uint16(i), uint32(i*10))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vc.Clone()
	}
}

// BenchmarkVectorClockIncrement benchmarks the Increment operation.
func BenchmarkVectorClockIncrement(b *testing.B) {
	vc := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Increment(0)
	}
}

// BenchmarkVectorClockGetSet benchmarks Get and Set operations.
func BenchmarkVectorClockGetSet(b *testing.B) {
	vc := New()

	b.Run("Get", func(b *testing.B) {
		vc.Set(0, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = vc.Get(0)
		}
	})

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			vc.Set(0, uint32(i))
		}
	})
}

// ========== POOLING TESTS ==========

// TestVectorClockPooling tests sync.Pool lifecycle.
func TestVectorClockPooling(t *testing.T) {
	t.Run("NewFromPool returns clean VectorClock", func(t *testing.T) {
		vc := NewFromPool()
		defer vc.Release()

		// Verify all clocks are zero.
		for i := 0; i < 10; i++ {
			if vc.Get(uint16(i)) != 0 {
				t.Errorf("NewFromPool() Get(%d) = %d, want 0", i, vc.Get(uint16(i)))
			}
		}

		// Verify maxTID is 0.
		if vc.GetMaxTID() != 0 {
			t.Errorf("NewFromPool() GetMaxTID() = %d, want 0", vc.GetMaxTID())
		}
	})

	t.Run("Release returns to pool and reuses", func(t *testing.T) {
		// Get first VC from pool, set some values.
		vc1 := NewFromPool()
		vc1.Set(0, 100)
		vc1.Set(5, 200)
		vc1.Set(100, 300)

		// Release back to pool.
		vc1.Release()

		// Get second VC from pool - should be the same object but reset.
		vc2 := NewFromPool()
		defer vc2.Release()

		// Verify it's clean (Reset was called).
		if vc2.Get(0) != 0 {
			t.Errorf("Reused VC Get(0) = %d, want 0 (should be reset)", vc2.Get(0))
		}
		if vc2.Get(5) != 0 {
			t.Errorf("Reused VC Get(5) = %d, want 0 (should be reset)", vc2.Get(5))
		}
		if vc2.GetMaxTID() != 0 {
			t.Errorf("Reused VC GetMaxTID() = %d, want 0 (should be reset)", vc2.GetMaxTID())
		}
	})

	t.Run("Release on nil is safe", func(_ *testing.T) {
		var vc *VectorClock
		// Should not panic.
		vc.Release()
	})

	t.Run("Multiple Release is safe", func(_ *testing.T) {
		vc := NewFromPool()
		vc.Release()
		vc.Release() // Second release should be safe (no-op).
	})
}

// TestVectorClockReset tests Reset method.
//
//nolint:gocognit // Test function with multiple sub-tests
func TestVectorClockReset(t *testing.T) {
	t.Run("Reset clears all values", func(t *testing.T) {
		vc := New()
		vc.Set(0, 10)
		vc.Set(5, 20)
		vc.Set(100, 30)
		vc.Set(65535, 40)

		vc.Reset()

		// Verify all values are zero.
		if vc.Get(0) != 0 {
			t.Errorf("After Reset, Get(0) = %d, want 0", vc.Get(0))
		}
		if vc.Get(5) != 0 {
			t.Errorf("After Reset, Get(5) = %d, want 0", vc.Get(5))
		}
		if vc.Get(100) != 0 {
			t.Errorf("After Reset, Get(100) = %d, want 0", vc.Get(100))
		}
		if vc.Get(65535) != 0 {
			t.Errorf("After Reset, Get(65535) = %d, want 0", vc.Get(65535))
		}

		// Verify maxTID is reset to 0.
		if vc.GetMaxTID() != 0 {
			t.Errorf("After Reset, GetMaxTID() = %d, want 0", vc.GetMaxTID())
		}
	})

	t.Run("Reset is sparse-aware", func(t *testing.T) {
		vc := New()
		// Set only first 10 elements.
		for i := uint16(0); i < 10; i++ {
			vc.Set(i, uint32(i*10))
		}

		// maxTID should be 9.
		if vc.GetMaxTID() != 9 {
			t.Errorf("Before Reset, GetMaxTID() = %d, want 9", vc.GetMaxTID())
		}

		vc.Reset()

		// After reset, maxTID should be 0.
		if vc.GetMaxTID() != 0 {
			t.Errorf("After Reset, GetMaxTID() = %d, want 0", vc.GetMaxTID())
		}

		// All values should be 0.
		for i := uint16(0); i < 10; i++ {
			if vc.Get(i) != 0 {
				t.Errorf("After Reset, Get(%d) = %d, want 0", i, vc.Get(i))
			}
		}
	})
}

// ========== POOLING BENCHMARKS ==========

// BenchmarkVectorClockPooling benchmarks pool vs direct allocation.
func BenchmarkVectorClockPooling(b *testing.B) {
	b.Run("New", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			vc := New()
			vc.Set(0, 1)
			_ = vc.Get(0)
		}
	})

	b.Run("NewFromPool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			vc := NewFromPool()
			vc.Set(0, 1)
			_ = vc.Get(0)
			vc.Release()
		}
	})

	b.Run("NewFromPool_NoRelease", func(b *testing.B) {
		// Simulate leak - no Release().
		// This should be same as New() performance-wise.
		for i := 0; i < b.N; i++ {
			vc := NewFromPool()
			vc.Set(0, 1)
			_ = vc.Get(0)
			// Intentionally not calling Release().
		}
	})
}

// BenchmarkVectorClockReset benchmarks Reset operation.
// Target: < 100ns for typical sparse clocks.
//
//nolint:gocognit // Benchmark function with multiple sub-benchmarks
func BenchmarkVectorClockReset(b *testing.B) {
	b.Run("Reset_Sparse10", func(b *testing.B) {
		vc := New()
		// Set 10 elements (typical case).
		for i := uint16(0); i < 10; i++ {
			vc.Set(i, uint32(i*10))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vc.Reset()
			// Re-set for next iteration.
			for j := uint16(0); j < 10; j++ {
				vc.Set(j, uint32(j*10))
			}
		}
	})

	b.Run("Reset_Sparse100", func(b *testing.B) {
		vc := New()
		// Set 100 elements.
		for i := uint16(0); i < 100; i++ {
			vc.Set(i, uint32(i*10))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vc.Reset()
			// Re-set for next iteration.
			for j := uint16(0); j < 100; j++ {
				vc.Set(j, uint32(j*10))
			}
		}
	})

	b.Run("Reset_Dense", func(b *testing.B) {
		vc := New()
		// Set all 65536 elements (worst case).
		for i := uint16(0); i < 65535; i++ {
			vc.Set(i, uint32(i))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vc.Reset()
			// Re-set for next iteration.
			for j := uint16(0); j < 65535; j++ {
				vc.Set(j, uint32(j))
			}
		}
	})
}
