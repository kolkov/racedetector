// Package api provides the public runtime API for the Pure-Go Race Detector.
//
// This package implements the entry points called by Go compiler instrumentation
// when code is built with -race flag. These functions are invoked on every memory
// access in instrumented code, making them CRITICAL HOT PATHS.
//
// The API follows the same interface contract as Go's runtime.race* functions,
// ensuring compatibility with existing compiler instrumentation.
//
// Performance Targets (MVP - Phase 1):
//   - raceread:  < 30ns per call (includes OnRead ~21ns)
//   - racewrite: < 25ns per call (includes OnWrite ~17ns)
//   - getCurrentContext (cached): < 5ns
//   - getCurrentContext (first): < 100ns
//
// MVP Simplifications:
//   - Goroutine ID extracted via runtime.Stack() parsing (SLOW - ~500ns)
//   - PC tracking collected but not used in reporting yet
//   - TID allocation is simple atomic counter (no reuse)
//   - No GoEnd() hook - contexts never freed
//
// Phase 2 Improvements (Future):
//   - Replace getGoroutineID() with assembly getg() stub (~1ns)
//   - Implement TID reuse pool
//   - Add GoEnd() cleanup
//   - Enable PC-based stack traces in reports
package api

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/kolkov/racedetector/internal/race/detector"
	"github.com/kolkov/racedetector/internal/race/goroutine"
)

// Global detector state.
//
// These variables are initialized once during init() and remain constant
// for the lifetime of the program. The detector itself is thread-safe.
var (
	// enabled controls whether race detection is active.
	// For MVP, this is always true. In Phase 7 (Production), this will
	// be configurable via environment variables (GORACE=...).
	enabled atomic.Bool

	// contexts maps goroutine IDs to their RaceContext instances.
	// Using sync.Map for lock-free concurrent access patterns:
	//   - Most goroutines created once and accessed frequently (cached reads)
	//   - Rare writes when new goroutines spawn
	// Key: int64 (goroutine ID)
	// Value: *goroutine.RaceContext.
	contexts sync.Map

	// nextTID is the atomic counter for allocating thread IDs.
	// Phase 2 Task 2.2: Used for statistics and cleanup trigger.
	// No longer wraps at 256 - TID pool handles reuse.
	nextTID atomic.Uint32

	// det is the global detector instance.
	// All race detection flows through this single instance.
	det *detector.Detector

	// === TID Pool Management (Phase 2 Task 2.2) ===
	// TID reuse pool supporting unlimited goroutines (1000+).

	// freeTIDs is a stack of available TIDs (0-255).
	// Protected by tidPoolMu. TIDs are popped on allocation and pushed on free.
	freeTIDs []uint16

	// tidPoolMu protects freeTIDs stack.
	// Lock contention is minimal as allocations are rare relative to raceread/racewrite.
	tidPoolMu sync.Mutex

	// tidToGID maps TID back to GID for cleanup verification.
	// Key: uint8 (TID), Value: int64 (GID).
	// Used during cleanup to identify stale contexts.
	tidToGID sync.Map

	// allocCounter counts context allocations to trigger periodic cleanup.
	// Every 1000 allocations, we scan for dead goroutines and reclaim TIDs.
	allocCounter atomic.Uint32
)

// init initializes the global race detector.
//
// This runs automatically before main() starts. It sets up:
//   - The global detector instance
//   - The enabled flag (true for MVP)
//   - The TID counter (starts at 0)
//
// The detector is ready to use immediately after init().
func init() {
	det = detector.NewDetector()
	enabled.Store(true)
	// nextTID starts at 0 (first goroutine gets TID 0).
}

// raceread is called by compiler instrumentation on every read access.
//
// This is the CRITICAL HOT PATH for read operations. It will be invoked
// millions of times during program execution, so performance is paramount.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Extract program counter (PC) of the access (for future reporting)
//  4. Call detector.OnRead() to check for races
//
// Parameters:
//   - addr: Memory address being read from
//
// Performance: Target <30ns per call (MVP).
//
// Zero Allocations: This function must not allocate on heap after context
// is cached. First call per goroutine may allocate when creating context.
//
// Example (compiler-generated):
//
//	x := *ptr  // Becomes: runtime.raceread(uintptr(unsafe.Pointer(ptr))); x = *ptr
//
//go:nosplit
func raceread(addr uintptr) {
	// Fast path: Check if race detection is enabled.
	// This allows disabling the detector at runtime with minimal overhead.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	// This allocates on first call per goroutine (~100ns), then cached (~5ns).
	ctx := getCurrentContext()

	// Extract program counter for the access.
	// Currently collected but not used in reports (planned for v0.4.0).
	_ = getcallerpc() // TODO: Pass to OnRead for enhanced stack trace reporting

	// Perform race detection check.
	// This calls the FastTrack algorithm to detect read-write races.
	// Pass the RaceContext for this goroutine to enable proper per-goroutine tracking.
	det.OnRead(addr, ctx)
}

