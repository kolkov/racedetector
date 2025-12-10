package shadowmem

import (
	"sync"
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
)

// ========================================
// Basic Functionality Tests
// ========================================

// TestCASBasedShadow_New verifies NewCASBasedShadow creates valid instance.
func TestCASBasedShadow_New(t *testing.T) {
	shadow := NewCASBasedShadow()

	if shadow == nil {
		t.Fatal("NewCASBasedShadow() returned nil")
	}

	t.Logf("NewCASBasedShadow() created valid instance")
}

// TestCASBasedShadow_Load_Empty verifies Load returns nil for empty shadow.
func TestCASBasedShadow_Load_Empty(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x1234)

	vs := shadow.Load(addr)

	if vs != nil {
		t.Errorf("Load(0x%x) returned non-nil for empty shadow: %v", addr, vs)
	}

	t.Logf("Load(0x%x) correctly returned nil for empty shadow", addr)
}

// TestCASBasedShadow_Store_New verifies Store creates new cell.
func TestCASBasedShadow_Store_New(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x5678)

	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))

	storedVS := shadow.Store(addr, vs)

	if storedVS != vs {
		t.Errorf("Store() returned different VarState: %p vs %p", storedVS, vs)
	}

	// Verify Load returns the stored cell.
	loadedVS := shadow.Load(addr)
	if loadedVS != vs {
		t.Errorf("Load() returned different VarState after Store(): %p vs %p", loadedVS, vs)
	}

	if loadedVS.GetW() != epoch.NewEpoch(5, 100) {
		t.Errorf("VarState.W = %v, want %v", loadedVS.GetW(), epoch.NewEpoch(5, 100))
	}

	t.Logf("Store(0x%x) and Load() work correctly", addr)
}

// TestCASBasedShadow_LoadOrStore_Miss verifies LoadOrStore creates new cell.
func TestCASBasedShadow_LoadOrStore_Miss(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0xABCD)

	vs, created := shadow.LoadOrStore(addr)

	if vs == nil {
		t.Fatal("LoadOrStore() returned nil")
	}

	if !created {
		t.Error("LoadOrStore() should return created=true for new address")
	}

	// New VarState should be zero-initialized.
	if vs.GetW() != 0 {
		t.Errorf("New VarState.W = %v, want 0", vs.GetW())
	}

	t.Logf("LoadOrStore(0x%x) created new cell: %s", addr, vs)
}

// TestCASBasedShadow_LoadOrStore_Hit verifies LoadOrStore returns existing cell.
func TestCASBasedShadow_LoadOrStore_Hit(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0xDEAD)

	// First call creates the cell.
	vs1, created1 := shadow.LoadOrStore(addr)
	vs1.SetW(epoch.NewEpoch(7, 200))

	if !created1 {
		t.Error("First LoadOrStore() should return created=true")
	}

	// Second call should return the same cell.
	vs2, created2 := shadow.LoadOrStore(addr)

	if created2 {
		t.Error("Second LoadOrStore() should return created=false")
	}

	if vs2 != vs1 {
		t.Errorf("LoadOrStore() returned different instance: %p vs %p", vs2, vs1)
	}

	if vs2.GetW() != epoch.NewEpoch(7, 200) {
		t.Errorf("VarState.W = %v, want %v", vs2.GetW(), epoch.NewEpoch(7, 200))
	}

	t.Logf("LoadOrStore(0x%x) returned existing cell: %s", addr, vs2)
}

// TestCASBasedShadow_MultipleAddresses verifies independent cells.
func TestCASBasedShadow_MultipleAddresses(t *testing.T) {
	shadow := NewCASBasedShadow()

	addresses := []uintptr{0x1000, 0x2000, 0x3000, 0x4000, 0x5000}
	cells := make([]*VarState, len(addresses))

	// Create cells for multiple addresses.
	for i, addr := range addresses {
		vs, created := shadow.LoadOrStore(addr)
		if !created {
			t.Errorf("LoadOrStore(0x%x) should create new cell", addr)
		}
		vs.SetW(epoch.NewEpoch(uint16(i+1), uint64((i+1)*100)))
		cells[i] = vs
	}

	// Verify each cell is independent.
	for i, addr := range addresses {
		vs := shadow.Load(addr)

		if vs == nil {
			t.Fatalf("Load(0x%x) returned nil", addr)
		}

		if vs != cells[i] {
			t.Errorf("Load(0x%x) returned different instance", addr)
		}

		expectedW := epoch.NewEpoch(uint16(i+1), uint64((i+1)*100))
		if vs.GetW() != expectedW {
			t.Errorf("VarState[0x%x].W = %v, want %v", addr, vs.GetW(), expectedW)
		}
	}

	t.Logf("LoadOrStore() correctly handles %d independent addresses", len(addresses))
}

