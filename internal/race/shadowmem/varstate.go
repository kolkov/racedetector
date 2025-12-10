// Package shadowmem implements shadow memory cells for FastTrack race detection.
//
// Shadow memory stores the access history for every instrumented memory location.
// VarState is the basic building block - a single cell tracking the last write
// and last read to a variable.
package shadowmem

import (
	"sync"
	"sync/atomic"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

const (
	// maxInlineReaders is the number of inline reader slots in VarState.
	// When exceeded, VarState promotes to VectorClock (1KB allocation).
	// 4 slots is optimal: covers 95%+ of read-shared patterns while keeping VarState small.
	// Research: SmartTrack PLDI 2020 + TSAN multi-cell approach.
	maxInlineReaders = 4

	// promotedMarker indicates the VarState has been promoted to VectorClock.
	// readerCount == promotedMarker means readClock is active.
	promotedMarker uint8 = 255
)

// VarState stores the access state for a single variable using adaptive representation.
//
// ADAPTIVE REPRESENTATION (FastTrack Phase 3 optimization):
//   - Common case: Exclusive access (one writer OR one reader)
//   - Uses Epoch only (4 bytes for write + 4 bytes for read = 8 bytes)
//   - O(1) operations, zero allocations
//   - Conflict case: Shared reads or multiple writers
//   - Promotes to VectorClock (1KB allocation for readClock)
//   - O(N) operations, tracks all threads
//
// Promotion triggers:
//   - OnRead: When second concurrent reader detected (readEpoch → readClock)
//   - OnWrite: Write operations demote back to Epoch (readClock → nil)
//
// SMARTTRACK OWNERSHIP TRACKING (v0.2.0 Task 3 optimization):
//   - Track exclusive writer TID to skip happens-before checks
//   - Common case (80%): Single writer pattern (owned)
//   - Fast path: No HB checks when reader == exclusive writer
//   - Shared case (20%): Multiple writers (exclusiveWriter = -1)
//   - Expected impact: 10-20% reduction in HB comparisons (PLDI 2020)
//
// Ownership states:
//   - exclusiveWriter >= 0: Single owner (fast path, skip HB checks)
//   - exclusiveWriter == -1: Shared/multiple writers (full FastTrack)
//   - exclusiveWriter == 0: Uninitialized (no writes yet)
//
// Memory layout (v0.3.0 Enhanced Read-Shared + Lock-Free):
//   - Base: 8 bytes (W atomic) + 8 bytes (mu) + 32 bytes (readEpochs[4]) + 1 byte (readerCount) + 8 bytes (readClock ptr) = 57 bytes
//   - SmartTrack: + 8 bytes (exclusiveWriter atomic) + 4 bytes (writeCount) = 69 bytes
//   - Stack Traces (Task 6): + 8 bytes (writeStackHash) + 8 bytes (readStackHash) = 85 bytes
//   - Lazy PC capture: + 8 bytes (writePC atomic) + 8 bytes (readPC atomic) = 101 bytes
//   - Total fast path: ~104 bytes per variable (with padding)
//   - Promoted path: 104 bytes + 1024 bytes (VectorClock allocation) = 1128 bytes
//
// v0.3.0 ENHANCED READ-SHARED OPTIMIZATION (P1):
// Trade-off: 88 bytes per variable (was 56 bytes) BUT avoids 1KB VectorClock allocation
// for the common case of 2-4 concurrent readers. Most read-heavy patterns stay in
// inline slots, saving 1KB per variable.
//
// Memory savings analysis:
//   - Old approach: 2+ readers → 56 + 1024 = 1080 bytes
//   - New approach: 2-4 readers → 88 bytes (no VectorClock)
//   - New approach: 5+ readers → 88 + 1024 = 1112 bytes
//   - Net savings for 2-4 readers: 992 bytes per variable!
type VarState struct {
	// Lock-free hot-path fields (atomic operations, no mutex needed):
	// These fields are accessed on EVERY memory access, so lock-free is critical.
	W               atomic.Uint64  // Last write epoch (always present). Stores epoch.Epoch as uint64.
	exclusiveWriter atomic.Int64   // TID of sole writer, -1 if shared, 0 if uninitialized.
	writePC         atomic.Uintptr // PC (program counter) of last write caller (8 bytes).
	readPC          atomic.Uintptr // PC (program counter) of last read caller (8 bytes).

	// Mutex-protected fields (complex operations):
	// These are accessed less frequently or require complex multi-field updates.
	mu sync.Mutex // Protects read fields, readClock, and write counters from concurrent access.

	// Read tracking (ENHANCED ADAPTIVE - v0.3.0 P1):
	// Inline slots for up to 4 concurrent readers before VectorClock promotion.
	// This delays expensive 1KB allocation for common patterns (2-4 readers).
	//
	// States:
	//   - readerCount == 0: No readers
	//   - readerCount == 1-4: Use readEpochs[0..readerCount-1]
	//   - readerCount == 255 (maxInlineReaders+1): Promoted to readClock
	//
	// If readClock != nil → use readClock (5+ readers, promoted state)
	readEpochs  [maxInlineReaders]epoch.Epoch // Inline reader slots (32 bytes = 4 × 8).
	readerCount uint8                         // Number of inline readers (0-4, or 255 if promoted).
	readClock   *vectorclock.VectorClock      // Multiple readers (promoted, 8 bytes pointer + 1KB allocation).

	// SmartTrack ownership tracking (v0.2.0 Task 3):
	// Tracks exclusive writer to skip expensive happens-before checks.
	writeCount uint32 // Number of writes (for statistics and debugging).

	// Stack Trace Storage (v0.2.0 Task 6):
	// Hash references to stack depot for previous write/read.
	// Enables complete race reports showing both current and previous stacks.
	writeStackHash uint64 // Hash of stack trace for last write (8 bytes).
	readStackHash  uint64 // Hash of stack trace for last read (8 bytes, only set when read-shared).
}

// NewVarState creates a new zero-initialized variable state.
//
// A zero VarState represents a variable that has never been accessed.
// Both W and readEpoch are zero (TID=0, Clock=0), readClock is nil.
//
//go:nosplit
func NewVarState() *VarState {
	return &VarState{}
}

// Reset resets the variable state to zero.
//
// This is used when a memory location is freed and reused.
// After Reset(), the state represents a fresh, never-accessed variable.
// If promoted, this demotes back to fast path (releases VectorClock to pool).
//
// SmartTrack (v0.2.0 Task 3): Also resets ownership tracking fields.
// Stack Traces (v0.2.0 Task 6): Also clears stack hashes.
// Enhanced Read-Shared (v0.3.0 P1): Also clears all inline reader slots.
// Lazy Stack Capture (v0.3.0 Performance): Also clears PC fields.
// Pooling: Returns VectorClock to pool for reuse if promoted.
//
// Performance: This operation must be zero-allocation and inline-friendly.
// Target: <5ns/op (was <2ns, increased due to clearing 4 slots).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) Reset() {
	// Reset lock-free fields using atomic stores.
	vs.W.Store(0)
	vs.exclusiveWriter.Store(0)
	vs.writePC.Store(0)
	vs.readPC.Store(0)

	// Reset mutex-protected fields.
	vs.mu.Lock()
	// Release VectorClock back to pool if promoted.
	if vs.readClock != nil {
		vs.readClock.Release()
	}
	// Clear all inline reader slots.
	for i := range vs.readEpochs {
		vs.readEpochs[i] = 0
	}
	vs.readerCount = 0
	vs.readClock = nil // Demote if promoted.
	vs.writeCount = 0
	vs.writeStackHash = 0
	vs.readStackHash = 0
	vs.mu.Unlock()
}

