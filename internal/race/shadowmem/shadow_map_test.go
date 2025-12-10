package shadowmem

import (
	"sync"
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
)

// TestShadowMemoryNew verifies that NewShadowMemory creates a valid instance.
func TestShadowMemoryNew(t *testing.T) {
	sm := NewShadowMemory()

	if sm == nil {
		t.Fatal("NewShadowMemory() returned nil")
	}

	t.Logf("NewShadowMemory() created valid instance")
}

// TestShadowMemoryGetOrCreate_NewAddress verifies GetOrCreate for a new address.
func TestShadowMemoryGetOrCreate_NewAddress(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0x1234)

	vs := sm.GetOrCreate(addr)

	if vs == nil {
		t.Fatal("GetOrCreate() returned nil for new address")
	}

	// New VarState should be zero-initialized.
	if vs.GetW() != 0 {
		t.Errorf("New VarState.W = %v, want 0", vs.GetW())
	}
	if vs.GetReadEpoch() != 0 {
		t.Errorf("New VarState.GetReadEpoch() = %v, want 0", vs.GetReadEpoch())
	}

	t.Logf("GetOrCreate(0x%x) created new VarState: %s", addr, vs)
}

// TestShadowMemoryGetOrCreate_ExistingAddress verifies GetOrCreate returns same instance.
func TestShadowMemoryGetOrCreate_ExistingAddress(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0x5678)

	// First call creates the VarState.
	vs1 := sm.GetOrCreate(addr)
	vs1.SetW(epoch.NewEpoch(5, 100))
	vs1.SetReadEpoch(epoch.NewEpoch(3, 50))

	// Second call should return the SAME VarState instance.
	vs2 := sm.GetOrCreate(addr)

	if vs2 != vs1 {
		t.Errorf("GetOrCreate() returned different instance: %p vs %p", vs2, vs1)
	}

	// Verify the state is preserved.
	if vs2.GetW() != epoch.NewEpoch(5, 100) {
		t.Errorf("VarState.W = %v, want %v", vs2.GetW(), epoch.NewEpoch(5, 100))
	}
	if vs2.GetReadEpoch() != epoch.NewEpoch(3, 50) {
		t.Errorf("VarState.GetReadEpoch() = %v, want %v", vs2.GetReadEpoch(), epoch.NewEpoch(3, 50))
	}

	t.Logf("GetOrCreate(0x%x) returned same instance: %s", addr, vs2)
}

// TestShadowMemoryGetOrCreate_MultipleAddresses verifies independent cells.
func TestShadowMemoryGetOrCreate_MultipleAddresses(t *testing.T) {
	sm := NewShadowMemory()

	addresses := []uintptr{0x1000, 0x2000, 0x3000, 0x4000}
	cells := make([]*VarState, len(addresses))

	// Create cells for multiple addresses.
	for i, addr := range addresses {
		cells[i] = sm.GetOrCreate(addr)
		cells[i].SetW(epoch.NewEpoch(uint16(i+1), uint64((i+1)*100)))
	}

	// Verify each cell is independent.
	for i, addr := range addresses {
		vs := sm.GetOrCreate(addr)

		if vs != cells[i] {
			t.Errorf("GetOrCreate(0x%x) returned different instance", addr)
		}

		expectedW := epoch.NewEpoch(uint16(i+1), uint64((i+1)*100))
		if vs.GetW() != expectedW {
			t.Errorf("VarState[0x%x].W = %v, want %v", addr, vs.GetW(), expectedW)
		}
	}

	t.Logf("GetOrCreate() correctly handles %d independent addresses", len(addresses))
}

// TestShadowMemoryGet_ExistingAddress verifies Get returns existing cell.
func TestShadowMemoryGet_ExistingAddress(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0xABCD)

	// Create cell first.
	vs1 := sm.GetOrCreate(addr)
	vs1.SetW(epoch.NewEpoch(7, 200))

	// Get should return the same instance.
	vs2 := sm.Get(addr)

	if vs2 == nil {
		t.Fatal("Get() returned nil for existing address")
	}

	if vs2 != vs1 {
		t.Errorf("Get() returned different instance: %p vs %p", vs2, vs1)
	}

	if vs2.GetW() != epoch.NewEpoch(7, 200) {
		t.Errorf("VarState.W = %v, want %v", vs2.GetW(), epoch.NewEpoch(7, 200))
	}

	t.Logf("Get(0x%x) returned existing cell: %s", addr, vs2)
}