// TestCASBasedShadow_Reset verifies Reset clears all cells.
func TestCASBasedShadow_Reset(t *testing.T) {
	shadow := NewCASBasedShadow()

	// Create multiple cells.
	addresses := []uintptr{0x1111, 0x2222, 0x3333, 0x4444}
	for _, addr := range addresses {
		vs, _ := shadow.LoadOrStore(addr)
		vs.SetW(epoch.NewEpoch(1, 10))
	}

	// Verify cells exist before reset.
	for _, addr := range addresses {
		if shadow.Load(addr) == nil {
			t.Fatalf("Cell at 0x%x should exist before Reset()", addr)
		}
	}

	// Reset should clear everything.
	shadow.Reset()

	// Verify all cells are gone.
	for _, addr := range addresses {
		vs := shadow.Load(addr)
		if vs != nil {
			t.Errorf("Load(0x%x) returned non-nil after Reset(): %v", addr, vs)
		}
	}

	t.Logf("Reset() correctly cleared %d cells", len(addresses))
}

// ========================================
// Hash Function Tests
// ========================================

// TestFastHash_Distribution verifies hash function has good distribution.
func TestFastHash_Distribution(t *testing.T) {
	const numAddresses = 10000
	hashes := make(map[uint64]int)

	// Generate sequential addresses and hash them.
	for i := 0; i < numAddresses; i++ {
		addr := uintptr(0x10000 + i*8) // Simulate sequential memory addresses.
		hash := fastHash(addr)

		if hash > 0xFFFF {
			t.Errorf("fastHash(%#x) = %#x, exceeds 16-bit range", addr, hash)
		}

		hashes[hash]++
	}

	// Calculate statistics.
	uniqueHashes := len(hashes)
	collisions := numAddresses - uniqueHashes
	collisionRate := float64(collisions) / float64(numAddresses) * 100

	t.Logf("Hash distribution: %d addresses → %d unique hashes (%.2f%% collision rate)",
		numAddresses, uniqueHashes, collisionRate)

	// For 10K addresses in 64K slots, birthday paradox predicts ~7% collision rate.
	// 10000^2 / (2 * 65536) ≈ 763 collisions expected.
	// Accept up to 20% collision rate (conservative threshold).
	if collisionRate > 20.0 {
		t.Errorf("Collision rate %.2f%% exceeds 20%% threshold (poor hash function)", collisionRate)
	}

	// Check for reasonable distribution (no bucket has >5% of addresses).
	maxBucketSize := 0
	for _, count := range hashes {
		maxBucketSize = max(maxBucketSize, count)
	}

	maxBucketPercent := float64(maxBucketSize) / float64(numAddresses) * 100
	if maxBucketPercent > 5.0 {
		t.Errorf("Max bucket has %.2f%% of addresses (poor distribution)", maxBucketPercent)
	}

	t.Logf("Hash distribution OK: max bucket = %d addresses (%.2f%%)", maxBucketSize, maxBucketPercent)
}

// TestFastHash_Deterministic verifies hash function is deterministic.
func TestFastHash_Deterministic(t *testing.T) {
	addresses := []uintptr{0x1000, 0x2000, 0xDEADBEEF, 0xCAFEBABE}

	for _, addr := range addresses {
		hash1 := fastHash(addr)
		hash2 := fastHash(addr)

		if hash1 != hash2 {
			t.Errorf("fastHash(%#x) non-deterministic: %#x vs %#x", addr, hash1, hash2)
		}
	}

	t.Logf("fastHash() is deterministic")
}

// ========================================
// Collision Handling Tests
// ========================================