// IsPromoted returns true if VarState uses VectorClock (promoted state).
//
// This is the discriminator for the adaptive representation:
//   - false: Fast path (inline reader slots, up to 4 readers, 32 bytes)
//   - true: Slow path (readClock, 5+ readers, 1KB allocation)
//
// v0.3.0 Enhanced Read-Shared: Checks both readerCount == promotedMarker AND readClock != nil.
// This ensures consistency even if demotion clears one but not the other.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) IsPromoted() bool {
	vs.mu.Lock()
	promoted := vs.readerCount == promotedMarker && vs.readClock != nil
	vs.mu.Unlock()
	return promoted
}

// PromoteToReadClock upgrades from inline reader slots to multi-reader VectorClock.
//
// v0.3.0 Enhanced Read-Shared: This now happens when:
//   - All 4 inline reader slots are full
//   - A 5th concurrent reader is detected
//
// Steps:
//  1. Allocate VectorClock from pool (one-time cost, reused allocation)
//  2. Copy ALL inline reader epochs into VectorClock
//  3. Merge new read VectorClock
//  4. Set readerCount = promotedMarker (marks as promoted)
//
// After promotion, all subsequent reads use VectorClock path.
//
// Performance: This is a one-time cost (~100ns allocation + copy).
// Now even rarer than before (only 5+ concurrent readers trigger this).
// Uses pooled allocation to reduce GC pressure.
//
// Parameters:
//   - newReadVC: The VectorClock of the new concurrent reader
func (vs *VarState) PromoteToReadClock(newReadVC *vectorclock.VectorClock) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Allocate VectorClock from pool for promoted read tracking.
	vs.readClock = vectorclock.NewFromPool()

	// Copy ALL inline reader epochs into VectorClock.
	for i := uint8(0); i < vs.readerCount && i < maxInlineReaders; i++ {
		if vs.readEpochs[i] != 0 {
			tid, clock := vs.readEpochs[i].Decode()
			//nolint:gosec // G115: Epoch clock is uint64, but per-thread VectorClock uses uint32 (safe truncation).
			vs.readClock.Set(tid, uint32(clock))
		}
	}

	// Merge the new reader's VectorClock.
	vs.readClock.Join(newReadVC)

	// Clear inline slots and mark as promoted.
	for i := range vs.readEpochs {
		vs.readEpochs[i] = 0
	}
	vs.readerCount = promotedMarker
}