// racewrite is called by compiler instrumentation on every write access.
//
// This is the CRITICAL HOT PATH for write operations. Like raceread,
// it's called millions of times, so performance is critical.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Extract program counter (PC) of the access
//  4. Call detector.OnWrite() to check for races
//
// Parameters:
//   - addr: Memory address being written to
//
// Performance: Target <25ns per call (MVP).
//
// Zero Allocations: Must not allocate after context is cached.
//
// Example (compiler-generated):
//
//	*ptr = x  // Becomes: runtime.racewrite(uintptr(unsafe.Pointer(ptr))); *ptr = x
//
//go:nosplit
func racewrite(addr uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Extract program counter for the access.
	_ = getcallerpc() // TODO: Pass to OnWrite for enhanced stack trace reporting

	// Perform race detection check.
	// This calls the FastTrack algorithm to detect write-write and read-write races.
	// Pass the RaceContext for this goroutine to enable proper per-goroutine tracking.
	det.OnWrite(addr, ctx)
}

// raceacquire is called by compiler instrumentation on mutex lock operations (Phase 4 Task 4.1).
//
// This establishes a happens-before edge from the previous Unlock to this Lock.
// The acquiring thread merges the mutex's release clock into its own clock.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnAcquire() to establish happens-before
//
// Parameters:
//   - addr: Address of the sync.Mutex being locked
//
// Performance: Target <500ns per call (VectorClock join overhead acceptable).
//
// Zero Allocations: First call per goroutine may allocate context.
// VectorClock join operation is zero-allocation.
//
// Example (compiler-generated):
//
//	mu.Lock()  // Becomes: runtime.raceacquire(uintptr(unsafe.Pointer(&mu))); mu.Lock()
//
//go:nosplit
func raceacquire(addr uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform sync acquire tracking.
	// This establishes happens-before from previous Unlock.
	det.OnAcquire(addr, ctx)
}

// racerelease is called by compiler instrumentation on mutex unlock operations (Phase 4 Task 4.1).
//
// This creates a happens-before edge that future Lock operations will synchronize with.
// The releasing thread captures its current clock into the mutex's release clock.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnRelease() to capture current clock
//
// Parameters:
//   - addr: Address of the sync.Mutex being unlocked
//
// Performance: Target <300ns per call (VectorClock copy overhead acceptable).
//
// Zero Allocations: VectorClock is updated in place (no new allocations after first).
//
// Example (compiler-generated):
//
//	mu.Unlock()  // Becomes: runtime.racerelease(uintptr(unsafe.Pointer(&mu))); mu.Unlock()
//
//go:nosplit
func racerelease(addr uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform sync release tracking.
	// This captures current clock for next Acquire.
	det.OnRelease(addr, ctx)
}

// racereleasemerge is called by compiler instrumentation on RWMutex unlock operations (Phase 4 Task 4.1).
//
// This is used for RWMutex.Unlock (write unlock) where multiple readers may have
// overlapping critical sections. We merge the current thread's clock into the
// lock's release clock to capture the union of all happens-before relationships.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnReleaseMerge() to merge current clock
//
// Parameters:
//   - addr: Address of the sync.RWMutex being unlocked
//
// Performance: Target <500ns per call (VectorClock merge overhead acceptable).
//
// Zero Allocations: VectorClock merge is zero-allocation.
//
// Example (compiler-generated):
//
//	mu.RUnlock()  // Becomes: runtime.racereleasemerge(uintptr(unsafe.Pointer(&mu))); mu.RUnlock()
//
//go:nosplit
func racereleasemerge(addr uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform sync release merge tracking.
	// This merges current clock into lock's release clock.
	det.OnReleaseMerge(addr, ctx)
}

// === Channel Synchronization API (Phase 4 Task 4.2) ===

// racechansendbefore is called by compiler instrumentation BEFORE channel send (Phase 4 Task 4.2).
//
// This is called before the send operation blocks/completes. For MVP, this is
// a no-op placeholder. Future phases could use this for validation or optimizations.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnChannelSendBefore()
//
// Parameters:
//   - ch: Address of the channel being sent to
//
// Performance: Target <100ns per call (minimal overhead).
//
// Example (compiler-generated):
//
//	ch <- value  // Becomes: runtime.racechansendbefore(&ch); ...; runtime.racechansendafter(&ch)
//
//go:nosplit
func racechansendbefore(ch uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform channel send before tracking (MVP: no-op).
	det.OnChannelSendBefore(ch, ctx)
}

// racechansendafter is called by compiler instrumentation AFTER channel send completes (Phase 4 Task 4.2).
//
// This establishes a happens-before edge from the sender to future receivers.
// The sender's clock is captured into the channel's sendClock.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnChannelSendAfter() to capture sender's clock
//
// Parameters:
//   - ch: Address of the channel being sent to
//
// Performance: Target <500ns per call (VectorClock copy overhead acceptable).
//
// Zero Allocations: First call may allocate ChannelState. Subsequent calls update in place.
//
// Example (compiler-generated):
//
//	ch <- value  // Becomes: runtime.racechansendbefore(&ch); ...; runtime.racechansendafter(&ch)
//
//go:nosplit
func racechansendafter(ch uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform channel send after tracking.
	// This captures sender's clock for receiver to see.
	det.OnChannelSendAfter(ch, ctx)
}