// TestCASBasedShadow_Collisions verifies linear probing handles collisions.
func TestCASBasedShadow_Collisions(t *testing.T) {
	shadow := NewCASBasedShadow()

	// Find addresses that hash to the same index.
	targetHash := uint64(0x1234)
	var collidingAddresses []uintptr

	for addr := uintptr(0); len(collidingAddresses) < 10; addr += 8 {
		if fastHash(addr) == targetHash {
			collidingAddresses = append(collidingAddresses, addr)
		}

		// Safety: stop after checking 1M addresses.
		if addr > 1000000 {
			t.Skip("Could not find enough colliding addresses for test")
			return
		}
	}

	t.Logf("Found %d addresses that hash to 0x%04x", len(collidingAddresses), targetHash)

	// Store all colliding addresses (should use linear probing).
	for i, addr := range collidingAddresses {
		vs, created := shadow.LoadOrStore(addr)
		if !created {
			t.Errorf("LoadOrStore(0x%x) should create new cell", addr)
		}
		vs.SetW(epoch.NewEpoch(1, uint64(i)))
	}

	// Verify all addresses can be retrieved correctly.
	for i, addr := range collidingAddresses {
		vs := shadow.Load(addr)
		if vs == nil {
			t.Errorf("Load(0x%x) returned nil after collision handling", addr)
			continue
		}

		expectedW := epoch.NewEpoch(1, uint64(i))
		if vs.GetW() != expectedW {
			t.Errorf("VarState[0x%x].W = %v, want %v (collision handling failed)", addr, vs.GetW(), expectedW)
		}
	}

	t.Logf("Linear probing correctly handled %d collisions", len(collidingAddresses))
}

// ========================================
// Concurrency Tests
// ========================================

// TestCASBasedShadow_Concurrent_LoadOrStore verifies concurrent LoadOrStore.
func TestCASBasedShadow_Concurrent_LoadOrStore(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0xCAFE)

	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make([]*VarState, numGoroutines)
	createdCount := make([]bool, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			vs, created := shadow.LoadOrStore(addr)
			results[idx] = vs
			createdCount[idx] = created
		}(i)
	}

	wg.Wait()

	// All goroutines should get the SAME VarState instance.
	first := results[0]
	if first == nil {
		t.Fatal("LoadOrStore() returned nil")
	}

	for i, vs := range results {
		if vs != first {
			t.Errorf("Goroutine %d got different VarState: %p vs %p", i, vs, first)
		}
	}

	// Exactly ONE goroutine should have created=true.
	createdTrue := 0
	for _, created := range createdCount {
		if created {
			createdTrue++
		}
	}

	if createdTrue != 1 {
		t.Errorf("Expected 1 goroutine with created=true, got %d", createdTrue)
	}

	t.Logf("Concurrent LoadOrStore() from %d goroutines: all got same instance, 1 creator", numGoroutines)
}

// TestCASBasedShadow_Concurrent_MultipleAddresses tests concurrent access to many addresses.
func TestCASBasedShadow_Concurrent_MultipleAddresses(t *testing.T) {
	shadow := NewCASBasedShadow()

	const numGoroutines = 50
	const numAddresses = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(gid int) {
			defer wg.Done()

			// Each goroutine accesses multiple addresses.
			for j := 0; j < numAddresses; j++ {
				addr := uintptr(0x10000 + j*0x100)
				vs, _ := shadow.LoadOrStore(addr)

				// Update the cell.
				vs.SetW(epoch.NewEpoch(uint16(gid), uint64(j)))
			}
		}(i)
	}

	wg.Wait()

	// Verify all addresses have cells.
	for j := 0; j < numAddresses; j++ {
		addr := uintptr(0x10000 + j*0x100)
		vs := shadow.Load(addr)

		if vs == nil {
			t.Errorf("Address 0x%x missing after concurrent access", addr)
		}
	}

	t.Logf("Concurrent access from %d goroutines to %d addresses succeeded", numGoroutines, numAddresses)
}

// ========================================
// Performance Tests
// ========================================

// TestCASBasedShadow_NoAllocOnHit verifies zero allocations for existing cells.
func TestCASBasedShadow_NoAllocOnHit(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x7777)

	// Create the cell first.
	shadow.LoadOrStore(addr)

	// Measure allocations for subsequent LoadOrStore (should be 0).
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = shadow.LoadOrStore(addr)
	})

	if allocs > 0 {
		t.Errorf("LoadOrStore(existing) allocated %.2f times, want 0", allocs)
	}

	t.Logf("LoadOrStore(existing) allocations: %.2f (zero allocation confirmed)", allocs)
}

