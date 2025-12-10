// Package vectorclock implements vector clocks for tracking happens-before relations.
//
// Vector clocks are used in FastTrack algorithm for read-shared data (0.1% of accesses).
// Most operations (99%+) use lightweight epochs, but when concurrent reads occur,
// we promote to vector clocks to precisely track partial order across all threads.
//
// Key operations:
//   - Join: Synchronization (point-wise maximum) - used on lock acquire
//   - LessOrEqual: Happens-before check (partial order) - used for race detection
//
// Performance targets: Join < 500ns, LessOrEqual < 300ns, zero allocations.
package vectorclock

import (
	"strings"
	"sync"
)

const (
	// MaxThreads is the maximum number of concurrent threads supported.
	// This is a fixed-size array for zero-allocation operation.
	//
	// 65,536 threads = 16-bit TID space (sufficient for 99%+ of real programs).
	// Memory: 65,536 × 4 bytes = 262,144 bytes = 256KB per VectorClock.
	//
	// Trade-off:
	//   - Pro: Supports up to 65K concurrent goroutines (vs 256 in MVP).
	//   - Con: 256KB per VectorClock (vs 1KB in MVP) - 256x memory increase.
	//   - Mitigation: VectorClock only allocated for read-shared variables (rare).
	//     FastTrack's adaptive algorithm keeps most variables in Epoch mode (8 bytes).
	//
	// v0.4 will add dynamic TID mapping for unlimited goroutines with compact storage.
	MaxThreads = 65536
)

// vcPool is a sync.Pool for VectorClock reuse to reduce GC pressure.
//
// VectorClock is 256KB per allocation, so reusing them is critical for performance.
// The pool is used by NewFromPool() and Release() for lifecycle management.
//
// Performance impact:
//   - Reduces allocations for VectorClock-heavy workloads (read-shared patterns)
//   - Lower GC pressure and latency
//   - Expected 5-10% overall improvement in read-heavy scenarios
//
// Usage:
//   - NewFromPool(): Get VectorClock from pool (or allocate new if pool empty)
//   - Release(): Return VectorClock to pool for reuse
//   - New(): Still available for non-pooled allocations (backward compatibility)
var vcPool = sync.Pool{
	New: func() interface{} {
		return &VectorClock{}
	},
}

// VectorClock represents logical time across multiple threads.
//
// Each element vc[tid] stores the clock value for thread tid.
// This is a fixed-size array (not a slice) to avoid heap allocations.
//
// Layout: [Thread0, Thread1, ..., Thread65535]
// Example: {0: 50, 1: 30, 2: 60, ...} means Thread0@50, Thread1@30, Thread2@60.
//
// Size: 65,536 × 4 bytes = 256KB (large, but only allocated for read-shared variables).
//
// v0.3.0 SPARSE-AWARE OPTIMIZATION (P1):
// Track maxTID to avoid iterating over 65536 elements when most are zero.
// Typical programs have ~10-100 active goroutines, so this reduces iteration
// from 65536 to ~100 elements (655x speedup for Join/LessOrEqual).
type VectorClock struct {
	clocks [MaxThreads]uint32 // Clock values per thread.
	maxTID uint16             // Highest TID with non-zero clock (sparse optimization).
}

// New creates a zero-initialized vector clock.
//
// All thread clocks start at 0, representing the beginning of logical time.
// Returns a pointer to avoid copying the 1KB array on return.
//
// NOTE: For pooled allocations, use NewFromPool() instead to reduce GC pressure.
// This function is kept for backward compatibility and non-pooled use cases.
func New() *VectorClock {
	return &VectorClock{}
}

// NewFromPool gets a VectorClock from the pool or creates a new one.
//
// This is the RECOMMENDED way to allocate VectorClocks in hot paths.
// The returned VectorClock is guaranteed to be in reset state (all zeros).
//
// VectorClock lifecycle with pooling:
//  1. Acquire: vc := NewFromPool()
//  2. Use: vc.Set(), vc.Join(), vc.LessOrEqual(), etc.
//  3. Release: vc.Release() when no longer needed
//
// Performance:
//   - Reuses 256KB allocations instead of creating new ones
//   - Reduces GC pressure significantly in read-heavy workloads
//   - Expected 5-10% overall improvement
//
// Thread Safety: sync.Pool is goroutine-safe.
//
// Example:
//
//	vc := vectorclock.NewFromPool()
//	defer vc.Release()  // Return to pool when done
//	vc.Set(tid, clock)
//	// ... use vc ...
func NewFromPool() *VectorClock {
	vc := vcPool.Get().(*VectorClock)
	vc.Reset() // Ensure clean state
	return vc
}

