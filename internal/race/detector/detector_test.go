package detector

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/goroutine"
)

// TestNewDetector verifies that NewDetector creates a properly initialized detector.
func TestNewDetector(t *testing.T) {
	d := NewDetector()

	if d == nil {
		t.Fatal("NewDetector() returned nil")
	}

	if d.shadowMemory == nil {
		t.Error("shadowMemory not initialized")
	}

	if d.racesDetected != 0 {
		t.Errorf("racesDetected = %d, want 0", d.racesDetected)
	}
}

// TestOnWrite_FirstAccess tests that the first write to an address initializes
// the shadow cell without reporting a race.
func TestOnWrite_FirstAccess(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	// First write should not report a race.
	d.OnWrite(addr, ctx)

	if d.RacesDetected() != 0 {
		t.Errorf("First write reported race, want 0 races")
	}

	// Verify shadow cell was created and updated.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created for first write")
	}

	// Check that write epoch was set (should be non-zero after IncrementClock).
	if vs.GetW() == 0 {
		t.Error("Write epoch not set after first write")
	}
}

// TestOnWrite_SameEpochFastPath tests the same-epoch optimization where
// multiple writes in the same epoch should use the fast path.
func TestOnWrite_SameEpochFastPath(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x2000)

	// First write.
	d.OnWrite(addr, ctx)
	initialEpoch := ctx.GetEpoch()

	// Get initial write epoch from shadow memory.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}
	firstWriteEpoch := vs.GetW()

	// Manually set the write epoch back to test same-epoch path.
	// In real scenario, this would happen when vs.GetW() == currentEpoch on entry.
	vs.SetW(initialEpoch)

	// Second write should hit fast path (same epoch).
	d.OnWrite(addr, ctx)

	// No race should be reported.
	if d.RacesDetected() != 0 {
		t.Errorf("Same-epoch write reported race, want 0 races")
	}

	// Verify first write epoch was captured
	if firstWriteEpoch == 0 {
		t.Log("First write epoch was zero (expected for fresh VarState)")
	}
}

// TestOnWrite_WriteWriteRace tests detection of write-write races.
//
// Scenario: Two writes to the same address from the same thread,
// but with happens-before violation (simulated by manually manipulating epochs).
func TestOnWrite_WriteWriteRace(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x3000)

	// First write at epoch (1, 10).
	ctx.C.Set(1, 10)
	ctx.Epoch = epoch.NewEpoch(1, 10)
	d.OnWrite(addr, ctx)

	// Simulate a happens-before violation by setting a conflicting epoch.
	// We'll manually set vs.W to a future epoch that doesn't happen-before current.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}

	// Set previous write to epoch (1, 20) - a "future" write.
	vs.SetW(epoch.NewEpoch(1, 20))

	// Reset context to earlier time (1, 5) to create a race condition.
	ctx.C.Set(1, 5)
	ctx.Epoch = epoch.NewEpoch(1, 5)

	// Capture stderr to verify race report.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Second write should detect write-write race.
	d.OnWrite(addr, ctx)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read captured output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify race was detected.
	if d.RacesDetected() != 1 {
		t.Errorf("Write-write race not detected, got %d races", d.RacesDetected())
	}

	// Verify race report contains expected information.
	if output == "" {
		t.Error("No race report printed to stderr")
	}

	// Check for key elements in race report (Phase 5 Task 5.1 new format).
	expectedStrings := []string{
		"DATA RACE",
		"Write at 0x0000000000003000",          // Current write
		"Previous Write at 0x0000000000003000", // Previous write
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Race report missing expected string: %q\nGot:\n%s", expected, output)
		}
	}
}

// TestOnWrite_ReadWriteRace tests detection of read-write races.
//
// Scenario: A read followed by a write with happens-before violation.
func TestOnWrite_ReadWriteRace(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x4000)

	// Simulate a previous read by manually setting shadow cell.
	vs := d.shadowMemory.GetOrCreate(addr)

	// Set previous read to epoch (1, 20) - a "future" read.
	vs.SetReadEpoch(epoch.NewEpoch(1, 20))

	// Set current context to earlier time (1, 5) to create race.
	ctx.C.Set(1, 5)
	ctx.Epoch = epoch.NewEpoch(1, 5)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Write should detect read-write race.
	d.OnWrite(addr, ctx)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read captured output.
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify race was detected.
	if d.RacesDetected() != 1 {
		t.Errorf("Read-write race not detected, got %d races", d.RacesDetected())
	}

	// Verify race report (Phase 5 Task 5.1 new format).
	output := buf.String()
	if !strings.Contains(output, "Write at") {
		t.Error("Race report should contain 'Write at' (current access)")
	}
	if !strings.Contains(output, "Previous Read at") {
		t.Error("Race report should contain 'Previous Read at'")
	}
}

