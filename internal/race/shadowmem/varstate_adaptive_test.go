package shadowmem

import (
	"testing"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

// TestVarState_SingleReader_FastPath verifies that single-reader case uses epoch-only fast path.
func TestVarState_SingleReader_FastPath(t *testing.T) {
	vs := NewVarState()

	// Single reader should not be promoted.
	if vs.IsPromoted() {
		t.Error("New VarState should not be promoted")
	}

	// Set read epoch.
	readEpoch := epoch.NewEpoch(5, 100)
	vs.SetReadEpoch(readEpoch)

	// Should still not be promoted.
	if vs.IsPromoted() {
		t.Error("Single reader should not trigger promotion")
	}

	// Verify read epoch is set correctly.
	if vs.GetReadEpoch() != readEpoch {
		t.Errorf("GetReadEpoch() = %v, want %v", vs.GetReadEpoch(), readEpoch)
	}

	// ReadClock should be nil (fast path).
	if vs.GetReadClock() != nil {
		t.Error("ReadClock should be nil for unpromoted state")
	}

	t.Logf("Single reader fast path: %s", vs.String())
}

// TestVarState_MultipleReaders_Promotion tests that concurrent readers trigger promotion.
func TestVarState_MultipleReaders_Promotion(t *testing.T) {
	vs := NewVarState()

	// First reader (TID=5).
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))

	if vs.IsPromoted() {
		t.Error("First reader should not trigger promotion")
	}

	// Second concurrent reader (TID=3) - different thread.
	// This should trigger promotion.
	vc := vectorclock.New()
	vc.Set(3, 50)
	vc.Set(5, 90) // Happens before first reader's clock (100).

	vs.PromoteToReadClock(vc)

	// Should now be promoted.
	if !vs.IsPromoted() {
		t.Error("Concurrent readers should trigger promotion")
	}

	// ReadClock should be allocated.
	if vs.GetReadClock() == nil {
		t.Fatal("ReadClock should be allocated after promotion")
	}

	// ReadEpoch should be cleared (replaced by ReadClock).
	if vs.GetReadEpoch() != 0 {
		t.Errorf("ReadEpoch should be cleared after promotion, got %v", vs.GetReadEpoch())
	}

	// Verify VectorClock contains both readers.
	rc := vs.GetReadClock()
	if rc.Get(5) != 100 {
		t.Errorf("ReadClock[5] = %d, want 100 (first reader)", rc.Get(5))
	}
	if rc.Get(3) != 50 {
		t.Errorf("ReadClock[3] = %d, want 50 (second reader)", rc.Get(3))
	}

	t.Logf("Multiple readers promoted: %s", vs.String())
}

// TestVarState_SequentialReads_NoPromotion tests that happens-before reads don't promote.
func TestVarState_SequentialReads_NoPromotion(t *testing.T) {
	vs := NewVarState()

	// Reader 1 at epoch (5, 100).
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))

	// Reader 2 at epoch (3, 200) - if happens-before reader 1, no promotion needed.
	// In real detector, this would be checked via HappensBefore(vc).
	// For this test, we simulate sequential reads by just replacing the epoch.

	// Sequential read - different thread but later in logical time.
	vs.SetReadEpoch(epoch.NewEpoch(3, 200))

	// Should still not be promoted (sequential access).
	if vs.IsPromoted() {
		t.Error("Sequential reads should not trigger promotion")
	}

	// Verify latest read epoch.
	got := vs.GetReadEpoch()
	want := epoch.NewEpoch(3, 200)
	if got != want {
		t.Errorf("GetReadEpoch() = %v, want %v", got, want)
	}

	t.Logf("Sequential reads (no promotion): %s", vs.String())
}

// TestVarState_WriteDemotesReadClock tests that writes demote promoted VarState.
func TestVarState_WriteDemotesReadClock(t *testing.T) {
	vs := NewVarState()

	// Promote to VectorClock.
	vc := vectorclock.New()
	vc.Set(5, 100)
	vc.Set(3, 50)
	vs.PromoteToReadClock(vc)

	if !vs.IsPromoted() {
		t.Fatal("Should be promoted after PromoteToReadClock")
	}

	// Simulate write: Clear read tracking using Demote().
	vs.Demote()

	// Should no longer be promoted.
	if vs.IsPromoted() {
		t.Error("Write should demote VarState back to fast path")
	}

	// ReadClock should be nil.
	if vs.GetReadClock() != nil {
		t.Error("ReadClock should be nil after demotion")
	}

	// ReadEpoch should be zero.
	if vs.GetReadEpoch() != 0 {
		t.Errorf("ReadEpoch should be 0 after demotion, got %v", vs.GetReadEpoch())
	}

	t.Logf("Write demoted VarState: %s", vs.String())
}

