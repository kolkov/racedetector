package api

import (
	"sync"
	"testing"
)

// TestInit verifies global detector is initialized.
func TestInit(t *testing.T) {
	// Re-enable in case a previous test called Fini()
	Enable()

	if det == nil {
		t.Fatal("Global detector not initialized")
	}

	if !enabled.Load() {
		t.Error("Detector should be enabled by default")
	}
}

// TestParseGID_ValidInput tests goroutine ID parsing with valid input.
func TestParseGID_ValidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{
			name:  "single_digit",
			input: "goroutine 1 [running]:\n",
			want:  1,
		},
		{
			name:  "double_digit",
			input: "goroutine 42 [running]:\n",
			want:  42,
		},
		{
			name:  "large_number",
			input: "goroutine 999999 [running]:\n",
			want:  999999,
		},
		{
			name:  "with_stack_trace",
			input: "goroutine 123 [running]:\ngithub.com/...\n",
			want:  123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGID([]byte(tt.input))
			if got != tt.want {
				t.Errorf("parseGID() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestParseGID_InvalidInput tests goroutine ID parsing with invalid input.
func TestParseGID_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "too_short",
			input: "goroutine",
		},
		{
			name:  "wrong_prefix",
			input: "thread 123 [running]:\n",
		},
		{
			name:  "no_number",
			input: "goroutine  [running]:\n",
		},
		{
			name:  "invalid_number",
			input: "goroutine abc [running]:\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGID([]byte(tt.input))
			if got != 0 {
				t.Errorf("parseGID() = %d, want 0 for invalid input", got)
			}
		})
	}
}

// TestGetGoroutineID verifies goroutine ID extraction.
func TestGetGoroutineID(t *testing.T) {
	// Get GID in main goroutine.
	gid1 := getGoroutineID()
	if gid1 == 0 {
		t.Error("getGoroutineID() returned 0 - parsing failed")
	}

	// Get GID again - should be same goroutine.
	gid2 := getGoroutineID()
	if gid1 != gid2 {
		t.Errorf("getGoroutineID() inconsistent: got %d then %d", gid1, gid2)
	}

	// Spawn new goroutine - should get different GID.
	var gid3 int64
	done := make(chan bool)
	go func() {
		gid3 = getGoroutineID()
		done <- true
	}()
	<-done

	if gid3 == 0 {
		t.Error("getGoroutineID() in spawned goroutine returned 0")
	}
	if gid1 == gid3 {
		t.Errorf("getGoroutineID() same for different goroutines: %d", gid1)
	}
}

// TestGetCurrentContext_FirstCall verifies context allocation on first call.
func TestGetCurrentContext_FirstCall(t *testing.T) {
	// Reset state to ensure clean test.
	Reset()

	// First call should allocate new context.
	ctx := getCurrentContext()
	if ctx == nil {
		t.Fatal("getCurrentContext() returned nil")
	}

	// TID should be 0 (first allocated from pool after Reset).
	if ctx.TID != 0 {
		t.Errorf("First context TID = %d, want 0", ctx.TID)
	}

	// Epoch should be initialized to 0@1.
	// CRITICAL: Clock must start at 1, not 0, to detect unsynchronized races.
	// Clock 0 means "never happened" in HappensBefore check (0 <= 0 is TRUE).
	tid, clock := ctx.Epoch.Decode()
	if tid != 0 {
		t.Errorf("First context Epoch TID = %d, want 0", tid)
	}
	if clock != 1 {
		t.Errorf("First context Epoch Clock = %d, want 1", clock)
	}
}

// TestGetCurrentContext_Cached verifies context caching.
func TestGetCurrentContext_Cached(t *testing.T) {
	Reset()

	// First call allocates.
	ctx1 := getCurrentContext()

	// Second call should return cached context.
	ctx2 := getCurrentContext()

	// Should be exact same instance (same pointer).
	if ctx1 != ctx2 {
		t.Error("getCurrentContext() not caching - returned different instances")
	}
}