// racechanrecvbefore is called by compiler instrumentation BEFORE channel receive (Phase 4 Task 4.2).
//
// This is called before the receive operation blocks/completes. For MVP, this is
// a no-op placeholder. Future phases could use this for validation or optimizations.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnChannelRecvBefore()
//
// Parameters:
//   - ch: Address of the channel being received from
//
// Performance: Target <100ns per call (minimal overhead).
//
// Example (compiler-generated):
//
//	value := <-ch  // Becomes: runtime.racechanrecvbefore(&ch); ...; runtime.racechanrecvafter(&ch)
//
//go:nosplit
func racechanrecvbefore(ch uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform channel receive before tracking (MVP: no-op).
	det.OnChannelRecvBefore(ch, ctx)
}

// racechanrecvafter is called by compiler instrumentation AFTER channel receive completes (Phase 4 Task 4.2).
//
// This establishes a happens-before edge from the sender to the receiver.
// The receiver merges the sender's clock to observe all the sender's work.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnChannelRecvAfter() to merge sender's clock
//
// Parameters:
//   - ch: Address of the channel being received from
//
// Performance: Target <500ns per call (VectorClock join overhead acceptable).
//
// Zero Allocations: VectorClock join is zero-allocation (in-place update).
//
// Example (compiler-generated):
//
//	value := <-ch  // Becomes: runtime.racechanrecvbefore(&ch); ...; runtime.racechanrecvafter(&ch)
//
//go:nosplit
func racechanrecvafter(ch uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform channel receive after tracking.
	// This merges sender's clock into receiver.
	det.OnChannelRecvAfter(ch, ctx)
}

// racechanclose is called by compiler instrumentation when channel is closed (Phase 4 Task 4.2).
//
// This establishes a happens-before edge from the closer to all future receives.
// The closer's clock is captured into the channel's closeClock.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnChannelClose() to capture closer's clock
//
// Parameters:
//   - ch: Address of the channel being closed
//
// Performance: Target <300ns per call (VectorClock copy overhead acceptable).
//
// Zero Allocations: First call allocates VectorClock for closeClock.
//
// Example (compiler-generated):
//
//	close(ch)  // Becomes: runtime.racechanclose(&ch); close(ch)
//
//go:nosplit
func racechanclose(ch uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform channel close tracking.
	// This captures closer's clock for future receives.
	det.OnChannelClose(ch, ctx)
}

// === WaitGroup Synchronization API (Phase 4 Task 4.3) ===

// racewaitgroupadd is called by compiler instrumentation on WaitGroup.Add(delta) (Phase 4 Task 4.3).
//
// This tracks WaitGroup counter increments. While Add() doesn't establish
// happens-before on its own, we track the counter for optional validation
// and debugging.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnWaitGroupAdd() to track counter
//
// Parameters:
//   - wg: Address of the sync.WaitGroup
//   - delta: The delta to add to the counter
//
// Performance: Target <200ns per call (minimal overhead).
//
// Zero Allocations: First call per goroutine may allocate context.
//
// Example (compiler-generated):
//
//	wg.Add(1)  // Becomes: runtime.racewaitgroupadd(uintptr(unsafe.Pointer(&wg)), 1); wg.Add(1)
//
//go:nosplit
//nolint:unused // Called by compiler instrumentation, not directly from code
func racewaitgroupadd(wg uintptr, delta int) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform WaitGroup add tracking.
	det.OnWaitGroupAdd(wg, delta, ctx)
}

// racewaitgroupdone is called by compiler instrumentation on WaitGroup.Done() (Phase 4 Task 4.3).
//
// This is the critical happens-before operation: Done() captures the current
// thread's clock and merges it into the WaitGroup's doneClock. When Wait()
// returns, it will merge this doneClock, establishing happens-before.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnWaitGroupDone() to merge clock into doneClock
//
// Parameters:
//   - wg: Address of the sync.WaitGroup
//
// Performance: Target <500ns per call (VectorClock merge overhead acceptable).
//
// Zero Allocations: VectorClock merge is zero-allocation.
//
// Example (compiler-generated):
//
//	wg.Done()  // Becomes: runtime.racewaitgroupdone(uintptr(unsafe.Pointer(&wg))); wg.Done()
//
//go:nosplit
//nolint:unused // Called by compiler instrumentation, not directly from code
func racewaitgroupdone(wg uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform WaitGroup done tracking.
	// This merges current thread's clock into doneClock.
	det.OnWaitGroupDone(wg, ctx)
}