// TestCASBasedShadow_Load_NoAlloc verifies Load never allocates.
func TestCASBasedShadow_Load_NoAlloc(t *testing.T) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x8888)

	// Create the cell.
	shadow.LoadOrStore(addr)

	// Measure allocations for Load (hit case).
	allocsHit := testing.AllocsPerRun(1000, func() {
		_ = shadow.Load(addr)
	})

	if allocsHit > 0 {
		t.Errorf("Load(existing) allocated %.2f times, want 0", allocsHit)
	}

	t.Logf("Load(existing) allocations: %.2f (zero allocation confirmed)", allocsHit)

	// Measure allocations for Load (miss case).
	allocsMiss := testing.AllocsPerRun(1000, func() {
		_ = shadow.Load(0x9999) // Different address that doesn't exist.
	})

	if allocsMiss > 0 {
		t.Errorf("Load(missing) allocated %.2f times, want 0", allocsMiss)
	}

	t.Logf("Load(missing) allocations: %.2f (zero allocation confirmed)", allocsMiss)
}

// ========================================
// Benchmarks
// ========================================

// BenchmarkCASBasedShadow_LoadOrStore_Hit benchmarks LoadOrStore for existing cell.
// Target: <10ns/op, 0 allocs (43% faster than sync.Map).
func BenchmarkCASBasedShadow_LoadOrStore_Hit(b *testing.B) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x1000)

	// Pre-create the cell.
	shadow.LoadOrStore(addr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = shadow.LoadOrStore(addr)
	}
}

// BenchmarkCASBasedShadow_LoadOrStore_Miss benchmarks LoadOrStore for new cell.
// Target: <30ns/op, 2 allocs (VarState + CASCell).
func BenchmarkCASBasedShadow_LoadOrStore_Miss(b *testing.B) {
	shadow := NewCASBasedShadow()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		addr := uintptr(0x10000 + i*8)
		_, _ = shadow.LoadOrStore(addr)
	}
}

// BenchmarkCASBasedShadow_Load_Hit benchmarks Load for existing cell.
// Target: <8ns/op, 0 allocs.
func BenchmarkCASBasedShadow_Load_Hit(b *testing.B) {
	shadow := NewCASBasedShadow()
	addr := uintptr(0x2000)

	// Pre-create the cell.
	shadow.LoadOrStore(addr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = shadow.Load(addr)
	}
}

// BenchmarkCASBasedShadow_Concurrent benchmarks concurrent LoadOrStore access.
// This simulates real race detector workload with multiple goroutines.
func BenchmarkCASBasedShadow_Concurrent(b *testing.B) {
	shadow := NewCASBasedShadow()

	// Pre-populate hot addresses.
	hotAddresses := make([]uintptr, 100)
	for i := range hotAddresses {
		hotAddresses[i] = uintptr(0x30000 + i*8)
		shadow.LoadOrStore(hotAddresses[i])
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			addr := hotAddresses[i%len(hotAddresses)]
			vs, _ := shadow.LoadOrStore(addr)
			vs.SetW(epoch.NewEpoch(1, uint64(i)))
			i++
		}
	})
}

// BenchmarkCASBasedShadow_CollisionRate measures collision impact on performance.
func BenchmarkCASBasedShadow_CollisionRate(b *testing.B) {
	shadow := NewCASBasedShadow()

	// Populate shadow with addresses that will cause collisions.
	// Use modulo arithmetic to create controlled collisions.
	const numVars = 50000 // 50K variables in 64K slots ≈ 77% load factor.

	for i := 0; i < numVars; i++ {
		addr := uintptr(i * 8)
		shadow.LoadOrStore(addr)
	}

	// Report collision statistics.
	total, occupied, collisions := shadow.GetCollisionStats()
	loadFactor := float64(occupied) / float64(total) * 100
	collisionRate := float64(collisions) / float64(occupied) * 100

	b.Logf("Load factor: %.1f%%, Collision rate: %.2f%%", loadFactor, collisionRate)

	b.ReportAllocs()
	b.ResetTimer()

	// Benchmark Load performance under collision stress.
	for i := 0; i < b.N; i++ {
		addr := uintptr((i % numVars) * 8)
		_ = shadow.Load(addr)
	}
}

// ========================================
// Statistics Tests
// ========================================