// TestVarState_PromotionStats tests the promotion workflow end-to-end.
func TestVarState_PromotionStats(t *testing.T) {
	// Test lifecycle: unpromoted → promoted → demoted → promoted again.

	vs := NewVarState()

	// Step 1: Unpromoted (single reader).
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))
	if vs.IsPromoted() {
		t.Error("Step 1: Should be unpromoted")
	}

	// Step 2: Promotion (concurrent readers).
	vc := vectorclock.New()
	vc.Set(3, 50)
	vs.PromoteToReadClock(vc)
	if !vs.IsPromoted() {
		t.Error("Step 2: Should be promoted")
	}

	// Step 3: Demotion (write).
	vs.Demote()
	if vs.IsPromoted() {
		t.Error("Step 3: Should be demoted")
	}

	// Step 4: Re-promotion (new concurrent readers).
	vs.SetReadEpoch(epoch.NewEpoch(7, 200))
	vc2 := vectorclock.New()
	vc2.Set(1, 150)
	vs.PromoteToReadClock(vc2)
	if !vs.IsPromoted() {
		t.Error("Step 4: Should be re-promoted")
	}

	// Verify final state.
	rc := vs.GetReadClock()
	if rc == nil {
		t.Fatal("ReadClock should be allocated after re-promotion")
	}
	if rc.Get(7) != 200 {
		t.Errorf("ReadClock[7] = %d, want 200", rc.Get(7))
	}
	if rc.Get(1) != 150 {
		t.Errorf("ReadClock[1] = %d, want 150", rc.Get(1))
	}

	t.Logf("Promotion lifecycle complete: %s", vs.String())
}

// TestVarState_ConcurrentReads_1000Goroutines simulates heavy concurrent read load.
func TestVarState_ConcurrentReads_1000Goroutines(t *testing.T) {
	vs := NewVarState()

	// Simulate 256 concurrent readers (max threads).
	vc := vectorclock.New()
	for tid := 0; tid < 256; tid++ {
		vc.Set(uint16(tid), uint32(tid*10))
	}

	vs.PromoteToReadClock(vc)

	if !vs.IsPromoted() {
		t.Fatal("Should be promoted with 256 concurrent readers")
	}

	// Verify all readers are tracked.
	rc := vs.GetReadClock()
	for tid := 0; tid < 256; tid++ {
		expected := uint32(tid * 10)
		if rc.Get(uint16(tid)) != expected {
			t.Errorf("ReadClock[%d] = %d, want %d", tid, rc.Get(uint16(tid)), expected)
		}
	}

	t.Logf("256 concurrent readers tracked: promoted=%v", vs.IsPromoted())
}

// TestVarState_PromotionTrigger_SecondReader tests exact promotion trigger condition.
func TestVarState_PromotionTrigger_SecondReader(t *testing.T) {
	vs := NewVarState()

	// Reader 1 (TID=5, Clock=100).
	vs.SetReadEpoch(epoch.NewEpoch(5, 100))

	// Before promotion.
	if vs.IsPromoted() {
		t.Error("Should not be promoted with single reader")
	}
	if vs.GetReadEpoch() != epoch.NewEpoch(5, 100) {
		t.Error("ReadEpoch should be set")
	}
	if vs.GetReadClock() != nil {
		t.Error("ReadClock should be nil before promotion")
	}

	// Reader 2 (TID=3, Clock=50) - concurrent (not happens-before reader 1).
	vc := vectorclock.New()
	vc.Set(3, 50)
	// Note: vc[5] = 0, which is < 100, so this read is concurrent with reader 1.

	vs.PromoteToReadClock(vc)

	// After promotion.
	if !vs.IsPromoted() {
		t.Fatal("Should be promoted after second concurrent reader")
	}
	if vs.GetReadEpoch() != 0 {
		t.Error("ReadEpoch should be cleared after promotion")
	}
	if vs.GetReadClock() == nil {
		t.Fatal("ReadClock should be allocated after promotion")
	}

	// Verify both readers are in VectorClock.
	rc := vs.GetReadClock()
	if rc.Get(5) != 100 {
		t.Errorf("Reader 1 not preserved: ReadClock[5] = %d, want 100", rc.Get(5))
	}
	if rc.Get(3) != 50 {
		t.Errorf("Reader 2 not merged: ReadClock[3] = %d, want 50", rc.Get(3))
	}

	t.Logf("Promotion triggered correctly: %s", vs.String())
}