// racewaitgroupwaitbefore is called by compiler instrumentation BEFORE WaitGroup.Wait() blocks (Phase 4 Task 4.3).
//
// This is called before Wait() blocks waiting for all Done() calls.
// For MVP, this is primarily a placeholder for future optimizations or validation.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnWaitGroupWaitBefore()
//
// Parameters:
//   - wg: Address of the sync.WaitGroup
//
// Performance: Target <100ns per call (minimal overhead).
//
// Example (compiler-generated):
//
//	wg.Wait()  // Becomes: runtime.racewaitgroupwaitbefore(&wg); ...; runtime.racewaitgroupwaitafter(&wg)
//
//go:nosplit
//nolint:unused // Called by compiler instrumentation, not directly from code
func racewaitgroupwaitbefore(wg uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform WaitGroup wait before tracking (MVP: minimal).
	det.OnWaitGroupWaitBefore(wg, ctx)
}

// racewaitgroupwaitafter is called by compiler instrumentation AFTER WaitGroup.Wait() returns (Phase 4 Task 4.3).
//
// This is the critical happens-before establishment: the waiter merges all
// accumulated Done() clocks into its own clock. After this, all writes
// done before Done() are visible to the waiter.
//
// Flow:
//  1. Check if race detection is enabled (fast atomic load)
//  2. Get or create RaceContext for current goroutine
//  3. Call detector.OnWaitGroupWaitAfter() to merge doneClock
//
// Parameters:
//   - wg: Address of the sync.WaitGroup
//
// Performance: Target <500ns per call (VectorClock merge overhead acceptable).
//
// Zero Allocations: VectorClock merge is zero-allocation.
//
// Example (compiler-generated):
//
//	wg.Wait()  // Becomes: runtime.racewaitgroupwaitbefore(&wg); ...; runtime.racewaitgroupwaitafter(&wg)
//	// After Wait() returns, waiter can safely read child goroutines' writes
//
//go:nosplit
//nolint:unused // Called by compiler instrumentation, not directly from code
func racewaitgroupwaitafter(wg uintptr) {
	// Fast path: Check if race detection is enabled.
	if !enabled.Load() {
		return
	}

	// Get RaceContext for current goroutine.
	ctx := getCurrentContext()

	// Perform WaitGroup wait after tracking.
	// This merges accumulated doneClock into waiter's clock.
	det.OnWaitGroupWaitAfter(wg, ctx)
}

// getCurrentContext returns the RaceContext for the current goroutine.
//
// This function maintains a per-goroutine context cache in the global
// contexts sync.Map. On first access, it:
//  1. Extracts goroutine ID (via fast assembly on amd64, ~1ns)
//  2. Allocates a TID from the reuse pool (0-255)
//  3. Creates a RaceContext for that TID
//  4. Caches it in the map
//
// On subsequent accesses, it just does a map lookup (~5ns).
//
// Performance:
//   - First call per goroutine: ~100ns (includes TID allocation from pool)
//   - Cached calls: ~5ns (sync.Map load operation)
//
// TID Allocation (Phase 2 Task 2.2):
//   - TIDs allocated from reuse pool (supports unlimited goroutines)
//   - Periodic cleanup (every 1000 allocations) reclaims TIDs from dead goroutines
//   - If pool exhausted, cleanup triggered immediately
//
// Thread Safety: Safe for concurrent calls from multiple goroutines.
func getCurrentContext() *goroutine.RaceContext {
	// Step 1: Get goroutine ID for current goroutine.
	// Phase 2.1: Fast assembly implementation on amd64 (~1ns).
	// Fallback: runtime.Stack parsing on other architectures (~4.7µs).
	gid := getGoroutineID()

	// Step 2: Try to load existing context from cache.
	// sync.Map.Load is lock-free for existing keys (fast path).
	if val, ok := contexts.Load(gid); ok {
		return val.(*goroutine.RaceContext)
	}

	// Step 3: Slow path - allocate new context for this goroutine.
	// This happens once per goroutine at first access.

	// Allocate TID from reuse pool.
	// This supports unlimited goroutines by recycling TIDs from dead goroutines.
	tid := allocTID()

	// Create new RaceContext for this goroutine.
	ctx := goroutine.Alloc(tid)

	// Store in cache for future accesses.
	// sync.Map.Store is thread-safe and handles concurrent stores gracefully.
	contexts.Store(gid, ctx)

	// Track TID → GID mapping for cleanup.
	tidToGID.Store(tid, gid)

	// Trigger periodic cleanup to reclaim TIDs from dead goroutines.
	maybeCleanup()

	return ctx
}

// === TID Pool Management Functions (Phase 2 Task 2.2) ===

// initTIDPool initializes the TID reuse pool with all available TIDs (0-255).
//
// This is called once during Init() to set up the free TID stack.
// All 256 TIDs are initially available for allocation.
//
// TIDs are stored in ascending order [0, 1, 2, ..., 255] so that when we pop
// from the end, we allocate TIDs in ascending order (0, 1, 2, ...).
//
// Thread Safety: NOT thread-safe. Must be called during initialization only.
func initTIDPool() {
	tidPoolMu.Lock()
	defer tidPoolMu.Unlock()

	// Initialize free TID stack with all 256 TIDs.
	// Stack order: [0, 1, 2, ..., 255]
	// Popping from end gives: 255, 254, ..., 1, 0
	// But after Init removes TID 0, we get: 255, 254, ..., 1
	// We want ascending allocation, so we reverse the order.
	freeTIDs = make([]uint16, 65536)
	for i := 0; i < 65536; i++ {
		//nolint:gosec // G115: Safe conversion, i is always < 256
		freeTIDs[i] = uint16(i) // Stack order: [0, 1, 2, ..., 255]
	}
}