// GetReadEpoch returns the first read epoch (backward compatibility).
//
// v0.3.0 Enhanced Read-Shared: For single reader (common case), returns readEpochs[0].
// For multiple readers, use GetReadEpochs() to get all inline readers.
//
// PRECONDITION: !IsPromoted() - caller must check this first.
// If promoted, this returns 0 (invalid epoch).
//
// This is used by detector OnRead/OnWrite for fast-path checks.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetReadEpoch() epoch.Epoch {
	vs.mu.Lock()
	var e epoch.Epoch
	if vs.readerCount > 0 && vs.readerCount != promotedMarker {
		e = vs.readEpochs[0]
	}
	vs.mu.Unlock()
	return e
}

// GetReadEpochs returns all inline reader epochs (v0.3.0 Enhanced Read-Shared).
//
// PRECONDITION: !IsPromoted() - caller must check this first.
// If promoted, this returns nil.
//
// Returns a slice of all active reader epochs (0 to 4 elements).
// The caller should check all epochs for happens-before relationships.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetReadEpochs() []epoch.Epoch {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.readerCount == 0 || vs.readerCount == promotedMarker {
		return nil
	}

	// Return a copy of active reader epochs.
	count := int(vs.readerCount)
	if count > maxInlineReaders {
		count = maxInlineReaders
	}
	result := make([]epoch.Epoch, count)
	copy(result, vs.readEpochs[:count])
	return result
}

// GetReaderCount returns the number of inline readers (v0.3.0 Enhanced Read-Shared).
//
// Returns:
//   - 0: No readers
//   - 1-4: Number of inline readers
//   - 255 (promotedMarker): Promoted to VectorClock
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetReaderCount() uint8 {
	vs.mu.Lock()
	count := vs.readerCount
	vs.mu.Unlock()
	return count
}

// SetReadEpoch sets the read epoch (backward compatibility for single reader).
//
// v0.3.0 Enhanced Read-Shared: This sets readEpochs[0] for single reader case.
// For adding concurrent readers, use AddReader().
//
// This is used by detector OnRead to update single-reader state.
// If already promoted, this is a no-op (readClock takes precedence).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) SetReadEpoch(e epoch.Epoch) {
	vs.mu.Lock()
	if vs.readerCount != promotedMarker { // Not promoted
		vs.readEpochs[0] = e
		if vs.readerCount == 0 {
			vs.readerCount = 1
		}
	}
	vs.mu.Unlock()
}