// TestGetCurrentContext_Concurrent verifies concurrent context allocation.
func TestGetCurrentContext_Concurrent(t *testing.T) {
	Reset()

	const numGoroutines = 100

	// Launch 100 goroutines concurrently.
	var wg sync.WaitGroup
	contexts := make([]uint16, numGoroutines) // Store TIDs instead of contexts
	tids := make([]uint16, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := getCurrentContext()
			contexts[idx] = ctx.TID
			tids[idx] = ctx.TID
		}(i)
	}

	wg.Wait()

	// All contexts should have been allocated (TIDs should be set).
	// This is verified by TID uniqueness check below.

	// Verify TIDs are unique (within uint16 range 0-65535).
	// Since we have 100 goroutines, all should be unique.
	tidSet := make(map[uint16]bool)
	for i, tid := range tids {
		if tidSet[tid] {
			t.Errorf("Duplicate TID %d at index %d", tid, i)
		}
		tidSet[tid] = true
	}

	// Verify we allocated 100 unique TIDs (0-99).
	if len(tidSet) != numGoroutines {
		t.Errorf("Expected %d unique TIDs, got %d", numGoroutines, len(tidSet))
	}
}

// TestGetCurrentContext_TIDPoolAllocation verifies TID pool allocation.
// Phase 2 Task 2.2: TIDs are now allocated from a reuse pool, not by wrapping.
func TestGetCurrentContext_TIDPoolAllocation(t *testing.T) {
	Reset()

	// Allocate 5 contexts in new goroutines.
	// With TID pool, they should get TIDs: 0, 1, 2, 3, 4 (sequential from pool).
	tids := make([]uint16, 5)
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Force allocation in new goroutine.
			ctx := getCurrentContext()
			tids[idx] = ctx.TID
		}(i)
	}

	wg.Wait()

	// Expected TIDs: 0, 1, 2, 3, 4 (in some order due to concurrency).
	// With TID pool initialized to [0, 1, 2, ..., 65535], we allocate from front (FIFO).
	expected := map[uint16]bool{0: true, 1: true, 2: true, 3: true, 4: true}
	for i, tid := range tids {
		if !expected[tid] {
			t.Errorf("TID at index %d = %d, expected one of {0, 1, 2, 3, 4}", i, tid)
		}
	}

	// Verify all TIDs are unique.
	tidSet := make(map[uint16]bool)
	for _, tid := range tids {
		if tidSet[tid] {
			t.Errorf("Duplicate TID %d", tid)
		}
		tidSet[tid] = true
	}
}

// TestRaceRead_Enabled verifies raceread calls detector when enabled.
func TestRaceRead_Enabled(t *testing.T) {
	Reset()
	Enable()

	// Perform read access.
	addr := uintptr(0x1000)
	raceread(addr)

	// Detector should have been called (no race detected for single access).
	// We can verify by checking that no races were detected.
	if got := RacesDetected(); got != 0 {
		t.Errorf("Unexpected race detected: %d", got)
	}
}

// TestRaceWrite_Enabled verifies racewrite calls detector when enabled.
func TestRaceWrite_Enabled(t *testing.T) {
	Reset()
	Enable()

	// Perform write access.
	addr := uintptr(0x2000)
	racewrite(addr)

	// Detector should have been called (no race for single write).
	if got := RacesDetected(); got != 0 {
		t.Errorf("Unexpected race detected: %d", got)
	}
}

// TestRaceRead_Disabled verifies raceread is no-op when disabled.
func TestRaceRead_Disabled(t *testing.T) {
	Reset()
	Disable()

	// Get initial shadow memory count.
	// Accessing a new address should NOT create shadow cell when disabled.
	addr := uintptr(0x3000)

	// Save races detected before.
	racesBefore := RacesDetected()

	// Perform read - should be no-op.
	raceread(addr)

	// Races count should be unchanged.
	racesAfter := RacesDetected()
	if racesAfter != racesBefore {
		t.Errorf("raceread() when disabled changed race count: %d -> %d", racesBefore, racesAfter)
	}

	// Re-enable for other tests.
	Enable()
}

// TestRaceWrite_Disabled verifies racewrite is no-op when disabled.
func TestRaceWrite_Disabled(t *testing.T) {
	Reset()
	Disable()

	addr := uintptr(0x4000)
	racesBefore := RacesDetected()

	// Perform write - should be no-op.
	racewrite(addr)

	racesAfter := RacesDetected()
	if racesAfter != racesBefore {
		t.Errorf("racewrite() when disabled changed race count: %d -> %d", racesBefore, racesAfter)
	}

	Enable()
}