// allocTID allocates a TID from the free pool.
//
// Algorithm:
//  1. Lock the pool
//  2. If pool is empty, trigger cleanup and retry
//  3. Pop TID from the queue (FIFO - allocate in ascending order)
//
// If cleanup doesn't free any TIDs (pool still exhausted), this reuses TID 0.
// This is graceful degradation - TID conflicts may occur, but program doesn't crash.
//
// Performance: ~50ns (mutex lock + queue pop).
// Lock contention is minimal as allocations are rare relative to memory accesses.
//
// We use FIFO (pop from beginning) instead of LIFO (pop from end) to allocate
// TIDs in ascending order (1, 2, 3, ...) which makes debugging easier.
//
// Thread Safety: Safe for concurrent calls (protected by tidPoolMu).
//
// Returns:
//   - uint8: Allocated TID (0-255)
func allocTID() uint16 {
	tidPoolMu.Lock()

	// Fast path: TID available in pool.
	if len(freeTIDs) > 0 {
		// Pop TID from the front (FIFO - ascending order).
		// freeTIDs is [1, 2, 3, ..., 255] after Init removes TID 0.
		tid := freeTIDs[0]
		freeTIDs = freeTIDs[1:]
		tidPoolMu.Unlock()
		return tid
	}

	// Slow path: Pool exhausted - trigger cleanup.
	tidPoolMu.Unlock()

	// Run cleanup in current goroutine to ensure TIDs are freed before retry.
	// This blocks allocation, but only happens when all 256 TIDs are in use.
	cleanupDeadGoroutines()

	// Retry allocation after cleanup.
	tidPoolMu.Lock()
	defer tidPoolMu.Unlock()

	if len(freeTIDs) > 0 {
		// Cleanup freed some TIDs - allocate one.
		tid := freeTIDs[0]
		freeTIDs = freeTIDs[1:]
		return tid
	}

	// Pool still exhausted after cleanup - graceful degradation.
	// Reuse TID 0 to avoid crashing the program.
	// This may cause TID conflicts in race detection, but better than panic.
	// In practice, this should never happen if cleanup works correctly.
	return 0
}

// freeTID returns a TID to the free pool.
//
// This makes the TID available for reuse by future goroutines.
//
// Performance: ~30ns (mutex lock + stack append).
//
// Thread Safety: Safe for concurrent calls (protected by tidPoolMu).
//
// Parameters:
//   - tid: TID to return to the pool (0-255)
func freeTID(tid uint16) {
	tidPoolMu.Lock()
	defer tidPoolMu.Unlock()

	// Push TID back onto the stack.
	//nolint:makezero // Intentional append to initialized slice (TID pool)
	freeTIDs = append(freeTIDs, tid)
}

// maybeCleanup triggers periodic cleanup of dead goroutines.
//
// Cleanup is triggered every 1000 context allocations to amortize the cost.
// The cleanup runs in a background goroutine to avoid blocking allocations.
//
// Cleanup overhead: ~1ms per 1000 goroutines scanned.
// Amortized overhead: ~0.1% (1ms / 1000 allocations).
//
// Thread Safety: Safe for concurrent calls (uses atomic counter).
func maybeCleanup() {
	// Increment allocation counter.
	count := allocCounter.Add(1)

	// Trigger cleanup every 1000 allocations.
	// This amortizes the ~1ms cleanup cost over 1000 allocations.
	const cleanupInterval = 1000
	if count%cleanupInterval == 0 {
		// Run cleanup in background to avoid blocking current allocation.
		// This is safe because cleanup is idempotent - multiple concurrent
		// cleanups will just scan the same contexts.
		go cleanupDeadGoroutines()
	}
}