// AddReader adds or updates a reader in the inline slots (v0.3.0 Enhanced Read-Shared).
//
// Logic:
//  1. If TID already exists in slots → update that slot's epoch
//  2. If there's room (readerCount < 4) → add to next slot
//  3. If slots are full → return false (caller should promote)
//
// Returns:
//   - true: Reader added/updated successfully
//   - false: Slots are full, promotion to VectorClock needed
//
// If already promoted, this is a no-op and returns true.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) AddReader(e epoch.Epoch) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Already promoted - let VectorClock handle it.
	if vs.readerCount == promotedMarker {
		return true
	}

	tid, _ := e.Decode()

	// Check if TID already exists in slots.
	for i := uint8(0); i < vs.readerCount; i++ {
		existingTID, _ := vs.readEpochs[i].Decode()
		if existingTID == tid {
			// Update existing slot.
			vs.readEpochs[i] = e
			return true
		}
	}

	// Check if there's room for new reader.
	if vs.readerCount < maxInlineReaders {
		vs.readEpochs[vs.readerCount] = e
		vs.readerCount++
		return true
	}

	// Slots are full - caller should promote.
	return false
}

// HasInlineSlot returns true if there's room for another inline reader.
//
// v0.3.0 Enhanced Read-Shared: Checks if readerCount < maxInlineReaders.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) HasInlineSlot() bool {
	vs.mu.Lock()
	hasSlot := vs.readerCount < maxInlineReaders && vs.readerCount != promotedMarker
	vs.mu.Unlock()
	return hasSlot
}

// GetReadClock returns the read VectorClock (slow path only).
//
// PRECONDITION: IsPromoted() - caller must check this first.
// If not promoted, this returns nil.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetReadClock() *vectorclock.VectorClock {
	vs.mu.Lock()
	rc := vs.readClock
	vs.mu.Unlock()
	return rc
}

// Demote clears all read state and demotes back to fast path.
//
// v0.3.0 Enhanced Read-Shared: Clears both inline slots AND VectorClock.
//
// This is called by OnWrite after a write operation to reset read tracking.
// Write dominates all previous reads, so we can safely clear the read state.
//
// This is a key optimization: variables with alternating read/write stay in fast path.
//
// Pooling: Returns VectorClock to pool for reuse if promoted.
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) Demote() {
	vs.mu.Lock()
	// Release VectorClock back to pool if promoted.
	if vs.readClock != nil {
		vs.readClock.Release()
	}
	// Clear all inline reader slots.
	for i := range vs.readEpochs {
		vs.readEpochs[i] = 0
	}
	vs.readerCount = 0
	vs.readClock = nil
	vs.mu.Unlock()
}

// === Lock-Free Accessor Methods (v0.3.0 Lock-Free Optimization) ===

// GetW returns the last write epoch using atomic load (lock-free).
//
// This is the HOT PATH method for checking write epochs during race detection.
// Using atomic load instead of mutex reduces overhead from ~20-50ns to ~2-5ns.
//
// Thread Safety: Lock-free (atomic load).
// Performance: ~2-5ns (atomic load).
//
//go:nosplit
func (vs *VarState) GetW() epoch.Epoch {
	return epoch.Epoch(vs.W.Load())
}

// SetW sets the last write epoch using atomic store (lock-free).
//
// This is the HOT PATH method for updating write epochs during race detection.
// Using atomic store instead of mutex reduces overhead from ~20-50ns to ~2-5ns.
//
// Parameters:
//   - e: The epoch to store
//
// Thread Safety: Lock-free (atomic store).
// Performance: ~2-5ns (atomic store).
//
//go:nosplit
func (vs *VarState) SetW(e epoch.Epoch) {
	vs.W.Store(uint64(e))
}

// CompareAndSwapW atomically compares and swaps the write epoch (lock-free).
//
// This is used for atomic write epoch updates when racing with other writers.
//
// Parameters:
//   - oldVal: Expected current value
//   - newVal: New value to set
//
// Returns:
//   - true if swap succeeded (current value was 'oldVal')
//   - false if swap failed (current value was not 'oldVal')
//
// Thread Safety: Lock-free (atomic CAS).
// Performance: ~5-10ns (atomic CAS).
//
//go:nosplit
func (vs *VarState) CompareAndSwapW(oldVal, newVal epoch.Epoch) bool {
	return vs.W.CompareAndSwap(uint64(oldVal), uint64(newVal))
}

// === SmartTrack Ownership Tracking Methods (v0.2.0 Task 3) ===

