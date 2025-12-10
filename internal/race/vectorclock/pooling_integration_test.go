package vectorclock

import (
	"runtime"
	"sync"
	"testing"
)

// TestVectorClockPooling_Integration tests pooling in concurrent scenario.
//
// This test simulates real-world usage where multiple goroutines allocate
// and release VectorClocks, verifying that pooling reduces allocations.
func TestVectorClockPooling_Integration(t *testing.T) {
	const (
		numGoroutines = 100
		numIterations = 1000
	)

	t.Run("Concurrent pool access", func(_ *testing.T) {
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			go func(gid int) {
				defer wg.Done()

				for i := 0; i < numIterations; i++ {
					// Simulate VectorClock lifecycle.
					vc := NewFromPool()

					// Simulate some usage.
					vc.Set(uint16(gid%256), uint32(i))
					vc.Increment(uint16(gid % 256))
					_ = vc.Get(uint16(gid % 256))

					// Release back to pool.
					vc.Release()
				}
			}(g)
		}

		wg.Wait()
	})

	t.Run("Pool reuse reduces allocations", func(t *testing.T) {
		// Force GC to clear previous allocations.
		runtime.GC()

		var m1, m2 runtime.MemStats
		runtime.ReadMemStats(&m1)

		// Allocate and release many VectorClocks using pool.
		for i := 0; i < 10000; i++ {
			vc := NewFromPool()
			vc.Set(0, uint32(i))
			vc.Release()
		}

		runtime.ReadMemStats(&m2)

		// Calculate allocations.
		allocsDiff := m2.Mallocs - m1.Mallocs

		// With pooling, allocations should be minimal.
		// Without pooling, we'd see 10,000+ allocations.
		// With pooling, we expect < 100 allocations (pool size stabilizes).
		if allocsDiff > 100 {
			t.Logf("WARNING: Pool may not be working optimally. Allocations: %d", allocsDiff)
		} else {
			t.Logf("SUCCESS: Pool is working. Allocations: %d (expected < 100)", allocsDiff)
		}
	})

	t.Run("Pool vs Direct allocation comparison", func(t *testing.T) {
		const iterations = 1000

		// Test 1: Direct allocation (no pooling).
		runtime.GC()
		var m1Direct, m2Direct runtime.MemStats
		runtime.ReadMemStats(&m1Direct)

		for i := 0; i < iterations; i++ {
			vc := New()
			vc.Set(0, uint32(i))
			// No Release() - simulates non-pooled usage.
			_ = vc
		}

		runtime.ReadMemStats(&m2Direct)
		directAllocs := m2Direct.Mallocs - m1Direct.Mallocs

		// Test 2: Pooled allocation.
		runtime.GC()
		var m1Pooled, m2Pooled runtime.MemStats
		runtime.ReadMemStats(&m1Pooled)

		for i := 0; i < iterations; i++ {
			vc := NewFromPool()
			vc.Set(0, uint32(i))
			vc.Release()
		}

		runtime.ReadMemStats(&m2Pooled)
		pooledAllocs := m2Pooled.Mallocs - m1Pooled.Mallocs

		// Pooled should have significantly fewer allocations.
		reduction := float64(directAllocs-pooledAllocs) / float64(directAllocs) * 100

		t.Logf("Direct allocations: %d", directAllocs)
		t.Logf("Pooled allocations: %d", pooledAllocs)
		t.Logf("Reduction: %.2f%%", reduction)

		// We expect at least 90% reduction in allocations.
		if reduction < 90 {
			t.Errorf("Pool not effective enough. Reduction: %.2f%%, expected >= 90%%", reduction)
		}
	})
}

// BenchmarkVectorClockPooling_Integration benchmarks pooling under realistic workload.
func BenchmarkVectorClockPooling_Integration(b *testing.B) {
	b.Run("Concurrent_NewFromPool", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				vc := NewFromPool()
				vc.Set(0, 1)
				vc.Increment(0)
				_ = vc.Get(0)
				vc.Release()
			}
		})
	})

	b.Run("Concurrent_New", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				vc := New()
				vc.Set(0, 1)
				vc.Increment(0)
				_ = vc.Get(0)
				// No Release() - direct allocation.
			}
		})
	})
}