// TestCASBasedShadow_GetCollisionStats verifies collision statistics tracking.
func TestCASBasedShadow_GetCollisionStats(t *testing.T) {
	shadow := NewCASBasedShadow()

	// Initially empty.
	total, occupied, collisions := shadow.GetCollisionStats()
	if total != 65536 {
		t.Errorf("GetCollisionStats() total = %d, want 65536", total)
	}
	if occupied != 0 {
		t.Errorf("GetCollisionStats() occupied = %d, want 0 (empty)", occupied)
	}
	if collisions != 0 {
		t.Errorf("GetCollisionStats() collisions = %d, want 0 (empty)", collisions)
	}

	// Add some addresses.
	const numVars = 1000
	for i := 0; i < numVars; i++ {
		addr := uintptr(i * 8)
		shadow.LoadOrStore(addr)
	}

	total, occupied, collisions = shadow.GetCollisionStats()
	if occupied != numVars {
		t.Errorf("GetCollisionStats() occupied = %d, want %d", occupied, numVars)
	}

	loadFactor := float64(occupied) / float64(total) * 100
	collisionRate := float64(collisions) / float64(occupied) * 100

	t.Logf("Collision stats: %d variables, load factor %.1f%%, collision rate %.2f%%",
		numVars, loadFactor, collisionRate)

	// For 1K variables in 64K slots, expect very low collision rate (<0.1%).
	if collisionRate > 1.0 {
		t.Errorf("Collision rate %.2f%% exceeds 1%% (unexpected)", collisionRate)
	}
}

// ========================================
// v0.3.0 Address Compression Tests (P1)
// ========================================

// TestCASBasedShadow_AddressCompression verifies 8-byte address alignment.
func TestCASBasedShadow_AddressCompression(t *testing.T) {
	shadow := NewCASBasedShadow()

	// Address compression is enabled by default.
	if !shadow.GetAddressCompression() {
		t.Error("Address compression should be enabled by default")
	}

	// Test that addresses within same 8-byte block share the same VarState.
	baseAddr := uintptr(0x1000) // Aligned to 8 bytes.
	vs1, _ := shadow.LoadOrStore(baseAddr)
	vs1.SetW(epoch.NewEpoch(1, 100))

	// Addresses +1 to +7 should return the same VarState (same 8-byte block).
	for offset := uintptr(1); offset < 8; offset++ {
		addr := baseAddr + offset
		vs, created := shadow.LoadOrStore(addr)
		if created {
			t.Errorf("LoadOrStore(0x%x) created new cell, expected existing", addr)
		}
		if vs != vs1 {
			t.Errorf("LoadOrStore(0x%x) returned different VarState: %p vs %p", addr, vs, vs1)
		}
	}

	// Address +8 should create a NEW VarState (different 8-byte block).
	vs2, created := shadow.LoadOrStore(baseAddr + 8)
	if !created {
		t.Error("LoadOrStore(baseAddr+8) should create new cell")
	}
	if vs2 == vs1 {
		t.Error("baseAddr+8 should have different VarState than baseAddr")
	}

	t.Logf("v0.3.0 Address compression: addresses within 8-byte block share VarState")
}

// TestCASBasedShadow_AddressCompressionDisabled verifies disabling works.
func TestCASBasedShadow_AddressCompressionDisabled(t *testing.T) {
	shadow := NewCASBasedShadow()
	shadow.SetAddressCompression(false) // Disable compression.

	if shadow.GetAddressCompression() {
		t.Error("Address compression should be disabled after SetAddressCompression(false)")
	}

	// With compression disabled, each address gets its own VarState.
	baseAddr := uintptr(0x2000)
	vs1, _ := shadow.LoadOrStore(baseAddr)
	vs1.SetW(epoch.NewEpoch(1, 100))

	// Even +1 should get a different VarState.
	vs2, created := shadow.LoadOrStore(baseAddr + 1)
	if !created {
		t.Error("With compression disabled, baseAddr+1 should create new cell")
	}
	if vs2 == vs1 {
		t.Error("With compression disabled, baseAddr+1 should have different VarState")
	}

	t.Logf("v0.3.0 Address compression disabled: each address gets unique VarState")
}

// TestAlignAddr verifies the address alignment function.
func TestAlignAddr(t *testing.T) {
	tests := []struct {
		input    uintptr
		expected uintptr
	}{
		{0x1000, 0x1000}, // Already aligned.
		{0x1001, 0x1000}, // Align down.
		{0x1007, 0x1000}, // Align down.
		{0x1008, 0x1008}, // Already aligned.
		{0x1009, 0x1008}, // Align down.
		{0x100F, 0x1008}, // Align down.
		{0x0000, 0x0000}, // Zero stays zero.
		{0x0007, 0x0000}, // Small addresses.
	}

	for _, tt := range tests {
		result := alignAddr(tt.input)
		if result != tt.expected {
			t.Errorf("alignAddr(0x%x) = 0x%x, want 0x%x", tt.input, result, tt.expected)
		}
	}

	t.Log("v0.3.0 alignAddr() correctly aligns to 8-byte boundaries")
}