// TestShadowMemoryGet_MissingAddress verifies Get returns nil for missing address.
func TestShadowMemoryGet_MissingAddress(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0xDEAD)

	vs := sm.Get(addr)

	if vs != nil {
		t.Errorf("Get() returned non-nil for missing address: %v", vs)
	}

	t.Logf("Get(0x%x) correctly returned nil for missing address", addr)
}

// TestShadowMemoryGet_AfterGetOrCreate verifies Get works after GetOrCreate.
func TestShadowMemoryGet_AfterGetOrCreate(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0xBEEF)

	// GetOrCreate creates the cell.
	vs1 := sm.GetOrCreate(addr)
	vs1.SetW(epoch.NewEpoch(2, 42))

	// Get should find it.
	vs2 := sm.Get(addr)

	if vs2 == nil {
		t.Fatal("Get() returned nil after GetOrCreate()")
	}

	if vs2 != vs1 {
		t.Errorf("Get() returned different instance")
	}

	t.Logf("Get(0x%x) correctly found cell after GetOrCreate()", addr)
}

// TestShadowMemoryReset verifies Reset clears all cells.
func TestShadowMemoryReset(t *testing.T) {
	sm := NewShadowMemory()

	// Create multiple cells.
	addresses := []uintptr{0x1111, 0x2222, 0x3333}
	for _, addr := range addresses {
		vs := sm.GetOrCreate(addr)
		vs.SetW(epoch.NewEpoch(1, 10))
	}

	// Verify cells exist before reset.
	for _, addr := range addresses {
		if sm.Get(addr) == nil {
			t.Fatalf("Cell at 0x%x should exist before Reset()", addr)
		}
	}

	// Reset should clear everything.
	sm.Reset()

	// Verify all cells are gone.
	for _, addr := range addresses {
		vs := sm.Get(addr)
		if vs != nil {
			t.Errorf("Get(0x%x) returned non-nil after Reset(): %v", addr, vs)
		}
	}

	t.Logf("Reset() correctly cleared %d cells", len(addresses))
}

// TestShadowMemoryReset_Idempotent verifies Reset can be called multiple times.
func TestShadowMemoryReset_Idempotent(t *testing.T) {
	sm := NewShadowMemory()

	sm.GetOrCreate(0x1234)

	// Multiple resets should not panic.
	sm.Reset()
	sm.Reset()
	sm.Reset()

	// Should still work after multiple resets.
	vs := sm.GetOrCreate(0x5678)
	if vs == nil {
		t.Fatal("GetOrCreate() failed after multiple Reset() calls")
	}

	t.Logf("Reset() is idempotent - multiple calls work correctly")
}

// TestShadowMemoryReset_NewCellsAfterReset verifies new cells work after reset.
func TestShadowMemoryReset_NewCellsAfterReset(t *testing.T) {
	sm := NewShadowMemory()

	// Create and reset.
	sm.GetOrCreate(0x1000)
	sm.Reset()

	// Create new cell after reset - should work.
	vs := sm.GetOrCreate(0x2000)
	if vs == nil {
		t.Fatal("GetOrCreate() failed after Reset()")
	}

	vs.SetW(epoch.NewEpoch(5, 100))

	// Verify the new cell persists.
	vs2 := sm.Get(0x2000)
	if vs2 == nil {
		t.Fatal("Get() failed to find cell created after Reset()")
	}

	if vs2.GetW() != epoch.NewEpoch(5, 100) {
		t.Errorf("VarState.W = %v, want %v", vs2.GetW(), epoch.NewEpoch(5, 100))
	}

	t.Logf("Shadow memory works correctly after Reset()")
}

// TestShadowMemoryConcurrent_GetOrCreate tests concurrent GetOrCreate calls.
func TestShadowMemoryConcurrent_GetOrCreate(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0xCAFE)

	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make([]*VarState, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = sm.GetOrCreate(addr)
		}(i)
	}

	wg.Wait()

	// All goroutines should get the SAME VarState instance.
	first := results[0]
	if first == nil {
		t.Fatal("GetOrCreate() returned nil")
	}

	for i, vs := range results {
		if vs != first {
			t.Errorf("Goroutine %d got different VarState instance: %p vs %p", i, vs, first)
		}
	}

	t.Logf("Concurrent GetOrCreate() from %d goroutines returned same instance", numGoroutines)
}