// Release returns the VectorClock to the pool for reuse.
//
// This should be called when the VectorClock is no longer needed.
// After Release(), the VectorClock must not be used again.
//
// CRITICAL: Only release VectorClocks that are NOT referenced elsewhere!
// Common mistake: Releasing a VectorClock that is still stored in VarState.
//
// Safe to call on nil VectorClock (no-op).
//
// Thread Safety: sync.Pool is goroutine-safe.
//
// Example:
//
//	vc := vectorclock.NewFromPool()
//	// ... use vc ...
//	vc.Release()  // Return to pool
//	// vc must NOT be used after this point!
func (vc *VectorClock) Release() {
	if vc != nil {
		vcPool.Put(vc)
	}
}

// Reset clears the VectorClock to zero state.
//
// This is called by NewFromPool() to ensure clean state when reusing from pool.
// Can also be called manually to reset a VectorClock for reuse.
//
// Only clears up to maxTID+1 elements for efficiency (sparse optimization).
// Sets maxTID to 0 to represent empty clock.
//
// Performance: Target <100ns for typical sparse clocks (~100 active threads).
//
// Thread Safety: Not thread-safe, caller must synchronize.
func (vc *VectorClock) Reset() {
	// Clear all clock values up to maxTID (sparse-aware).
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(vc.maxTID); i++ {
		vc.clocks[i] = 0
	}
	vc.maxTID = 0
}

// Clone creates a deep copy of the vector clock.
//
// This is used when we need to preserve a snapshot of logical time,
// for example when storing a vector clock in shadow memory.
//
// v0.3.0: Only copies up to maxTID for efficiency.
//
// Returns a pointer to the new copy to avoid copying on return.
func (vc *VectorClock) Clone() *VectorClock {
	clone := &VectorClock{maxTID: vc.maxTID}
	// Only copy up to maxTID+1 elements for efficiency.
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(vc.maxTID); i++ {
		clone.clocks[i] = vc.clocks[i]
	}
	return clone
}

// Join performs point-wise maximum: vc = vc ⊔ other.
//
// This is the synchronization operation for happens-before in FastTrack.
// Used when a thread acquires a lock: Ct := Ct ⊔ Lm (thread clock joins lock clock).
//
// Algorithm: For each thread i, vc[i] = max(vc[i], other[i])
//
// v0.3.0 SPARSE-AWARE: Only iterates up to max(vc.maxTID, other.maxTID).
// For typical programs (~100 goroutines), this is 655x faster than iterating 65536 elements.
//
// Performance: Critical operation, must be fast. Target: < 10ns for sparse clocks.
//
//go:nosplit
func (vc *VectorClock) Join(other *VectorClock) {
	// Determine the range to iterate (sparse optimization).
	limit := uint32(vc.maxTID)
	if uint32(other.maxTID) > limit {
		limit = uint32(other.maxTID)
	}

	// Point-wise maximum only up to limit.
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= limit; i++ {
		if other.clocks[i] > vc.clocks[i] {
			vc.clocks[i] = other.clocks[i]
		}
	}

	// Update maxTID if other had higher TIDs.
	if other.maxTID > vc.maxTID {
		vc.maxTID = other.maxTID
	}
}