// cleanupDeadGoroutines scans the contexts map and reclaims TIDs from dead goroutines.
//
// Algorithm:
//  1. Get list of all live goroutine IDs via runtime.Stack()
//  2. Build a set of live GIDs for O(1) lookup
//  3. Scan contexts map for GIDs not in the live set
//  4. For each dead goroutine, free its TID and remove context
//
// Performance:
//   - runtime.Stack(all=true): ~1ms for 1000 goroutines
//   - Set construction: ~10µs for 1000 goroutines
//   - contexts.Range: ~50µs for 1000 contexts
//   - Total: ~1ms for 1000 goroutines
//
// Thread Safety: Safe for concurrent calls. Uses sync.Map which handles
// concurrent reads/writes/deletes gracefully.
func cleanupDeadGoroutines() {
	// Step 1: Get list of all live goroutine IDs.
	// This is the expensive part (~1ms for 1000 goroutines).
	liveGIDs := getLiveGoroutineIDs()

	// Step 2: Build set for O(1) lookup.
	liveSet := make(map[int64]bool, len(liveGIDs))
	for _, gid := range liveGIDs {
		liveSet[gid] = true
	}

	// Step 3: Scan contexts and remove dead goroutines.
	contexts.Range(func(key, value interface{}) bool {
		gid := key.(int64)
		ctx := value.(*goroutine.RaceContext)

		// Check if goroutine is still alive.
		if !liveSet[gid] {
			// Goroutine is dead - reclaim its TID.
			freeTID(ctx.TID)

			// Remove from contexts map.
			contexts.Delete(gid)

			// Remove from TID → GID mapping.
			tidToGID.Delete(ctx.TID)
		}

		// Continue iteration.
		return true
	})
}

// getLiveGoroutineIDs returns a list of all live goroutine IDs.
//
// This uses runtime.Stack(all=true) to get a stack trace for ALL goroutines,
// then parses the output to extract GIDs.
//
// Performance: ~1ms for 1000 goroutines.
// This is the main cost of cleanup, which is why we amortize it over 1000 allocations.
//
// Thread Safety: Safe for concurrent calls (runtime.Stack is thread-safe).
//
// Returns:
//   - []int64: List of all live goroutine IDs
func getLiveGoroutineIDs() []int64 {
	// Allocate buffer for stack traces.
	// 1MB should be enough for ~1000 goroutines with typical stack depths.
	// If buffer is too small, runtime.Stack returns truncated output,
	// but we'll still get GIDs for all goroutines in the trace.
	buf := make([]byte, 1024*1024) // 1MB

	// Get stack traces for ALL goroutines.
	// all=true is critical - we need every goroutine's stack.
	n := runtime.Stack(buf, true)

	// Parse stack dump to extract all GIDs.
	return parseAllGIDs(buf[:n])
}

// parseAllGIDs parses runtime.Stack(all=true) output to extract all goroutine IDs.
//
// Input format (example):
//
//	goroutine 1 [running]:
//	main.main()
//	    /path/to/main.go:10 +0x20
//
//	goroutine 5 [chan receive]:
//	main.worker()
//	    /path/to/main.go:20 +0x40
//
// We extract: [1, 5, ...]
//
// Algorithm:
//  1. Split buffer into lines
//  2. Find lines starting with "goroutine "
//  3. Parse the GID from each line
//
// Performance: ~100µs for 1000 goroutines.
//
// Parameters:
//   - buf: Stack trace buffer from runtime.Stack(all=true)
//
// Returns:
//   - []int64: List of goroutine IDs
func parseAllGIDs(buf []byte) []int64 {
	var gids []int64

	// Split into lines.
	// runtime.Stack output has one "goroutine N [state]:" line per goroutine.
	i := 0
	for i < len(buf) {
		// Find next newline.
		end := i
		for end < len(buf) && buf[end] != '\n' {
			end++
		}

		// Extract line.
		line := buf[i:end]

		// Check if this is a "goroutine N" line.
		if len(line) >= 10 && string(line[:10]) == "goroutine " {
			// Parse GID from this line.
			gid := parseGID(line)
			if gid != 0 {
				gids = append(gids, gid)
			}
		}

		// Move to next line.
		i = end + 1
	}

	return gids
}

// NOTE: getGoroutineID() and parseGID() are defined in goid_generic.go
// They use assembly-optimized path on amd64/arm64 (Go 1.23-1.25) via goid_fast.go
// or fall back to runtime.Stack parsing via goid_fallback.go

// getcallerpc returns the program counter (PC) of the caller.
//
// This extracts the PC of the memory access that triggered raceread/racewrite.
// The PC can be used to get source location information for race reports.
//
// Call Stack:
//
//	0: getcallerpc()
//	1: raceread() or racewrite()
//	2: instrumented code (the actual memory access)
//
// We want the PC at level 2, so we call runtime.Caller(2).
//
// Performance: ~50ns (runtime.Caller overhead).
//
// MVP: PC is extracted but not used in reporting yet.
// Phase 7: PC will be passed to detector for stack trace generation.
//
// Returns:
//   - uintptr: Program counter of the memory access
//
//nolint:unparam // Return value will be used in Phase 7 for stack traces.
func getcallerpc() uintptr {
	// runtime.Caller(2) skips:
	//   - getcallerpc (this function) - skip 0
	//   - raceread/racewrite - skip 1
	//   - returns: instrumented code - skip 2
	_, _, pc, ok := runtime.Caller(2)
	if !ok {
		return 0
	}
	return uintptr(pc)
}

// Enable turns on race detection.
//
// This is currently a no-op for MVP (always enabled), but provides the
// API hook for Phase 7 when we implement runtime enable/disable.
//
// Thread Safety: Safe for concurrent calls.
func Enable() {
	enabled.Store(true)
}