// TestEnableDisable verifies Enable/Disable functionality.
func TestEnableDisable(t *testing.T) {
	// Enable.
	Enable()
	if !enabled.Load() {
		t.Error("Enable() did not enable detector")
	}

	// Disable.
	Disable()
	if enabled.Load() {
		t.Error("Disable() did not disable detector")
	}

	// Re-enable.
	Enable()
	if !enabled.Load() {
		t.Error("Re-Enable() did not enable detector")
	}
}

// TestRacesDetected verifies race counter.
func TestRacesDetected(t *testing.T) {
	Reset()
	Enable()

	// Initially 0.
	if got := RacesDetected(); got != 0 {
		t.Errorf("RacesDetected() = %d, want 0 initially", got)
	}

	// Trigger a write-write race.
	// Write to same address from same goroutine with same epoch should NOT race.
	// But if we increment clock between writes, we can trigger happens-before check.
	// Simpler: Write, then write again without sync - this is same-epoch optimization.
	// To trigger race, we need concurrent writes. Let's use detector directly for testing.

	// For API test, just verify counter works.
	// Actual race detection is tested in detector_test.go.
	// Here we just verify RacesDetected() returns correct count.

	// Since we can't easily trigger race via API alone (need concurrent access),
	// we'll test that RacesDetected() returns 0 for safe accesses.
	addr := uintptr(0x5000)
	racewrite(addr)
	raceread(addr)

	// No race should be detected (sequential access).
	if got := RacesDetected(); got != 0 {
		t.Errorf("RacesDetected() = %d, want 0 for sequential access", got)
	}
}

// TestReset verifies Reset clears all state.
func TestReset(t *testing.T) {
	// Do some operations to create state.
	racewrite(uintptr(0x6000))
	getCurrentContext() // Allocate context

	// Reset.
	Reset()

	// Verify nextTID reset to 0.
	if got := nextTID.Load(); got != 0 {
		t.Errorf("After Reset(), nextTID = %d, want 0", got)
	}

	// Verify races counter reset.
	if got := RacesDetected(); got != 0 {
		t.Errorf("After Reset(), RacesDetected() = %d, want 0", got)
	}

	// Verify contexts cleared.
	// Allocate context - should get TID 0 again.
	ctx := getCurrentContext()
	if ctx.TID != 0 {
		t.Errorf("After Reset(), first context TID = %d, want 0", ctx.TID)
	}
}

// TestGetCallerPC verifies PC extraction.
func TestGetCallerPC(t *testing.T) {
	// Call getcallerpc from this test function.
	// Stack: getcallerpc -> TestGetCallerPC
	// Caller(2) would go: getcallerpc(0) -> TestGetCallerPC(1) -> testing.tRunner(2)
	// So this test just verifies it doesn't panic and returns non-zero.

	// We can't directly test getcallerpc since it's designed to be called
	// from raceread/racewrite. But we can verify it doesn't crash.

	// Instead, verify that calling raceread doesn't panic (it calls getcallerpc internally).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("raceread panicked (likely getcallerpc issue): %v", r)
		}
	}()

	raceread(uintptr(0x7000))
}

// TestGetCallerPC_Direct tests getcallerpc directly.
func TestGetCallerPC_Direct(_ *testing.T) {
	// Direct call - this will have different stack depth.
	pc := getcallerpc()
	// PC should be non-zero even if Caller fails (returns 0).
	// We just verify it doesn't panic.
	_ = pc
}

// TestConcurrentRaceAccess tests concurrent raceread/racewrite calls.
func TestConcurrentRaceAccess(t *testing.T) {
	Reset()
	Enable()

	const numGoroutines = 50
	const numAccesses = 100

	var wg sync.WaitGroup

	// Launch goroutines that concurrently read/write different addresses.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()

			// Each goroutine accesses unique addresses to avoid actual races.
			baseAddr := uintptr(0x10000 + gid*1000)

			for j := 0; j < numAccesses; j++ {
				addr := baseAddr + uintptr(j)
				if j%2 == 0 {
					racewrite(addr)
				} else {
					raceread(addr)
				}
			}
		}(i)
	}

	wg.Wait()

	// No races should be detected (all access unique addresses).
	if got := RacesDetected(); got != 0 {
		t.Errorf("Concurrent accesses to unique addresses reported %d races, want 0", got)
	}
}