// TestOnWrite_NoRaceWithHappensBefore tests that synchronized writes
// (with proper happens-before relationships) do NOT report races.
func TestOnWrite_NoRaceWithHappensBefore(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x5000)

	// First write at epoch (1, 10).
	ctx.C.Set(1, 10)
	ctx.Epoch = epoch.NewEpoch(1, 10)
	d.OnWrite(addr, ctx)

	// Advance clock to establish happens-before.
	ctx.C.Set(1, 20)
	ctx.Epoch = epoch.NewEpoch(1, 20)

	// Second write at epoch (1, 20) happens-after first write.
	d.OnWrite(addr, ctx)

	// No race should be detected (proper ordering).
	if d.RacesDetected() != 0 {
		t.Errorf("Synchronized writes reported race, got %d races", d.RacesDetected())
	}
}

// TestOnWrite_MultipleAddresses tests that writes to different addresses
// are tracked independently.
func TestOnWrite_MultipleAddresses(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr1 := uintptr(0x6000)
	addr2 := uintptr(0x7000)
	addr3 := uintptr(0x8000)

	// Write to three different addresses.
	d.OnWrite(addr1, ctx)
	d.OnWrite(addr2, ctx)
	d.OnWrite(addr3, ctx)

	// No races should be detected.
	if d.RacesDetected() != 0 {
		t.Errorf("Writes to different addresses reported races")
	}

	// Verify each address has its own shadow cell.
	vs1 := d.shadowMemory.Get(addr1)
	vs2 := d.shadowMemory.Get(addr2)
	vs3 := d.shadowMemory.Get(addr3)

	if vs1 == nil || vs2 == nil || vs3 == nil {
		t.Error("Shadow cells not created for all addresses")
	}

	// Verify cells are different instances.
	if vs1 == vs2 || vs2 == vs3 || vs1 == vs3 {
		t.Error("Shadow cells should be distinct instances")
	}
}

// TestOnWrite_UpdatesShadowMemory tests that OnWrite correctly updates
// the shadow memory write epoch.
func TestOnWrite_UpdatesShadowMemory(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x9000)

	// Get initial epoch.
	initialEpoch := ctx.GetEpoch()

	// Write to address.
	d.OnWrite(addr, ctx)

	// Get shadow cell.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}

	// Verify write epoch was updated (should be after initial due to IncrementClock).
	// The exact value depends on when IncrementClock is called in OnWrite.
	if vs.GetW() == 0 {
		t.Error("Write epoch not updated in shadow memory")
	}

	// The write epoch should be based on the context's TID (1).
	tid, _ := vs.GetW().Decode()
	if tid != 1 {
		t.Errorf("Write epoch TID = %d, want 1", tid)
	}

	_ = initialEpoch // Suppress unused warning
}

// TestOnWrite_IncrementsLogicalClock tests that OnWrite advances the
// logical clock after processing.
func TestOnWrite_IncrementsLogicalClock(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0xA000)

	// Get initial clock value.
	initialClock := ctx.C.Get(1)

	// Perform write.
	d.OnWrite(addr, ctx)

	// Get new clock value.
	newClock := ctx.C.Get(1)

	// Clock should have incremented.
	if newClock <= initialClock {
		t.Errorf("Logical clock not incremented: initial=%d, new=%d", initialClock, newClock)
	}
}

// TestRacesDetected tests the RacesDetected counter.
func TestRacesDetected(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)

	if d.RacesDetected() != 0 {
		t.Errorf("Initial RacesDetected = %d, want 0", d.RacesDetected())
	}

	// Trigger a race by manipulating epochs.
	addr := uintptr(0xB000)
	vs := d.shadowMemory.GetOrCreate(addr)
	vs.SetW(epoch.NewEpoch(1, 100)) // Future write))
	vs.SetExclusiveWriter(-1)       // Force shared state for full FastTrack check
	ctx.C.Set(1, 50)                // Earlier time
	ctx.Epoch = epoch.NewEpoch(1, 50)

	// Suppress stderr for this test.
	oldStderr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)

	d.OnWrite(addr, ctx)

	os.Stderr = oldStderr

	// Check counter incremented.
	if d.RacesDetected() != 1 {
		t.Errorf("RacesDetected = %d, want 1", d.RacesDetected())
	}
}