// Disable turns off race detection.
//
// After calling Disable(), raceread/racewrite become no-ops (fast return).
// This can be used to disable race detection for performance-critical sections.
//
// Thread Safety: Safe for concurrent calls.
//
// Example:
//
//	race.Disable()
//	// ... performance-critical code with known-safe access patterns ...
//	race.Enable()
func Disable() {
	enabled.Store(false)
}

// RacesDetected returns the total number of races detected.
//
// This is exported for testing and statistics purposes.
//
// Thread Safety: Safe for concurrent calls.
//
// Returns:
//   - int: Total number of races detected since initialization
func RacesDetected() int {
	return det.RacesDetected()
}

// RaceRead is an exported wrapper for raceread, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments all memory accesses. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - addr: Memory address being read from
func RaceRead(addr uintptr) {
	raceread(addr)
}

// RaceWrite is an exported wrapper for racewrite, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments all memory accesses. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - addr: Memory address being written to
func RaceWrite(addr uintptr) {
	racewrite(addr)
}

// RaceAcquire is an exported wrapper for raceacquire, for demonstration purposes (Phase 4 Task 4.1).
//
// In production code, you should compile with -race flag, which automatically
// instruments mutex operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - addr: Address of the mutex being locked
func RaceAcquire(addr uintptr) {
	raceacquire(addr)
}

// RaceRelease is an exported wrapper for racerelease, for demonstration purposes (Phase 4 Task 4.1).
//
// In production code, you should compile with -race flag, which automatically
// instruments mutex operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - addr: Address of the mutex being unlocked
func RaceRelease(addr uintptr) {
	racerelease(addr)
}

// RaceReleaseMerge is an exported wrapper for racereleasemerge, for demonstration purposes (Phase 4 Task 4.1).
//
// In production code, you should compile with -race flag, which automatically
// instruments RWMutex operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - addr: Address of the RWMutex being unlocked
func RaceReleaseMerge(addr uintptr) {
	racereleasemerge(addr)
}

// === Exported Channel API Functions (Phase 4 Task 4.2) ===

// RaceChannelSendBefore is an exported wrapper for racechansendbefore, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments channel operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - ch: Address of the channel being sent to
func RaceChannelSendBefore(ch uintptr) {
	racechansendbefore(ch)
}

// RaceChannelSendAfter is an exported wrapper for racechansendafter, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments channel operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - ch: Address of the channel being sent to
func RaceChannelSendAfter(ch uintptr) {
	racechansendafter(ch)
}

// RaceChannelRecvBefore is an exported wrapper for racechanrecvbefore, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments channel operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - ch: Address of the channel being received from
func RaceChannelRecvBefore(ch uintptr) {
	racechanrecvbefore(ch)
}

// RaceChannelRecvAfter is an exported wrapper for racechanrecvafter, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments channel operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - ch: Address of the channel being received from
func RaceChannelRecvAfter(ch uintptr) {
	racechanrecvafter(ch)
}

// RaceChannelClose is an exported wrapper for racechanclose, for demonstration purposes.
//
// In production code, you should compile with -race flag, which automatically
// instruments channel operations. This function is provided for examples
// and testing purposes only.
//
// Parameters:
//   - ch: Address of the channel being closed
func RaceChannelClose(ch uintptr) {
	racechanclose(ch)
}

// Reset resets the detector state for testing.
//
// This clears all shadow memory, resets the race counter, and clears
// the goroutine context cache. It's primarily used in test setup/teardown.
//
// Thread Safety: NOT safe for concurrent access.
// The caller must ensure no other goroutines are using the detector.
func Reset() {
	det.Reset()
	// Clear goroutine contexts.
	contexts = sync.Map{}
	// Clear TID → GID mapping.
	tidToGID = sync.Map{}
	// Reset TID counter.
	nextTID.Store(0)
	// Reset allocation counter.
	allocCounter.Store(0)
	// Reinitialize TID pool for tests.
	// Tests call Reset() but expect to be able to allocate TIDs afterwards.
	initTIDPool()
}