// LessOrEqual checks partial order: vc ⊑ other.
//
// Returns true if vc[i] <= other[i] for all threads i.
// This implements the happens-before relation check.
//
// Used in FastTrack to check if a write epoch happens-before a read:
// If write's VC ⊑ read's VC, then no race (write happened-before read).
//
// v0.3.0 SPARSE-AWARE: Only checks up to vc.maxTID (elements beyond are zero).
// For typical programs (~100 goroutines), this is 655x faster.
//
// Performance: Critical operation on race check path. Target: < 5ns for sparse clocks.
//
//go:nosplit
func (vc *VectorClock) LessOrEqual(other *VectorClock) bool {
	// Only need to check up to vc.maxTID (elements beyond are 0, which is always <= other[i]).
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(vc.maxTID); i++ {
		if vc.clocks[i] > other.clocks[i] {
			return false
		}
	}
	return true
}

// HappensBefore checks if this VectorClock happened-before another VectorClock.
//
// This is an alias for LessOrEqual for better API clarity.
// Returns true if vc ⊑ other (all elements vc[i] <= other[i]).
//
// Used in adaptive VarState to check if read VectorClock happened-before write VectorClock.
//
// Performance: Same as LessOrEqual, < 300ns, 0 allocs.
//
//go:nosplit
func (vc *VectorClock) HappensBefore(other *VectorClock) bool {
	return vc.LessOrEqual(other)
}

// Increment advances the clock for thread tid.
//
// This is called on every memory access by thread tid.
// Increments vc[tid] to represent forward progress in logical time.
//
// v0.3.0: Updates maxTID if needed.
func (vc *VectorClock) Increment(tid uint16) {
	vc.clocks[tid]++
	if tid > vc.maxTID {
		vc.maxTID = tid
	}
}

// Get returns the clock value for thread tid.
//
// Used to read a specific thread's logical time from the vector clock.
func (vc *VectorClock) Get(tid uint16) uint32 {
	return vc.clocks[tid]
}

// Set sets the clock value for thread tid.
//
// Used to update a specific thread's logical time in the vector clock.
// Typically used during initialization or synchronization operations.
//
// v0.3.0: Updates maxTID if clock > 0 and tid > maxTID.
func (vc *VectorClock) Set(tid uint16, clock uint32) {
	vc.clocks[tid] = clock
	if clock > 0 && tid > vc.maxTID {
		vc.maxTID = tid
	}
}

// GetMaxTID returns the highest TID with non-zero clock (v0.3.0 sparse optimization).
//
// This is useful for debugging and understanding the sparsity of the vector clock.
func (vc *VectorClock) GetMaxTID() uint16 {
	return vc.maxTID
}

// CopyFrom copies all values from another VectorClock into this one.
//
// v0.3.0: Uses sparse-aware copying for efficiency.
// This is used for in-place updates to avoid allocations.
func (vc *VectorClock) CopyFrom(other *VectorClock) {
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(other.maxTID); i++ {
		vc.clocks[i] = other.clocks[i]
	}
	// Clear elements beyond other.maxTID if vc had higher maxTID.
	if vc.maxTID > other.maxTID {
		for i := uint32(other.maxTID) + 1; i <= uint32(vc.maxTID); i++ {
			vc.clocks[i] = 0
		}
	}
	vc.maxTID = other.maxTID
}

// String returns a debug representation of the vector clock.
//
// Format: "{tid1:clock1, tid2:clock2, ...}" showing only non-zero clocks.
// This is used for debugging and race reporting, not on hot path.
//
// v0.3.0: Only iterates up to maxTID for efficiency.
//
// Example: "{0:50, 1:30, 5:42}" means Thread0=50, Thread1=30, Thread5=42.
func (vc *VectorClock) String() string {
	var parts []string
	// v0.3.0: Only iterate up to maxTID (sparse optimization).
	// Use uint32 loop counter to avoid uint16 overflow at maxTID=65535.
	for i := uint32(0); i <= uint32(vc.maxTID); i++ {
		if vc.clocks[i] != 0 {
			parts = append(parts, itoa(i)+":"+itoa(vc.clocks[i]))
		}
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// itoa converts an integer to string without fmt import.
// Simple implementation for debugging output only.
// This avoids importing fmt which can cause circular dependencies in runtime code.
func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}

	// Calculate number of digits.
	tmp := n
	digits := 0
	for tmp > 0 {
		digits++
		tmp /= 10
	}

	// Build string from right to left.
	buf := make([]byte, digits)
	for i := digits - 1; i >= 0; i-- {
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	return string(buf)
}