// IsOwned returns true if the variable has an exclusive writer (owned state).
//
// Ownership states:
//   - exclusiveWriter >= 0: Single owner (fast path, skip HB checks)
//   - exclusiveWriter == -1: Shared/multiple writers (full FastTrack)
//   - exclusiveWriter == 0: Uninitialized (no writes yet)
//
// This is used by the detector to decide whether to skip happens-before checks.
//
// Thread Safety: Lock-free (atomic load).
// Performance: ~2-5ns (atomic load).
//
//go:nosplit
func (vs *VarState) IsOwned() bool {
	return vs.exclusiveWriter.Load() >= 0
}

// GetExclusiveWriter returns the TID of the exclusive writer, or -1 if shared.
//
// Returns:
//   - TID >= 0: Single exclusive writer (owned)
//   - -1: Shared/multiple writers
//   - 0: Uninitialized (no writes yet)
//
// Thread Safety: Lock-free (atomic load).
// Performance: ~2-5ns (atomic load).
//
//go:nosplit
func (vs *VarState) GetExclusiveWriter() int64 {
	return vs.exclusiveWriter.Load()
}

// SetExclusiveWriter sets the exclusive writer TID.
//
// This is called when:
//   - First write: Claim ownership (tid >= 0)
//   - Second writer detected: Promote to shared (tid = -1)
//
// Thread Safety: Lock-free (atomic store).
// Performance: ~2-5ns (atomic store).
//
//go:nosplit
func (vs *VarState) SetExclusiveWriter(tid int64) {
	vs.exclusiveWriter.Store(tid)
}

// CompareAndSwapExclusiveWriter atomically compares and swaps the exclusive writer.
//
// This is used to atomically claim ownership when first writing to a variable.
// It solves the TOCTOU race condition where two goroutines both see exclusiveWriter=0
// and both think they're the first writer.
//
// Parameters:
//   - oldVal: Expected current value (typically 0 for first writer claim)
//   - newVal: New value to set (current goroutine's TID)
//
// Returns:
//   - true if swap succeeded (current value was 'oldVal')
//   - false if swap failed (current value was not 'oldVal')
//
// Thread Safety: Lock-free (atomic CAS).
// Performance: ~5-10ns (atomic CAS).
//
//go:nosplit
func (vs *VarState) CompareAndSwapExclusiveWriter(oldVal, newVal int64) bool {
	return vs.exclusiveWriter.CompareAndSwap(oldVal, newVal)
}

// IncrementWriteCount increments the write counter.
//
// This is called on every write to track total write operations.
// Used for statistics and debugging.
//
// Thread Safety: Protected by mutex.
// Performance: <5ns/op (mutex + field increment).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) IncrementWriteCount() {
	vs.mu.Lock()
	vs.writeCount++
	vs.mu.Unlock()
}

// GetWriteCount returns the total number of writes to this variable.
//
// Thread Safety: Protected by mutex.
// Performance: <5ns/op (mutex + field read).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetWriteCount() uint32 {
	vs.mu.Lock()
	count := vs.writeCount
	vs.mu.Unlock()
	return count
}

// String returns a debug representation of the variable state.
//
// v0.3.0 Enhanced Read-Shared: Shows all inline reader slots.
//
// Format:
//   - No readers: "W:<epoch> R:[]"
//   - Single reader: "W:<epoch> R:[50@3]"
//   - Multi reader: "W:<epoch> R:[50@3, 60@5, 70@7]"
//   - Promoted: "W:<epoch> R:<vectorclock> [PROMOTED]"
//
// Example:
//   - "W:100@5 R:[50@3]" (single reader, fast path)
//   - "W:100@5 R:[50@3, 60@5]" (2 readers, inline slots)
//   - "W:100@5 R:{0:50, 1:60} [PROMOTED]" (5+ readers, promoted)
//
// This method is only used for debugging and race reporting, not on hot path.
func (vs *VarState) String() string {
	// Note: We manually build the string to avoid fmt import overhead.
	wStr := "W:" + vs.GetW().String()

	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.readerCount == promotedMarker && vs.readClock != nil {
		// Promoted: Show VectorClock.
		return wStr + " R:" + vs.readClock.String() + " [PROMOTED]"
	}

	// Inline slots: Show all active reader epochs.
	rStr := "R:["
	for i := uint8(0); i < vs.readerCount; i++ {
		if i > 0 {
			rStr += ", "
		}
		rStr += vs.readEpochs[i].String()
	}
	rStr += "]"
	return wStr + " " + rStr
}