// Init initializes the race detector for use.
//
// This function sets up the race detector runtime and makes it ready to
// track memory accesses. It should be called at the start of your program,
// typically in main() or init().
//
// Init() performs the following initialization steps:
//  1. Enables race detection
//  2. Resets the TID counter to 0
//  3. Creates a fresh detector instance (with optional sampling from env)
//  4. Initializes the TID reuse pool (Phase 2 Task 2.2)
//  5. Allocates a RaceContext for the main goroutine with TID=0
//
// Environment Variables (v0.3.0):
//
//	RACEDETECTOR_SAMPLE_RATE=N  - Enable sampling with rate N (1=disabled, 10=1/10, 100=1/100)
//	                             This trades detection rate for performance (~50-90% overhead reduction).
//	                             Example: RACEDETECTOR_SAMPLE_RATE=10 ./myprogram
//
// Main Goroutine Convention:
// By convention, the main goroutine (the one calling Init) always receives
// TID=0. This is consistent with Go's runtime.raceinit behavior and helps
// identify the main goroutine in race reports.
//
// Init() is idempotent - calling it multiple times is safe and will
// re-initialize the detector with fresh state.
//
// Thread Safety: NOT safe for concurrent calls.
// Init() should only be called during program startup before any
// goroutines are spawned.
//
// Example:
//
//	func main() {
//	    race.Init()
//	    defer race.Fini()
//
//	    // Your program code here...
//	}
//
// Example with sampling:
//
//	$ RACEDETECTOR_SAMPLE_RATE=10 ./myprogram  # Check 1 in 10 accesses
func Init() {
	// Enable race detection.
	enabled.Store(true)

	// Reset TID counter to 0.
	nextTID.Store(0)

	// Reset allocation counter for cleanup trigger.
	allocCounter.Store(0)

	// Create a fresh detector instance with optional sampling (v0.3.0).
	// Check RACEDETECTOR_SAMPLE_RATE environment variable.
	opts := detector.DetectorOptions{}
	if sampleRateStr := os.Getenv("RACEDETECTOR_SAMPLE_RATE"); sampleRateStr != "" {
		if rate, err := strconv.ParseUint(sampleRateStr, 10, 64); err == nil && rate > 1 {
			opts.SamplingEnabled = true
			opts.SampleRate = rate
			fmt.Fprintf(os.Stderr, "Race detector: sampling enabled (rate=%d, ~%.0f%% overhead reduction)\n",
				rate, float64(rate-1)/float64(rate)*100)
		}
	}
	det = detector.NewDetectorWithOptions(opts)

	// Clear any existing goroutine contexts.
	// This ensures a clean slate when re-initializing.
	contexts = sync.Map{}

	// Clear TID → GID mapping.
	tidToGID = sync.Map{}

	// Initialize TID reuse pool (Phase 2 Task 2.2).
	// This sets up the free TID stack with all 256 TIDs available.
	initTIDPool()

	// Allocate RaceContext for the main goroutine.
	// By convention, the main goroutine always gets TID=0.
	gid := getGoroutineID()
	mainCtx := goroutine.Alloc(0)
	contexts.Store(gid, mainCtx)

	// Track main goroutine in TID → GID mapping.
	tidToGID.Store(uint8(0), gid)

	// Remove TID 0 from the free pool (already allocated to main goroutine).
	// TID 0 is at index 0 in the stack: [0, 1, 2, ..., 255]
	tidPoolMu.Lock()
	if len(freeTIDs) > 0 && freeTIDs[0] == 0 {
		// Remove first element (TID 0).
		// Stack becomes: [1, 2, 3, ..., 255]
		// Next allocation pops from end: 255, 254, ...
		// Wait, this is backwards! We want next allocation to be TID 1.
		// Let's pop from the beginning instead when allocating.
		// Actually, let's just remove TID 0 from wherever it is.
		// Since stack is [0, 1, 2, ..., 255], TID 0 is at index 0.
		freeTIDs = freeTIDs[1:] // Now: [1, 2, 3, ..., 255]
	}
	tidPoolMu.Unlock()

	// Increment nextTID so that the next spawned goroutine gets TID >= 1.
	// This ensures TID=0 is reserved exclusively for the main goroutine.
	nextTID.Store(1)
}

// Fini finalizes the race detector and prints a summary report.
//
// This function should be called at the end of your program, typically
// using defer in main() right after Init(). It performs cleanup and
// prints a summary of race detection results to stderr.
//
// The summary report includes:
//   - Total number of races detected (if any)
//   - Success message if no races were found
//
// After Fini() is called, the detector is disabled and raceread/racewrite
// become no-ops. If you need to re-enable detection, call Init() again.
//
// Thread Safety: Safe to call multiple times, but only the first call
// will print the summary. Subsequent calls are no-ops.
//
// Example:
//
//	func main() {
//	    race.Init()
//	    defer race.Fini()
//
//	    // Your program code here...
//	}
//	// On exit, Fini() prints:
//	// ==================
//	// Race Detector Report
//	// ==================
//	// ✓ No data races detected.
//	// ==================
func Fini() {
	// Disable race detection first.
	// This ensures no more race checks happen while we're printing the report.
	enabled.Store(false)

	// Get the total number of races detected.
	racesDetected := det.RacesDetected()

	// Print summary report to stderr.
	// This matches Go's runtime race detector output format.
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "==================\n")
	fmt.Fprintf(os.Stderr, "Race Detector Report\n")
	fmt.Fprintf(os.Stderr, "==================\n")

	if racesDetected == 0 {
		// Success case - no races found.
		fmt.Fprintf(os.Stderr, "✓ No data races detected.\n")
	} else {
		// Warning case - races were detected.
		fmt.Fprintf(os.Stderr, "WARNING: %d data race(s) detected!\n", racesDetected)
		fmt.Fprintf(os.Stderr, "\nSee above for details.\n")
	}

	fmt.Fprintf(os.Stderr, "==================\n\n")
}