// TestReset tests that Reset clears all detector state.
func TestReset(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0xC000)

	// Perform some operations.
	d.OnWrite(addr, ctx)

	// Trigger a race to increment counter.
	vs := d.shadowMemory.GetOrCreate(addr)
	vs.SetW(epoch.NewEpoch(1, 100))
	vs.SetExclusiveWriter(-1) // Force shared state for full FastTrack check
	ctx.C.Set(1, 50)
	ctx.Epoch = epoch.NewEpoch(1, 50)

	oldStderr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	d.OnWrite(addr, ctx)
	os.Stderr = oldStderr

	// Verify state before reset.
	if d.RacesDetected() == 0 {
		t.Error("Expected races before reset")
	}

	// Reset detector.
	d.Reset()

	// Verify state after reset.
	if d.RacesDetected() != 0 {
		t.Errorf("RacesDetected after reset = %d, want 0", d.RacesDetected())
	}

	// Verify shadow memory was cleared.
	if d.shadowMemory.Get(addr) != nil {
		t.Error("Shadow memory not cleared after reset")
	}
}

// TestHappensBeforeWrite tests the happensBeforeWrite logic.
func TestHappensBeforeWrite(t *testing.T) {
	d := NewDetector()

	tests := []struct {
		name       string
		prevWrite  epoch.Epoch
		currClock  uint64
		wantResult bool
	}{
		{
			name:       "Same thread, earlier write",
			prevWrite:  epoch.NewEpoch(1, 10),
			currClock:  20,
			wantResult: true, // 10 <= 20
		},
		{
			name:       "Same thread, same clock",
			prevWrite:  epoch.NewEpoch(1, 15),
			currClock:  15,
			wantResult: true, // 15 <= 15
		},
		{
			name:       "Same thread, later write (race)",
			prevWrite:  epoch.NewEpoch(1, 30),
			currClock:  20,
			wantResult: false, // 30 > 20 (race!)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup context with specified clock.
			ctx := goroutine.Alloc(1)
			ctx.C.Set(1, uint32(tt.currClock))
			ctx.Epoch = epoch.NewEpoch(1, tt.currClock)

			result := d.happensBeforeWrite(tt.prevWrite, ctx)

			if result != tt.wantResult {
				t.Errorf("happensBeforeWrite = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

// TestHappensBeforeRead tests the happensBeforeRead logic.
func TestHappensBeforeRead(t *testing.T) {
	d := NewDetector()

	// For MVP, happensBeforeRead is identical to happensBeforeWrite.
	tests := []struct {
		name       string
		prevRead   epoch.Epoch
		currClock  uint64
		wantResult bool
	}{
		{
			name:       "Earlier read",
			prevRead:   epoch.NewEpoch(1, 5),
			currClock:  10,
			wantResult: true,
		},
		{
			name:       "Later read (race)",
			prevRead:   epoch.NewEpoch(1, 15),
			currClock:  10,
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := goroutine.Alloc(1)
			ctx.C.Set(1, uint32(tt.currClock))
			ctx.Epoch = epoch.NewEpoch(1, tt.currClock)

			result := d.happensBeforeRead(tt.prevRead, ctx)

			if result != tt.wantResult {
				t.Errorf("happensBeforeRead = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

// TestReportRace tests the race reporting function.
func TestReportRace(t *testing.T) {
	d := NewDetector()

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Report a race.
	addr := uintptr(0xDEADBEEF)
	prevEpoch := epoch.NewEpoch(2, 100)
	currEpoch := epoch.NewEpoch(3, 200)

	d.reportRace("test-race", addr, prevEpoch, currEpoch)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read output.
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected elements.
	expectedStrings := []string{
		"DATA RACE",
		"test-race",
		"0xdeadbeef", // Lowercase hex
		"100@2",      // Previous epoch
		"200@3",      // Current epoch
	}

	for _, expected := range expectedStrings {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Race report missing expected string: %q\nGot:\n%s", expected, output)
		}
	}

	// Verify race counter was incremented.
	if d.RacesDetected() != 1 {
		t.Errorf("RacesDetected = %d, want 1", d.RacesDetected())
	}
}

// TestConcurrentWrites tests thread-safety of OnWrite.
//
// This test spawns multiple goroutines writing to the same detector
// to verify that concurrent access doesn't cause panics or data corruption.
//
// Note: This is a basic concurrency test. Full multi-threaded race detection
// will be implemented in Task 1.8 (goroutine ID extraction).
func TestConcurrentWrites(_ *testing.T) {
	d := NewDetector()

	const numGoroutines = 10
	const writesPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	// Spawn concurrent writers.
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Each goroutine gets its own context with unique TID
			ctx := goroutine.Alloc(uint16(id + 1))
			baseAddr := uintptr(0x10000 + id*0x1000)
			for j := 0; j < writesPerGoroutine; j++ {
				addr := baseAddr + uintptr(j)
				d.OnWrite(addr, ctx)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete.
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Test passes if no panics occurred.
	// Exact race count is non-deterministic in MVP (single test context).
}

// TestOnRead_FirstAccess tests that the first read to an address initializes
// the shadow cell without reporting a race.
func TestOnRead_FirstAccess(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x1000)

	// First read should not report a race.
	d.OnRead(addr, ctx)

	if d.RacesDetected() != 0 {
		t.Errorf("First read reported race, want 0 races")
	}

	// Verify shadow cell was created and updated.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created for first read")
	}

	// Check that read epoch was set (should be non-zero after IncrementClock).
	if vs.GetReadEpoch() == 0 {
		t.Error("Read epoch not set after first read")
	}
}

// TestOnRead_SameEpochFastPath tests the same-epoch optimization where
// multiple reads in the same epoch should use the fast path.
func TestOnRead_SameEpochFastPath(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x2000)

	// First read.
	d.OnRead(addr, ctx)
	initialEpoch := ctx.GetEpoch()

	// Get initial read epoch from shadow memory.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}

	// Manually set the read epoch back to test same-epoch path.
	vs.SetReadEpoch(initialEpoch)

	// Second read should hit fast path (same epoch).
	d.OnRead(addr, ctx)

	// No race should be reported.
	if d.RacesDetected() != 0 {
		t.Errorf("Same-epoch read reported race, want 0 races")
	}
}

// TestOnRead_WriteReadRace tests detection of write-read races.
//
// Scenario: A write followed by a read with happens-before violation.
func TestOnRead_WriteReadRace(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x3000)

	// Simulate a previous write by manually setting shadow cell.
	vs := d.shadowMemory.GetOrCreate(addr)

	// Set previous write to epoch (1, 20) - a "future" write.
	vs.SetW(epoch.NewEpoch(1, 20))

	// Set current context to earlier time (1, 5) to create race.
	ctx.C.Set(1, 5)
	ctx.Epoch = epoch.NewEpoch(1, 5)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Read should detect write-read race.
	d.OnRead(addr, ctx)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read captured output.
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify race was detected.
	if d.RacesDetected() != 1 {
		t.Errorf("Write-read race not detected, got %d races", d.RacesDetected())
	}

	// Verify race report (Phase 5 Task 5.1 new format).
	output := buf.String()
	if !strings.Contains(output, "Read at") {
		t.Error("Race report should contain 'Read at' (current access)")
	}
	if !strings.Contains(output, "Previous Write at") {
		t.Error("Race report should contain 'Previous Write at'")
	}
}

// TestOnRead_NoRaceWithHappensBefore tests that synchronized reads
// (with proper happens-before relationships) do NOT report races.
func TestOnRead_NoRaceWithHappensBefore(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x4000)

	// Simulate a write at epoch (1, 10).
	vs := d.shadowMemory.GetOrCreate(addr)
	vs.SetW(epoch.NewEpoch(1, 10))

	// Set context to later time (1, 20) - read happens-after write.
	ctx.C.Set(1, 20)
	ctx.Epoch = epoch.NewEpoch(1, 20)

	// Read should NOT detect a race (proper ordering).
	d.OnRead(addr, ctx)

	// No race should be detected.
	if d.RacesDetected() != 0 {
		t.Errorf("Synchronized read reported race, got %d races", d.RacesDetected())
	}
}

// TestOnRead_NoWriteBefore tests that reads without prior writes work correctly.
func TestOnRead_NoWriteBefore(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x5000)

	// Read from address that has never been written to.
	d.OnRead(addr, ctx)

	// No race should be reported (no previous write).
	if d.RacesDetected() != 0 {
		t.Errorf("Read without prior write reported race, got %d races", d.RacesDetected())
	}

	// Verify read epoch was set.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}

	if vs.GetReadEpoch() == 0 {
		t.Error("Read epoch not set")
	}

	// Write epoch should still be zero (no write occurred).
	if vs.GetW() != 0 {
		t.Error("Write epoch should be zero (no write occurred)")
	}
}

// TestOnRead_MultipleReads tests that multiple reads to the same address
// update the read epoch correctly.
func TestOnRead_MultipleReads(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0x6000)

	// First read.
	d.OnRead(addr, ctx)
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}
	firstReadEpoch := vs.GetReadEpoch()

	// Advance clock for second read.
	ctx.IncrementClock()
	ctx.Epoch = ctx.GetEpoch()

	// Second read.
	d.OnRead(addr, ctx)

	// Read epoch should have been updated.
	secondReadEpoch := vs.GetReadEpoch()
	if secondReadEpoch.Same(firstReadEpoch) {
		t.Error("Read epoch not updated on second read")
	}

	// No races should be reported.
	if d.RacesDetected() != 0 {
		t.Errorf("Multiple reads reported race, got %d races", d.RacesDetected())
	}
}

// TestOnRead_MultipleAddresses tests that reads to different addresses
// are tracked independently.
func TestOnRead_MultipleAddresses(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr1 := uintptr(0x7000)
	addr2 := uintptr(0x8000)
	addr3 := uintptr(0x9000)

	// Read from three different addresses.
	d.OnRead(addr1, ctx)
	d.OnRead(addr2, ctx)
	d.OnRead(addr3, ctx)

	// No races should be detected.
	if d.RacesDetected() != 0 {
		t.Errorf("Reads to different addresses reported races")
	}

	// Verify each address has its own shadow cell.
	vs1 := d.shadowMemory.Get(addr1)
	vs2 := d.shadowMemory.Get(addr2)
	vs3 := d.shadowMemory.Get(addr3)

	if vs1 == nil || vs2 == nil || vs3 == nil {
		t.Error("Shadow cells not created for all addresses")
	}

	// Verify cells are different instances.
	if vs1 == vs2 || vs2 == vs3 || vs1 == vs3 {
		t.Error("Shadow cells should be distinct instances")
	}

	// Verify all have read epochs set.
	if vs1.GetReadEpoch() == 0 || vs2.GetReadEpoch() == 0 || vs3.GetReadEpoch() == 0 {
		t.Error("Not all read epochs were set")
	}
}

// TestOnRead_UpdatesShadowMemory tests that OnRead correctly updates
// the shadow memory read epoch.
func TestOnRead_UpdatesShadowMemory(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0xA000)

	// Read from address.
	d.OnRead(addr, ctx)

	// Get shadow cell.
	vs := d.shadowMemory.Get(addr)
	if vs == nil {
		t.Fatal("Shadow cell not created")
	}

	// Verify read epoch was updated.
	if vs.GetReadEpoch() == 0 {
		t.Error("Read epoch not updated in shadow memory")
	}

	// The read epoch should be based on the context's TID (1).
	tid, _ := vs.GetReadEpoch().Decode()
	if tid != 1 {
		t.Errorf("Read epoch TID = %d, want 1", tid)
	}
}

// TestOnRead_IncrementsLogicalClock tests that OnRead advances the
// logical clock after processing.
func TestOnRead_IncrementsLogicalClock(t *testing.T) {
	d := NewDetector()
	ctx := goroutine.Alloc(1)
	addr := uintptr(0xB000)

	// Get initial clock value.
	initialClock := ctx.C.Get(1)

	// Perform read.
	d.OnRead(addr, ctx)

	// Get new clock value.
	newClock := ctx.C.Get(1)

	// Clock should have incremented.
	if newClock <= initialClock {
		t.Errorf("Logical clock not incremented: initial=%d, new=%d", initialClock, newClock)
	}
}

// TestOnRead_Integration_WithWrite tests integration of OnRead and OnWrite.
//
// This test verifies that writes and reads interact correctly to detect races.
func TestOnRead_Integration_WithWrite(t *testing.T) {
	d := NewDetector()
	addr := uintptr(0xC000)

	tests := []struct {
		name        string
		setup       func() *goroutine.RaceContext
		operation   func(*goroutine.RaceContext)
		wantRaces   int
		description string
	}{
		{
			name: "Write then Read (synchronized)",
			setup: func() *goroutine.RaceContext {
				d.Reset()
				ctx := goroutine.Alloc(1)
				ctx.C.Set(1, 10)
				ctx.Epoch = epoch.NewEpoch(1, 10)
				return ctx
			},
			operation: func(ctx *goroutine.RaceContext) {
				d.OnWrite(addr, ctx)
				// Advance time to establish happens-before.
				ctx.C.Set(1, 20)
				ctx.Epoch = epoch.NewEpoch(1, 20)
				d.OnRead(addr, ctx)
			},
			wantRaces:   0,
			description: "Read after write with proper ordering should not race",
		},
		{
			name: "Read then Write (synchronized)",
			setup: func() *goroutine.RaceContext {
				d.Reset()
				ctx := goroutine.Alloc(1)
				ctx.C.Set(1, 10)
				ctx.Epoch = epoch.NewEpoch(1, 10)
				return ctx
			},
			operation: func(ctx *goroutine.RaceContext) {
				d.OnRead(addr, ctx)
				// Advance time.
				ctx.C.Set(1, 20)
				ctx.Epoch = epoch.NewEpoch(1, 20)
				d.OnWrite(addr, ctx)
			},
			wantRaces:   0,
			description: "Write after read with proper ordering should not race",
		},
		{
			name: "Multiple Reads (no race)",
			setup: func() *goroutine.RaceContext {
				d.Reset()
				ctx := goroutine.Alloc(1)
				ctx.C.Set(1, 10)
				ctx.Epoch = epoch.NewEpoch(1, 10)
				return ctx
			},
			operation: func(ctx *goroutine.RaceContext) {
				d.OnRead(addr, ctx)
				ctx.IncrementClock()
				ctx.Epoch = ctx.GetEpoch()
				d.OnRead(addr, ctx)
				ctx.IncrementClock()
				ctx.Epoch = ctx.GetEpoch()
				d.OnRead(addr, ctx)
			},
			wantRaces:   0,
			description: "Multiple reads should not race with each other",
		},
	}

	// Suppress stderr for race tests.
	oldStderr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldStderr }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			tt.operation(ctx)

			if d.RacesDetected() != tt.wantRaces {
				t.Errorf("%s: got %d races, want %d", tt.description, d.RacesDetected(), tt.wantRaces)
			}
		})
	}
}