// === Stack Trace Accessor Methods (v0.2.0 Task 6) ===

// SetWriteStack records the stack trace hash for a write access.
//
// This is called by the detector on every write to store the stack trace
// for later retrieval during race reporting.
//
// Parameters:
//   - stackHash: Hash returned by stackdepot.CaptureStack()
//
// Thread Safety: Uses atomic store for lock-free access.
// Performance: ~2ns (atomic store).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) SetWriteStack(stackHash uint64) {
	vs.mu.Lock()
	vs.writeStackHash = stackHash
	vs.mu.Unlock()
}

// GetWriteStack retrieves the stack trace hash for the last write.
//
// This is called during race reporting to retrieve the previous write stack.
//
// Returns:
//   - uint64: Hash that can be passed to stackdepot.GetStack()
//   - 0: If no write stack has been captured
//
// Thread Safety: Uses atomic load for lock-free access.
// Performance: ~2ns (atomic load).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetWriteStack() uint64 {
	vs.mu.Lock()
	hash := vs.writeStackHash
	vs.mu.Unlock()
	return hash
}

// SetReadStack records the stack trace hash for a read access (read-shared case).
//
// This is called by the detector on read to promoted (read-shared) variables.
// For unpromoted variables, read stack is not stored (not needed for races).
//
// Parameters:
//   - stackHash: Hash returned by stackdepot.CaptureStack()
//
// Thread Safety: Protected by mutex (consistent with other read state).
// Performance: ~5ns (mutex + field write).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) SetReadStack(stackHash uint64) {
	vs.mu.Lock()
	vs.readStackHash = stackHash
	vs.mu.Unlock()
}

// GetReadStack retrieves the stack trace hash for the last read.
//
// This is called during race reporting to retrieve the previous read stack.
// Only meaningful for promoted (read-shared) variables.
//
// Returns:
//   - uint64: Hash that can be passed to stackdepot.GetStack()
//   - 0: If no read stack has been captured
//
// Thread Safety: Protected by mutex (consistent with other read state).
// Performance: ~5ns (mutex + field read).
//
// Note: Removed //go:nosplit because sync.Mutex.Lock() requires stack space.
func (vs *VarState) GetReadStack() uint64 {
	vs.mu.Lock()
	hash := vs.readStackHash
	vs.mu.Unlock()
	return hash
}

// === Lazy Stack Capture Methods (v0.3.0 Performance) ===

// SetWritePC stores the program counter (PC) of the write caller.
//
// This is the HOT PATH optimization: instead of capturing full stack (~500ns),
// we store only the caller PC (~5ns). Full stack is captured lazily when race detected.
//
// Parameters:
//   - pc: Program counter from runtime.Callers(2, pcs[:1])
//
// Thread Safety: Lock-free (atomic store).
// Performance: ~2-5ns (atomic store).
//
//go:nosplit
func (vs *VarState) SetWritePC(pc uintptr) {
	vs.writePC.Store(pc)
}

// GetWritePC retrieves the program counter of the last write.
//
// This is called during race reporting to capture full stack lazily.
//
// Returns:
//   - uintptr: Program counter of last write caller
//   - 0: If no write PC has been captured
//
// Thread Safety: Lock-free (atomic load).
// Performance: ~2-5ns (atomic load).
//
//go:nosplit
func (vs *VarState) GetWritePC() uintptr {
	return vs.writePC.Load()
}

// SetReadPC stores the program counter (PC) of the read caller.
//
// This is the HOT PATH optimization for reads on read-shared variables.
//
// Parameters:
//   - pc: Program counter from runtime.Callers(2, pcs[:1])
//
// Thread Safety: Lock-free (atomic store).
// Performance: ~2-5ns (atomic store).
//
//go:nosplit
func (vs *VarState) SetReadPC(pc uintptr) {
	vs.readPC.Store(pc)
}

// GetReadPC retrieves the program counter of the last read.
//
// This is called during race reporting to capture full stack lazily.
//
// Returns:
//   - uintptr: Program counter of last read caller
//   - 0: If no read PC has been captured
//
// Thread Safety: Lock-free (atomic load).
// Performance: ~2-5ns (atomic load).
//
//go:nosplit
func (vs *VarState) GetReadPC() uintptr {
	return vs.readPC.Load()
}
