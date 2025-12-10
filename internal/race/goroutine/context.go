package goroutine

import (
	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

// RaceContext represents the race detection state for a single goroutine.
//
// Each goroutine has its own RaceContext tracking logical time and happens-before
// relationships. The context maintains both a full vector clock (C) and a cached
// epoch (Epoch) for the current thread.
//
// The epoch cache enables FastTrack's critical optimization: most operations
// (96%+) only need the epoch value, avoiding expensive vector clock operations.
//
// Layout:
//   - TID: Thread/Goroutine ID (0-65535, uint16)
//   - C: Full vector clock [65536]uint32 tracking all threads
//   - Epoch: Cached value of C[TID] as compact 64-bit epoch
//
// Invariant: Epoch must ALWAYS equal epoch.NewEpoch(TID, C[TID]).
// This invariant is maintained by IncrementClock() which atomically updates both.
type RaceContext struct {
	// TID is the thread/goroutine identifier (0-65535).
	// 16-bit uint for production (65,536 concurrent goroutines max).
	TID uint16

	// C is the full vector clock tracking logical time for all threads.
	// C[i] represents the logical time for thread i.
	// This is used for happens-before checks when epoch fast-path fails.
	C *vectorclock.VectorClock

	// Epoch is the cached epoch for this goroutine: Epoch == C[TID].
	// This enables O(1) access to the current logical time without
	// accessing the full vector clock array.
	//
	// CRITICAL: This field is on the hot path for every memory access!
	// Must be kept in sync with C[TID] at all times.
	Epoch epoch.Epoch
}

// Alloc creates and initializes a new RaceContext for the given thread ID.
//
// The context is initialized with:
//   - TID set to the provided tid
//   - C initialized with C[tid]=1 (this thread at time 1, others at 0)
//   - Epoch set to epoch.NewEpoch(tid, 1) (TID@1)
//
// This represents a newly started goroutine at the beginning of logical time.
//
// IMPORTANT: Clock starts at 1, not 0. This is critical for race detection:
// - Clock 0 means "never happened" (default in VectorClock)
// - Two accesses at clock 0 would appear to "happen-before" each other
// - Starting at 1 ensures unsynchronized accesses are detected as races
//
// Pooling: Uses pooled VectorClock allocation to reduce GC pressure.
// VectorClock is released back to pool when goroutine ends (racegoend).
//
// Example:
//
//	ctx := Alloc(5)
//	// ctx.TID = 5
//	// ctx.C = {5:1, others:0}
//	// ctx.Epoch = 1@5 (clock=1, tid=5)
func Alloc(tid uint16) *RaceContext {
	ctx := &RaceContext{
		TID: tid,
		C:   vectorclock.NewFromPool(),
	}
	// Initialize epoch cache to TID@1 (clock 1 for new goroutine).
	// CRITICAL: Clock must start at 1, not 0, to detect unsynchronized races.
	// Clock 0 means "never happened" in HappensBefore check (0 <= 0 is TRUE).
	ctx.C.Set(tid, 1) // Set initial clock in VectorClock
	ctx.Epoch = epoch.NewEpoch(tid, 1)
	return ctx
}

// IncrementClock advances the logical clock for this goroutine.
//
// This is called on every memory access by this goroutine to represent
// forward progress in logical time. It performs two atomic updates:
//  1. Increments C[TID] in the vector clock
//  2. Updates the cached Epoch to reflect the new C[TID] value
//
// The updates are performed sequentially to maintain the invariant:
//
//	Epoch == epoch.NewEpoch(TID, C[TID])
//
// Performance: Target <200ns/op (VectorClock.Increment + Epoch creation).
// This is on the hot path - called millions of times during execution.
//
// Example:
//
//	ctx := Alloc(5)
//	// ctx.C[5] = 1, ctx.Epoch = 1@5
//	ctx.IncrementClock()
//	// ctx.C[5] = 2, ctx.Epoch = 2@5
//	ctx.IncrementClock()
//	// ctx.C[5] = 3, ctx.Epoch = 3@5
func (rc *RaceContext) IncrementClock() {
	// Step 1: Increment the vector clock for this thread.
	rc.C.Increment(rc.TID)

	// Step 2: Update the cached epoch to match C[TID].
	// This maintains the invariant: Epoch == epoch.NewEpoch(TID, C[TID]).
	rc.Epoch = epoch.NewEpoch(rc.TID, uint64(rc.C.Get(rc.TID)))
}

// GetEpoch returns the cached epoch for this goroutine.
//
// This is the CRITICAL HOT PATH operation - called on every memory access
// for race detection. It must be:
//   - O(1): Just a field access, no computation
//   - Zero allocations
//   - Inline-candidate (no function call overhead)
//   - //go:nosplit to prevent stack growth
//
// The cached epoch represents the current logical time for this goroutine
// as a compact 32-bit value (TID in top 8 bits, clock in bottom 24 bits).
//
// Performance: Target <1ns/op (single field read).
//
// Example:
//
//	ctx := Alloc(5)
//	e := ctx.GetEpoch()  // Returns 1@5 (clock=1, tid=5)
//	ctx.IncrementClock()
//	e = ctx.GetEpoch()   // Returns 2@5 (clock=2, tid=5)
//
//go:nosplit
func (rc *RaceContext) GetEpoch() epoch.Epoch {
	return rc.Epoch
}

// AllocWithParentClock creates a RaceContext that inherits parent's clock.
//
// This is the key function for happens-before at goroutine creation (fork):
//  1. child.C := parent.C (Copy parent's clock - inherit HB relations)
//  2. child.C[child.TID] = 1 (Initialize child's own component)
//  3. child.Epoch = NewEpoch(tid, 1)
//
// After this, any operation in child "sees" all operations that happened
// in parent before the fork (go func() statement).
//
// Pooling: Uses pooled VectorClock allocation to reduce GC pressure.
// VectorClock is released back to pool when goroutine ends (racegoend).
//
// Parameters:
//   - tid: Thread ID allocated for this child goroutine
//   - parentClock: Snapshot of parent's VectorClock at fork time
//
// Returns:
//   - *RaceContext: Context ready for race detection with inherited HB
//
// Example:
//
//	Parent at fork: clock={1:5, 3:2}
//	Child after AllocWithParentClock(2, parentClock):
//	  clock={1:5, 2:1, 3:2}
//	        ↑ inherited from parent
//	             ↑ child's own component initialized to 1
//	                  ↑ inherited from parent
func AllocWithParentClock(tid uint16, parentClock *vectorclock.VectorClock) *RaceContext {
	ctx := &RaceContext{
		TID: tid,
		C:   vectorclock.NewFromPool(),
	}

	// Step 1: Inherit parent's clock (HB edge: parent fork -> child start).
	// This copies all components from parent's clock to child's clock.
	if parentClock != nil {
		ctx.C.CopyFrom(parentClock)
	}

	// Step 2: Initialize child's own clock component.
	// CRITICAL: Must start at 1, not 0, to detect unsynchronized races.
	// Clock 0 means "never happened" in HappensBefore check (0 <= 0 is TRUE).
	ctx.C.Set(tid, 1)

	// Step 3: Initialize cached epoch.
	ctx.Epoch = epoch.NewEpoch(tid, 1)

	return ctx
}