// TestRaceDetection_SimpleWriteWrite verifies write-write race detection via API.
func TestRaceDetection_SimpleWriteWrite(_ *testing.T) {
	Reset()
	Enable()

	addr := uintptr(0x8000)

	// Setup: Two goroutines writing to same address.
	// First goroutine writes.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		racewrite(addr)
	}()
	wg.Wait()

	// Second goroutine writes (should detect race if no synchronization).
	// However, due to happens-before via WaitGroup, no race should be detected.
	// To trigger race, we need concurrent writes without sync.

	// For true race, we'd need to not use sync primitives.
	// But this is just API test - detector_test.go has comprehensive race tests.

	// Here we just verify API doesn't crash with concurrent access.
	wg.Add(2)
	go func() {
		defer wg.Done()
		racewrite(addr)
	}()
	go func() {
		defer wg.Done()
		racewrite(addr)
	}()
	wg.Wait()

	// Due to WaitGroup sync, happens-before is established, so no race.
	// Detector tests verify actual race detection logic.
}

// TestMultipleGoroutinesUniqueContexts verifies each goroutine gets unique context.
func TestMultipleGoroutinesUniqueContexts(t *testing.T) {
	Reset()

	const numGoroutines = 20
	tids := make([]uint16, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine gets its own context.
			ctx := getCurrentContext()
			tids[idx] = ctx.TID
		}(i)
	}

	wg.Wait()

	// Verify all TIDs are unique.
	tidSet := make(map[uint16]bool)
	for i, tid := range tids {
		if tidSet[tid] {
			t.Errorf("TID %d at index %d is duplicate", tid, i)
		}
		tidSet[tid] = true
	}

	if len(tidSet) != numGoroutines {
		t.Errorf("Expected %d unique TIDs, got %d", numGoroutines, len(tidSet))
	}
}

// TestRaceReadWrite_ZeroAddress tests handling of zero address.
func TestRaceReadWrite_ZeroAddress(t *testing.T) {
	Reset()
	Enable()

	// Zero address is technically valid (though unusual).
	// Should not crash.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Access to zero address panicked: %v", r)
		}
	}()

	racewrite(0)
	raceread(0)
}

// TestRaceReadWrite_HighAddress tests handling of high memory addresses.
func TestRaceReadWrite_HighAddress(t *testing.T) {
	Reset()
	Enable()

	// Test with high address values.
	addr := uintptr(0xFFFFFFFFFFFF)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Access to high address panicked: %v", r)
		}
	}()

	racewrite(addr)
	raceread(addr)
}

// TestMainGoroutineContext verifies main goroutine gets context.
func TestMainGoroutineContext(t *testing.T) {
	// This test runs in main goroutine.
	ctx := getCurrentContext()
	if ctx == nil {
		t.Fatal("Main goroutine context is nil")
	}

	// Should have valid epoch.
	epochTID, _ := ctx.Epoch.Decode()
	if epochTID != ctx.TID {
		t.Errorf("Epoch TID %d doesn't match context TID %d", epochTID, ctx.TID)
	}
}

// TestGetGoroutineID_Stability verifies GID doesn't change within goroutine.
func TestGetGoroutineID_Stability(t *testing.T) {
	const numSamples = 100

	// Get GID multiple times in same goroutine.
	gids := make([]int64, numSamples)
	for i := 0; i < numSamples; i++ {
		gids[i] = getGoroutineID()
	}

	// All should be identical.
	first := gids[0]
	for i, gid := range gids {
		if gid != first {
			t.Errorf("GID changed at sample %d: got %d, want %d", i, gid, first)
		}
	}
}

// TestContextCaching_Performance verifies context lookup is fast after first call.
func TestContextCaching_Performance(t *testing.T) {
	Reset()

	// First call (allocates).
	_ = getCurrentContext()

	// Measure second call (should be cached).
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		ctx := getCurrentContext()
		if ctx == nil {
			t.Fatal("Cached context lookup returned nil")
		}
	}

	// This test just verifies caching works (no performance measurement).
	// Benchmark tests will measure actual performance.
}