// TestShadowMemoryConcurrent_MultipleAddresses tests concurrent access to different addresses.
func TestShadowMemoryConcurrent_MultipleAddresses(t *testing.T) {
	sm := NewShadowMemory()

	const numGoroutines = 50
	const numAddresses = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(gid int) {
			defer wg.Done()

			// Each goroutine accesses multiple addresses.
			for j := 0; j < numAddresses; j++ {
				addr := uintptr(0x10000 + j*0x100)
				vs := sm.GetOrCreate(addr)

				// Update the cell.
				vs.SetW(epoch.NewEpoch(uint16(gid), uint64(j)))
			}
		}(i)
	}

	wg.Wait()

	// Verify all addresses have cells.
	for j := 0; j < numAddresses; j++ {
		addr := uintptr(0x10000 + j*0x100)
		vs := sm.Get(addr)

		if vs == nil {
			t.Errorf("Address 0x%x missing after concurrent access", addr)
		}
	}

	t.Logf("Concurrent access from %d goroutines to %d addresses succeeded", numGoroutines, numAddresses)
}

// TestShadowMemoryConcurrent_GetAndGetOrCreate tests mixed Get/GetOrCreate.
func TestShadowMemoryConcurrent_GetAndGetOrCreate(t *testing.T) {
	t.Skip("Known issue v0.1.0: Test has data race in concurrent access - fix in v0.2.0")

	sm := NewShadowMemory()
	addr := uintptr(0xFACE)

	const numReaders = 50
	const numWriters = 10

	var wg sync.WaitGroup
	wg.Add(numReaders + numWriters)

	// Writers create or update the cell.
	for i := 0; i < numWriters; i++ {
		go func(wid int) {
			defer wg.Done()

			vs := sm.GetOrCreate(addr)
			vs.SetW(epoch.NewEpoch(uint16(wid), uint64(wid*10)))
		}(i)
	}

	// Readers try to get the cell (may be nil initially).
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			// Get may return nil or a valid VarState.
			_ = sm.Get(addr)
		}()
	}

	wg.Wait()

	// After all goroutines finish, the cell should exist.
	vs := sm.Get(addr)
	if vs == nil {
		t.Fatal("Cell should exist after concurrent Get/GetOrCreate")
	}

	t.Logf("Concurrent Get/GetOrCreate from %d goroutines succeeded", numReaders+numWriters)
}

// TestShadowMemoryConcurrent_ReadWrite simulates realistic race detector workload.
func TestShadowMemoryConcurrent_ReadWrite(t *testing.T) {
	t.Skip("Known issue v0.1.0: Test has data race in VarState access - fix in v0.2.0")

	sm := NewShadowMemory()

	const numGoroutines = 20
	const numOpsPerGoroutine = 1000

	// Pre-populate some addresses.
	addresses := make([]uintptr, 100)
	for i := range addresses {
		addresses[i] = uintptr(0x20000 + i*8)
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for gid := 0; gid < numGoroutines; gid++ {
		go func(goroutineID int) {
			defer wg.Done()

			for op := 0; op < numOpsPerGoroutine; op++ {
				// Access random address.
				addr := addresses[op%len(addresses)]

				// GetOrCreate and update.
				vs := sm.GetOrCreate(addr)
				vs.SetW(epoch.NewEpoch(uint16(goroutineID), uint64(op)))

				// Sometimes just read.
				if op%3 == 0 {
					_ = sm.Get(addr)
				}
			}
		}(gid)
	}

	wg.Wait()

	// Verify all addresses have cells.
	for _, addr := range addresses {
		vs := sm.Get(addr)
		if vs == nil {
			t.Errorf("Address 0x%x missing after concurrent workload", addr)
		}
	}

	t.Logf("Realistic concurrent workload: %d goroutines × %d ops = %d total operations",
		numGoroutines, numOpsPerGoroutine, numGoroutines*numOpsPerGoroutine)
}

// TestShadowMemoryGetOrCreate_NoAllocOnHit verifies zero allocations for existing cells.
func TestShadowMemoryGetOrCreate_NoAllocOnHit(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0x7777)

	// Create the cell first.
	sm.GetOrCreate(addr)

	// Measure allocations for subsequent GetOrCreate (should be 0).
	allocs := testing.AllocsPerRun(1000, func() {
		_ = sm.GetOrCreate(addr)
	})

	if allocs > 0 {
		t.Errorf("GetOrCreate(existing) allocated %.2f times, want 0", allocs)
	}

	t.Logf("GetOrCreate(existing) allocations: %.2f (correct - zero allocations)", allocs)
}