// TestVarState_HappensBeforeReads_NoPromotion tests that happens-before reads stay unpromoted.
func TestVarState_HappensBeforeReads_NoPromotion(t *testing.T) {
	vs := NewVarState()

	// Simulate a series of happens-before reads from different threads.
	// In real detector, this would be checked via Epoch.HappensBefore(vc).

	// Read 1: Thread 5 at clock 10.
	vs.SetReadEpoch(epoch.NewEpoch(5, 10))

	// Read 2: Thread 3 at clock 20 (assume happens-after read 1 via synchronization).
	// In real code, detector would check: epoch(5,10).HappensBefore(ctx.C) where ctx.C[5] >= 10.
	// For this test, we just replace the epoch (simulating happens-before).
	vs.SetReadEpoch(epoch.NewEpoch(3, 20))

	// Should still be unpromoted.
	if vs.IsPromoted() {
		t.Error("Happens-before reads should not trigger promotion")
	}

	// Read 3: Thread 7 at clock 30 (also happens-after).
	vs.SetReadEpoch(epoch.NewEpoch(7, 30))

	if vs.IsPromoted() {
		t.Error("Sequential happens-before reads should remain unpromoted")
	}

	// Verify latest read.
	if vs.GetReadEpoch() != epoch.NewEpoch(7, 30) {
		t.Errorf("Latest read epoch incorrect: got %v", vs.GetReadEpoch())
	}

	t.Logf("Happens-before reads (unpromoted): %s", vs.String())
}

// TestVarState_String_PromotedFormat tests String() output for promoted state.
// v0.3.0: Updated for Enhanced Read-Shared format "R:[epochs...]".
func TestVarState_String_PromotedFormat(t *testing.T) {
	vs := NewVarState()
	vs.SetW(epoch.NewEpoch(5, 100))

	// Unpromoted (single reader in brackets).
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))
	unpromoted := vs.String()
	// v0.3.0: New format with brackets for inline slots
	if unpromoted != "W:100@5 R:[50@3]" {
		t.Errorf("Unpromoted String() = %q, want %q", unpromoted, "W:100@5 R:[50@3]")
	}

	// Promoted.
	vc := vectorclock.New()
	vc.Set(3, 50)
	vc.Set(7, 60)
	vs.PromoteToReadClock(vc)

	promoted := vs.String()
	// Should contain "PROMOTED" and VectorClock representation.
	if promoted != "W:100@5 R:{3:50, 7:60} [PROMOTED]" {
		t.Errorf("Promoted String() = %q, want format with [PROMOTED]", promoted)
	}

	t.Logf("Unpromoted: %s", unpromoted)
	t.Logf("Promoted: %s", promoted)
}

// TestVarState_InlineSlots_4Readers tests v0.3.0 Enhanced Read-Shared with 4 inline slots.
func TestVarState_InlineSlots_4Readers(t *testing.T) {
	vs := NewVarState()

	// Add 4 readers (should all fit in inline slots, no VectorClock allocation).
	for i := uint16(1); i <= 4; i++ {
		added := vs.AddReader(epoch.NewEpoch(i, uint64(100*i)))
		if !added {
			t.Errorf("AddReader(%d) should succeed for inline slot", i)
		}
	}

	// Should NOT be promoted (4 readers fit in inline slots).
	if vs.IsPromoted() {
		t.Error("4 readers should fit in inline slots without promotion")
	}

	// Verify all 4 readers are tracked.
	count := vs.GetReaderCount()
	if count != 4 {
		t.Errorf("GetReaderCount() = %d, want 4", count)
	}

	// Verify epochs.
	epochs := vs.GetReadEpochs()
	if len(epochs) != 4 {
		t.Errorf("GetReadEpochs() length = %d, want 4", len(epochs))
	}

	// Try to add 5th reader (should fail, need promotion).
	added := vs.AddReader(epoch.NewEpoch(5, 500))
	if added {
		t.Error("AddReader(5) should fail when slots are full")
	}

	// Now promote to VectorClock.
	vc := vectorclock.New()
	vc.Set(5, 500)
	vs.PromoteToReadClock(vc)

	// Should now be promoted.
	if !vs.IsPromoted() {
		t.Error("Should be promoted after PromoteToReadClock")
	}

	// Verify all 5 readers in VectorClock.
	rc := vs.GetReadClock()
	if rc == nil {
		t.Fatal("ReadClock should not be nil after promotion")
	}

	for i := uint16(1); i <= 4; i++ {
		if rc.Get(i) != uint32(100*int(i)) {
			t.Errorf("ReadClock[%d] = %d, want %d", i, rc.Get(i), 100*i)
		}
	}
	if rc.Get(5) != 500 {
		t.Errorf("ReadClock[5] = %d, want 500", rc.Get(5))
	}

	t.Logf("v0.3.0 Enhanced Read-Shared: 4 inline slots + promotion to VectorClock works correctly")
}