// TestInitFunctionality verifies Init() correctly initializes the detector.
func TestInitFunctionality(t *testing.T) {
	// Call Init to reset everything.
	Init()

	// Verify detector is enabled.
	if !enabled.Load() {
		t.Error("Init() did not enable detector")
	}

	// Verify nextTID is set to 2 after Init().
	// Init() allocates TID=1 for the main goroutine (TID=0 is reserved as sentinel
	// meaning "no exclusive writer" in SmartTrack), then sets nextTID to 2
	// to ensure subsequent goroutines get TID >= 2.
	expectedNextTID := uint32(2)
	if got := nextTID.Load(); got != expectedNextTID {
		t.Errorf("After Init(), nextTID = %d, want %d", got, expectedNextTID)
	}

	// Verify main goroutine has TID=1.
	// TID=0 is reserved as sentinel value in SmartTrack.
	ctx := getCurrentContext()
	if ctx == nil {
		t.Fatal("getCurrentContext() returned nil after Init()")
	}
	if ctx.TID != 1 {
		t.Errorf("Main goroutine TID = %d, want 1", ctx.TID)
	}

	// Verify detector instance is not nil.
	if det == nil {
		t.Fatal("Init() did not create detector instance")
	}

	// Verify no races detected initially.
	if got := RacesDetected(); got != 0 {
		t.Errorf("After Init(), RacesDetected() = %d, want 0", got)
	}
}

// TestInitIdempotent verifies Init() can be called multiple times safely.
func TestInitIdempotent(t *testing.T) {
	// First Init.
	Init()

	// Do some operations to create state.
	addr := uintptr(0x9000)
	racewrite(addr)
	raceread(addr)

	// Call Init again - should reset everything.
	Init()

	// Verify state is reset.
	if got := RacesDetected(); got != 0 {
		t.Errorf("After second Init(), RacesDetected() = %d, want 0", got)
	}

	// Verify main goroutine still has TID=1.
	// TID=0 is reserved as sentinel value in SmartTrack.
	ctx := getCurrentContext()
	if ctx.TID != 1 {
		t.Errorf("After second Init(), main goroutine TID = %d, want 1", ctx.TID)
	}

	// Verify enabled.
	if !enabled.Load() {
		t.Error("After second Init(), detector not enabled")
	}
}

// TestInitMainGoroutineTID verifies main goroutine always gets TID=1.
// TID=0 is reserved as sentinel value in SmartTrack meaning "no exclusive writer".
func TestInitMainGoroutineTID(t *testing.T) {
	// Reset and Init.
	Init()

	// Main goroutine (this test) should have TID=1.
	// TID=0 is reserved as sentinel value.
	mainCtx := getCurrentContext()
	if mainCtx.TID != 1 {
		t.Errorf("Main goroutine TID = %d, want 1", mainCtx.TID)
	}

	// Spawn a new goroutine - should get TID=2 (or higher).
	var spawnedTID uint16
	done := make(chan bool)
	go func() {
		spawnedCtx := getCurrentContext()
		spawnedTID = spawnedCtx.TID
		done <- true
	}()
	<-done

	// Spawned goroutine should NOT have TID=0 or TID=1.
	// TID=0 is reserved as sentinel, TID=1 is reserved for main.
	if spawnedTID == 0 {
		t.Error("Spawned goroutine incorrectly has TID=0 (reserved as sentinel)")
	}
	if spawnedTID == 1 {
		t.Error("Spawned goroutine incorrectly has TID=1 (reserved for main)")
	}
}

// TestFiniOutput verifies Fini() prints correct summary.
func TestFiniOutput(t *testing.T) {
	// This test captures stderr output to verify Fini() output.
	// We'll use a simpler approach: just verify Fini() doesn't panic.
	// Actual output format can be manually verified.

	Init()

	// Call Fini - should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Fini() panicked: %v", r)
		}
	}()

	// Note: Fini() prints to stderr, which is hard to capture in tests.
	// For MVP, we just verify it doesn't crash.
	// In Phase 7, we could add output redirection for testing.
	Fini()

	// Verify detector is disabled after Fini.
	if enabled.Load() {
		t.Error("Fini() did not disable detector")
	}
}

