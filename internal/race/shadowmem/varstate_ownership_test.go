package shadowmem

import (
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
)

// TestVarState_OwnershipInit tests initial state of ownership fields.
func TestVarState_OwnershipInit(t *testing.T) {
	vs := NewVarState()

	if vs.GetExclusiveWriter() != 0 {
		t.Errorf("Expected exclusiveWriter = 0 (uninitialized), got %d", vs.GetExclusiveWriter())
	}

	if vs.GetWriteCount() != 0 {
		t.Errorf("Expected writeCount = 0, got %d", vs.GetWriteCount())
	}

	if !vs.IsOwned() {
		t.Error("Expected IsOwned() = true for uninitialized state (exclusiveWriter = 0 is >= 0)")
	}
}

// TestVarState_ClaimOwnership tests first writer claiming ownership.
func TestVarState_ClaimOwnership(t *testing.T) {
	vs := NewVarState()

	// First writer (TID=1) claims ownership
	vs.SetExclusiveWriter(1)

	if vs.GetExclusiveWriter() != 1 {
		t.Errorf("Expected exclusiveWriter = 1, got %d", vs.GetExclusiveWriter())
	}

	if !vs.IsOwned() {
		t.Error("Expected IsOwned() = true after claiming ownership")
	}
}

// TestVarState_PromoteToShared tests promotion from single to multiple writers.
func TestVarState_PromoteToShared(t *testing.T) {
	vs := NewVarState()

	// First writer claims ownership
	vs.SetExclusiveWriter(1)

	// Second writer detected - promote to shared
	vs.SetExclusiveWriter(-1)

	if vs.GetExclusiveWriter() != -1 {
		t.Errorf("Expected exclusiveWriter = -1 (shared), got %d", vs.GetExclusiveWriter())
	}

	if vs.IsOwned() {
		t.Error("Expected IsOwned() = false for shared state")
	}
}

// TestVarState_WriteCount tests write counter tracking.
func TestVarState_WriteCount(t *testing.T) {
	vs := NewVarState()

	if vs.GetWriteCount() != 0 {
		t.Errorf("Expected initial writeCount = 0, got %d", vs.GetWriteCount())
	}

	// Increment write count
	vs.IncrementWriteCount()
	if vs.GetWriteCount() != 1 {
		t.Errorf("Expected writeCount = 1, got %d", vs.GetWriteCount())
	}

	// Increment again
	vs.IncrementWriteCount()
	if vs.GetWriteCount() != 2 {
		t.Errorf("Expected writeCount = 2, got %d", vs.GetWriteCount())
	}
}

// TestVarState_Reset_ClearsOwnership tests that Reset() clears ownership fields.
func TestVarState_Reset_ClearsOwnership(t *testing.T) {
	vs := NewVarState()

	// Set ownership state
	vs.SetExclusiveWriter(5)
	vs.IncrementWriteCount()
	vs.IncrementWriteCount()
	vs.SetW(epoch.NewEpoch(5, 100))

	// Reset
	vs.Reset()

	// Verify ownership fields are cleared
	if vs.GetExclusiveWriter() != 0 {
		t.Errorf("Expected exclusiveWriter = 0 after Reset(), got %d", vs.GetExclusiveWriter())
	}

	if vs.GetWriteCount() != 0 {
		t.Errorf("Expected writeCount = 0 after Reset(), got %d", vs.GetWriteCount())
	}

	if vs.GetW() != 0 {
		t.Errorf("Expected W = 0 after Reset(), got %v", vs.GetW())
	}
}

// TestVarState_OwnershipTransitions tests ownership lifecycle.
func TestVarState_OwnershipTransitions(t *testing.T) {
	vs := NewVarState()

	// State 1: Uninitialized (exclusiveWriter = 0)
	if vs.GetExclusiveWriter() != 0 {
		t.Errorf("State 1: Expected exclusiveWriter = 0, got %d", vs.GetExclusiveWriter())
	}
	if !vs.IsOwned() {
		t.Error("State 1: Expected IsOwned() = true")
	}

	// State 2: First writer (TID=3) claims ownership
	vs.SetExclusiveWriter(3)
	if vs.GetExclusiveWriter() != 3 {
		t.Errorf("State 2: Expected exclusiveWriter = 3, got %d", vs.GetExclusiveWriter())
	}
	if !vs.IsOwned() {
		t.Error("State 2: Expected IsOwned() = true")
	}

	// State 3: Second writer (TID=7) detected - promote to shared
	vs.SetExclusiveWriter(-1)
	if vs.GetExclusiveWriter() != -1 {
		t.Errorf("State 3: Expected exclusiveWriter = -1, got %d", vs.GetExclusiveWriter())
	}
	if vs.IsOwned() {
		t.Error("State 3: Expected IsOwned() = false")
	}

	// State 4: Shared state persists (cannot demote back)
	vs.SetExclusiveWriter(-1) // Stays shared
	if vs.GetExclusiveWriter() != -1 {
		t.Errorf("State 4: Expected exclusiveWriter = -1, got %d", vs.GetExclusiveWriter())
	}
}