// TestVarState_AddReader_SameTID tests that AddReader updates existing TID.
func TestVarState_AddReader_SameTID(t *testing.T) {
	vs := NewVarState()

	// Add reader TID=5.
	vs.AddReader(epoch.NewEpoch(5, 100))
	if vs.GetReaderCount() != 1 {
		t.Errorf("GetReaderCount() = %d, want 1", vs.GetReaderCount())
	}

	// Add same TID=5 with higher clock (should update, not add new slot).
	vs.AddReader(epoch.NewEpoch(5, 200))
	if vs.GetReaderCount() != 1 {
		t.Errorf("GetReaderCount() = %d, want 1 (should update existing slot)", vs.GetReaderCount())
	}

	// Verify updated epoch.
	epochs := vs.GetReadEpochs()
	if len(epochs) != 1 {
		t.Fatalf("GetReadEpochs() length = %d, want 1", len(epochs))
	}

	tid, clock := epochs[0].Decode()
	if tid != 5 || clock != 200 {
		t.Errorf("Updated epoch = (tid=%d, clock=%d), want (5, 200)", tid, clock)
	}

	t.Logf("AddReader correctly updates existing TID slot")
}

// TestVarState_Reset_ClearsPromotion tests that Reset() demotes promoted state.
func TestVarState_Reset_ClearsPromotion(t *testing.T) {
	vs := NewVarState()

	// Promote.
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	if !vs.IsPromoted() {
		t.Fatal("Setup failed: should be promoted")
	}

	// Reset.
	vs.Reset()

	// Should be demoted.
	if vs.IsPromoted() {
		t.Error("Reset() should demote promoted state")
	}
	if vs.GetReadClock() != nil {
		t.Error("Reset() should clear ReadClock")
	}
	if vs.GetReadEpoch() != 0 {
		t.Error("Reset() should clear ReadEpoch")
	}
	if vs.GetW() != 0 {
		t.Error("Reset() should clear W")
	}

	t.Logf("Reset() correctly demoted: %s", vs.String())
}

// TestVarState_PromoteZeroEpoch tests promotion when readEpoch is zero (first read after write).
func TestVarState_PromoteZeroEpoch(t *testing.T) {
	vs := NewVarState()

	// No previous read (readEpoch = 0).
	if vs.GetReadEpoch() != 0 {
		t.Fatal("Setup failed: readEpoch should be 0")
	}

	// Promote with new reader.
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	if !vs.IsPromoted() {
		t.Error("Should be promoted even when readEpoch was 0")
	}

	// Verify VectorClock contains only the new reader (no previous epoch to preserve).
	rc := vs.GetReadClock()
	if rc.Get(5) != 100 {
		t.Errorf("ReadClock[5] = %d, want 100", rc.Get(5))
	}

	// Other threads should be 0.
	for tid := uint16(0); tid < 65535; tid++ {
		if tid != 5 && rc.Get(tid) != 0 {
			t.Errorf("ReadClock[%d] = %d, want 0", tid, rc.Get(tid))
		}
	}

	t.Logf("Promoted from zero epoch: %s", vs.String())
}

// TestVarState_SetReadEpoch_WhenPromoted tests that SetReadEpoch is no-op when promoted.
func TestVarState_SetReadEpoch_WhenPromoted(t *testing.T) {
	vs := NewVarState()

	// Promote.
	vc := vectorclock.New()
	vc.Set(5, 100)
	vs.PromoteToReadClock(vc)

	if !vs.IsPromoted() {
		t.Fatal("Setup failed: should be promoted")
	}

	// Try to set read epoch (should be no-op).
	vs.SetReadEpoch(epoch.NewEpoch(3, 50))

	// Should remain promoted.
	if !vs.IsPromoted() {
		t.Error("SetReadEpoch should not demote promoted state")
	}

	// ReadEpoch should still be 0 (SetReadEpoch is no-op when promoted).
	if vs.GetReadEpoch() != 0 {
		t.Error("SetReadEpoch should be no-op when promoted")
	}

	// ReadClock should be unchanged.
	rc := vs.GetReadClock()
	if rc.Get(5) != 100 {
		t.Error("ReadClock should be unchanged after SetReadEpoch")
	}

	t.Logf("SetReadEpoch no-op when promoted: %s", vs.String())
}