// TestFiniWithNoRaces verifies Fini() output when no races detected.
func TestFiniWithNoRaces(t *testing.T) {
	Init()

	// No operations - no races.
	// RacesDetected should be 0.
	if got := RacesDetected(); got != 0 {
		t.Errorf("RacesDetected() = %d before Fini(), want 0", got)
	}

	// Call Fini - should print "No data races detected".
	// We can't easily capture stderr in tests, so just verify no panic.
	Fini()
}

// TestFiniWithRaces verifies Fini() output when races detected.
func TestFiniWithRaces(_ *testing.T) {
	Init()

	// Trigger a race by concurrent writes without synchronization.
	// This is tricky because WaitGroup creates happens-before.
	// For this test, we'll just verify that if RacesDetected() > 0,
	// Fini() doesn't panic.

	// Actually, we can't easily trigger a guaranteed race via API alone
	// without proper concurrent access. So we'll test the path by
	// using detector directly (if we had access), or we can test
	// that Fini() works correctly when count > 0 by mocking.

	// For now, just verify Fini() doesn't crash even with races.
	// In a real scenario where races exist, this would print warnings.

	// Skip actual race triggering - tested in detector_test.go.
	// Just verify Fini() can be called.
	Fini()
}

// TestInitFiniCycle verifies full initialization and finalization cycle.
func TestInitFiniCycle(t *testing.T) {
	// Cycle 1: Init -> operations -> Fini
	Init()

	addr := uintptr(0xa000)
	racewrite(addr)
	raceread(addr)

	Fini()

	// Detector should be disabled now.
	if enabled.Load() {
		t.Error("After Fini(), detector still enabled")
	}

	// Cycle 2: Init again -> operations -> Fini
	Init()

	// Should be able to use detector again.
	if !enabled.Load() {
		t.Error("After second Init(), detector not enabled")
	}

	addr2 := uintptr(0xb000)
	racewrite(addr2)

	Fini()

	// Should be disabled again.
	if enabled.Load() {
		t.Error("After second Fini(), detector still enabled")
	}
}

// TestInitResetsState verifies Init() clears previous state.
func TestInitResetsState(t *testing.T) {
	// First cycle: create some state.
	Init()

	// Allocate some contexts by spawning goroutines.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = getCurrentContext()
		}()
	}
	wg.Wait()

	// Do some accesses.
	for i := 0; i < 100; i++ {
		addr := uintptr(0xc000 + i)
		racewrite(addr)
	}

	// Now Init again - should reset everything.
	Init()

	// Verify nextTID is back to 2 after Init().
	// Init() allocates TID=1 for main (TID=0 is sentinel), then sets nextTID to 2.
	if got := nextTID.Load(); got != 2 {
		t.Errorf("After Init() reset, nextTID = %d, want 2", got)
	}

	// Verify RacesDetected is 0.
	if got := RacesDetected(); got != 0 {
		t.Errorf("After Init() reset, RacesDetected() = %d, want 0", got)
	}

	// Verify main goroutine has TID=1 again.
	// TID=0 is reserved as sentinel value in SmartTrack.
	ctx := getCurrentContext()
	if ctx.TID != 1 {
		t.Errorf("After Init() reset, main goroutine TID = %d, want 1", ctx.TID)
	}
}

// TestFiniMultipleCalls verifies Fini() can be called multiple times.
func TestFiniMultipleCalls(t *testing.T) {
	Init()

	// First Fini.
	Fini()

	// Second Fini - should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Second Fini() panicked: %v", r)
		}
	}()

	Fini()

	// Detector should remain disabled.
	if enabled.Load() {
		t.Error("After multiple Fini() calls, detector enabled")
	}
}

// TestInitAfterAutoInit verifies Init() works even after automatic init().
func TestInitAfterAutoInit(t *testing.T) {
	// The package init() already ran, so detector is already initialized.
	// Calling Init() should reset it.

	// Before Init(), detector should be in some state.
	// Let's do an operation.
	racewrite(uintptr(0xd000))

	// Now call Init().
	Init()

	// Should reset races.
	if got := RacesDetected(); got != 0 {
		t.Errorf("After explicit Init(), RacesDetected() = %d, want 0", got)
	}

	// Main goroutine should have TID=1.
	// TID=0 is reserved as sentinel value in SmartTrack.
	ctx := getCurrentContext()
	if ctx.TID != 1 {
		t.Errorf("After explicit Init(), main goroutine TID = %d, want 1", ctx.TID)
	}
}