// TestConcurrentReads tests thread-safety of OnRead.
//
// This test spawns multiple goroutines reading from the same detector
// to verify that concurrent access doesn't cause panics or data corruption.
func TestConcurrentReads(_ *testing.T) {
	d := NewDetector()

	const numGoroutines = 10
	const readsPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	// Spawn concurrent readers.
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Each goroutine gets its own context with unique TID
			ctx := goroutine.Alloc(uint16(id + 1))
			baseAddr := uintptr(0x20000 + id*0x1000)
			for j := 0; j < readsPerGoroutine; j++ {
				addr := baseAddr + uintptr(j)
				d.OnRead(addr, ctx)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete.
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Test passes if no panics occurred.
}

// TestConcurrentReadsAndWrites tests concurrent reads and writes.
func TestConcurrentReadsAndWrites(_ *testing.T) {
	d := NewDetector()

	const numGoroutines = 10
	const opsPerGoroutine = 100

	done := make(chan bool, numGoroutines*2)

	// Spawn concurrent readers.
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Each goroutine gets its own context with unique TID
			ctx := goroutine.Alloc(uint16(id + 1))
			baseAddr := uintptr(0x30000 + id*0x1000)
			for j := 0; j < opsPerGoroutine; j++ {
				addr := baseAddr + uintptr(j)
				d.OnRead(addr, ctx)
			}
			done <- true
		}(i)
	}

	// Spawn concurrent writers.
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Each goroutine gets its own context with unique TID
			ctx := goroutine.Alloc(uint16(id + numGoroutines + 1))
			baseAddr := uintptr(0x40000 + id*0x1000)
			for j := 0; j < opsPerGoroutine; j++ {
				addr := baseAddr + uintptr(j)
				d.OnWrite(addr, ctx)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete.
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}

	// Test passes if no panics occurred.
}