// TestVarState_ConcurrentOwnershipAccess tests thread-safe ownership access.
func TestVarState_ConcurrentOwnershipAccess(t *testing.T) {
	vs := NewVarState()

	// This test verifies that ownership methods use mutex protection.
	// We'll launch concurrent goroutines accessing ownership fields.

	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	// Concurrent reads/writes to ownership fields
	for i := 0; i < numGoroutines; i++ {
		go func(tid int64) {
			vs.SetExclusiveWriter(tid)
			_ = vs.GetExclusiveWriter()
			vs.IncrementWriteCount()
			_ = vs.GetWriteCount()
			_ = vs.IsOwned()
			done <- true
		}(int64(i))
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final state is consistent (should be shared or last writer)
	finalWriter := vs.GetExclusiveWriter()
	if finalWriter != -1 && (finalWriter < 0 || finalWriter >= numGoroutines) {
		t.Errorf("Expected exclusiveWriter in range [0, %d] or -1, got %d", numGoroutines-1, finalWriter)
	}

	// Write count should be numGoroutines
	if vs.GetWriteCount() != numGoroutines {
		t.Errorf("Expected writeCount = %d, got %d", numGoroutines, vs.GetWriteCount())
	}
}

// TestVarState_OwnershipEdgeCases tests edge cases for ownership tracking.
func TestVarState_OwnershipEdgeCases(t *testing.T) {
	t.Run("MaxTID", func(t *testing.T) {
		vs := NewVarState()
		vs.SetExclusiveWriter(255) // Max uint8 TID
		if vs.GetExclusiveWriter() != 255 {
			t.Errorf("Expected exclusiveWriter = 255, got %d", vs.GetExclusiveWriter())
		}
	})

	t.Run("NegativeTID", func(t *testing.T) {
		vs := NewVarState()
		vs.SetExclusiveWriter(-1) // Shared state
		if vs.GetExclusiveWriter() != -1 {
			t.Errorf("Expected exclusiveWriter = -1, got %d", vs.GetExclusiveWriter())
		}
		if vs.IsOwned() {
			t.Error("Expected IsOwned() = false for negative TID")
		}
	})

	t.Run("WriteCountOverflow", func(t *testing.T) {
		vs := NewVarState()
		// Increment to near overflow (uint32 max)
		vs.writeCount = 4294967290 // Close to uint32 max (4294967295)
		vs.IncrementWriteCount()
		vs.IncrementWriteCount()
		vs.IncrementWriteCount()
		vs.IncrementWriteCount()
		vs.IncrementWriteCount()
		// Should wrap around (acceptable for statistics)
		if vs.GetWriteCount() == 4294967290 {
			t.Error("WriteCount did not increment")
		}
	})
}

// TestVarState_OwnershipWithAdaptiveRepresentation tests ownership with read promotion.
func TestVarState_OwnershipWithAdaptiveRepresentation(t *testing.T) {
	vs := NewVarState()

	// Set ownership
	vs.SetExclusiveWriter(1)
	vs.IncrementWriteCount()

	// Note: We don't actually promote here because PromoteToReadClock requires a valid VectorClock.
	// This test just verifies that ownership fields are independent of read tracking.
	// The actual integration test will test promotion with ownership tracking.

	// Verify ownership is set
	if vs.GetExclusiveWriter() != 1 {
		t.Errorf("Expected exclusiveWriter = 1, got %d", vs.GetExclusiveWriter())
	}

	if vs.GetWriteCount() != 1 {
		t.Errorf("Expected writeCount = 1, got %d", vs.GetWriteCount())
	}

	// Demote (write occurs)
	vs.Demote()

	// Verify ownership still persists after demotion
	if vs.GetExclusiveWriter() != 1 {
		t.Errorf("Expected exclusiveWriter = 1 after demotion, got %d", vs.GetExclusiveWriter())
	}
}

// BenchmarkVarState_GetExclusiveWriter benchmarks ownership read performance.
func BenchmarkVarState_GetExclusiveWriter(b *testing.B) {
	vs := NewVarState()
	vs.SetExclusiveWriter(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vs.GetExclusiveWriter()
	}
}

// BenchmarkVarState_SetExclusiveWriter benchmarks ownership write performance.
func BenchmarkVarState_SetExclusiveWriter(b *testing.B) {
	vs := NewVarState()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vs.SetExclusiveWriter(int64(i % 256))
	}
}

// BenchmarkVarState_IncrementWriteCount benchmarks write counter performance.
func BenchmarkVarState_IncrementWriteCount(b *testing.B) {
	vs := NewVarState()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vs.IncrementWriteCount()
	}
}

// BenchmarkVarState_IsOwned benchmarks ownership check performance.
func BenchmarkVarState_IsOwned(b *testing.B) {
	vs := NewVarState()
	vs.SetExclusiveWriter(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vs.IsOwned()
	}
}