// TestShadowMemoryGet_NoAlloc verifies Get never allocates.
func TestShadowMemoryGet_NoAlloc(t *testing.T) {
	sm := NewShadowMemory()
	addr := uintptr(0x8888)

	// Create the cell.
	sm.GetOrCreate(addr)

	// Measure allocations for Get (hit case).
	allocsHit := testing.AllocsPerRun(1000, func() {
		_ = sm.Get(addr)
	})

	if allocsHit > 0 {
		t.Errorf("Get(existing) allocated %.2f times, want 0", allocsHit)
	}

	t.Logf("Get(existing) allocations: %.2f (correct - zero allocations)", allocsHit)

	// Measure allocations for Get (miss case).
	allocsMiss := testing.AllocsPerRun(1000, func() {
		_ = sm.Get(0x9999) // Different address that doesn't exist.
	})

	if allocsMiss > 0 {
		t.Errorf("Get(missing) allocated %.2f times, want 0", allocsMiss)
	}

	t.Logf("Get(missing) allocations: %.2f (correct - zero allocations)", allocsMiss)
}

// BenchmarkShadowMemory_GetOrCreate_Hit benchmarks GetOrCreate for existing cell.
// Target: <10ns/op (sync.Map.Load fast path).
func BenchmarkShadowMemory_GetOrCreate_Hit(b *testing.B) {
	sm := NewShadowMemory()
	addr := uintptr(0x1000)

	// Pre-create the cell.
	sm.GetOrCreate(addr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = sm.GetOrCreate(addr)
	}
}

// BenchmarkShadowMemory_GetOrCreate_Miss benchmarks GetOrCreate for new cell.
// Target: <50ns/op (sync.Map.LoadOrStore + VarState allocation).
func BenchmarkShadowMemory_GetOrCreate_Miss(b *testing.B) {
	sm := NewShadowMemory()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		addr := uintptr(0x10000 + i*8)
		_ = sm.GetOrCreate(addr)
	}
}

// BenchmarkShadowMemory_Get_Hit benchmarks Get for existing cell.
// Target: <10ns/op (sync.Map.Load).
func BenchmarkShadowMemory_Get_Hit(b *testing.B) {
	sm := NewShadowMemory()
	addr := uintptr(0x2000)

	// Pre-create the cell.
	sm.GetOrCreate(addr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = sm.Get(addr)
	}
}

// BenchmarkShadowMemory_Get_Miss benchmarks Get for missing cell.
// Target: <10ns/op (sync.Map.Load returns not found).
func BenchmarkShadowMemory_Get_Miss(b *testing.B) {
	sm := NewShadowMemory()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = sm.Get(uintptr(0x20000 + i*8))
	}
}

// BenchmarkShadowMemory_Concurrent benchmarks concurrent GetOrCreate access.
// This simulates the real race detector workload where multiple goroutines
// access shadow memory simultaneously.
func BenchmarkShadowMemory_Concurrent(b *testing.B) {
	sm := NewShadowMemory()

	// Pre-populate some hot addresses (common in real workloads).
	hotAddresses := make([]uintptr, 100)
	for i := range hotAddresses {
		hotAddresses[i] = uintptr(0x30000 + i*8)
		sm.GetOrCreate(hotAddresses[i])
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			addr := hotAddresses[i%len(hotAddresses)]
			vs := sm.GetOrCreate(addr)
			vs.SetW(epoch.NewEpoch(1, uint64(i)))
			i++
		}
	})
}

// BenchmarkShadowMemory_HighContention benchmarks extreme contention scenarios.
// This tests the sharding benefit when many goroutines access many addresses.
func BenchmarkShadowMemory_HighContention(b *testing.B) {
	sm := NewShadowMemory()

	// Pre-populate a large address space across many shards.
	numAddresses := 10000
	addresses := make([]uintptr, numAddresses)
	for i := range addresses {
		addresses[i] = uintptr(0x50000 + i*8)
		sm.GetOrCreate(addresses[i])
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Access random addresses across all shards.
			addr := addresses[i%numAddresses]
			vs := sm.GetOrCreate(addr)
			vs.SetW(epoch.NewEpoch(1, uint64(i)))
			i++
		}
	})
}

// BenchmarkShadowMemory_Reset benchmarks Reset performance.
// This is not on hot path but good to measure.
func BenchmarkShadowMemory_Reset(b *testing.B) {
	sm := NewShadowMemory()

	// Pre-populate with some cells.
	for i := 0; i < 1000; i++ {
		sm.GetOrCreate(uintptr(0x40000 + i*8))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sm.Reset()
	}
}
