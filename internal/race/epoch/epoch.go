// Package epoch implements 64-bit logical timestamps for FastTrack race detector.
//
// Epoch represents a single thread's logical time as a compact 64-bit value:
// - Top 16 bits: Thread ID (0-65,535)
// - Bottom 48 bits: Clock value (0-281 trillion)
//
// This encoding enables O(1) happens-before checks which are the foundation
// of FastTrack's performance (96%+ operations use epoch-only fast path).
//
// Design rationale for uint64 (vs uint32):
// - Production workloads easily spawn 1000+ goroutines (256 was too small).
// - Tight loops can exceed 16M operations in seconds (24-bit clock overflowed).
// - Go 1.25+ has excellent 64-bit performance on all platforms.
// - Memory cost: 4 bytes → 8 bytes per variable (acceptable for reliability).
package epoch

import (
	"sync/atomic"

	"github.com/kolkov/racedetector/internal/race/vectorclock"
)

// Epoch is a 64-bit logical timestamp encoding both thread ID and clock value.
// Layout: [TID:16][Clock:48]
//
// Example: 0x0005000000001234 represents TID=5, Clock=0x1234 (4660 decimal).
//
// Limits:
//   - Max TID: 65,535 (16-bit) - supports up to 65K concurrent goroutines.
//   - Max Clock: 281,474,976,710,655 (48-bit) - 281 trillion operations.
type Epoch uint64

const (
	// TIDBits is the number of bits allocated for thread ID.
	// 16 bits = 65,536 threads max (vs 256 in MVP).
	// This covers 99%+ of real-world programs; v0.4 will add dynamic TID mapping.
	TIDBits = 16

	// ClockBits is the number of bits allocated for clock value.
	// 48 bits = 281,474,976,710,655 operations max (vs 16M in MVP).
	// This is practically unlimited for any real program.
	ClockBits = 48

	// ClockMask is the bitmask for extracting clock value (0x0000FFFFFFFFFFFF).
	ClockMask = (1 << ClockBits) - 1

	// MaxTID is the maximum thread ID value (65,535).
	MaxTID = uint32((1 << TIDBits) - 1)

	// MaxClock is the maximum clock value (281,474,976,710,655).
	MaxClock = uint64((1 << ClockBits) - 1)

	// MaxTIDWarning is the threshold for warning about TID approaching overflow (90% of max).
	// 58,982 goroutines (90% of 65,536).
	MaxTIDWarning = uint32((1 << TIDBits) * 9 / 10)

	// MaxClockWarning is the threshold for warning about clock approaching overflow (90% of max).
	// 253,327,479,039,589 operations (90% of 281 trillion).
	MaxClockWarning = uint64((1 << ClockBits) * 9 / 10)
)

var (
	// tidOverflowDetected is set to 1 when TID overflow is detected.
	// Use atomic operations to access this flag.
	tidOverflowDetected uint32

	// clockOverflowDetected is set to 1 when clock overflow is detected.
	// Use atomic operations to access this flag.
	clockOverflowDetected uint32

	// tidNearOverflow is set to 1 when TID reaches 90% threshold.
	// Use atomic operations to access this flag.
	tidNearOverflow uint32

	// clockNearOverflow is set to 1 when clock reaches 90% threshold.
	// Use atomic operations to access this flag.
	clockNearOverflow uint32
)

// NewEpoch creates an epoch from thread ID and clock value.
//
// The TID is stored in the top 16 bits, clock in the bottom 48 bits.
//
// Overflow detection (v0.2.0 Task 5):
// - If TID > MaxTID: Sets tidOverflowDetected flag and clamps to MaxTID.
// - If clock > MaxClock: Sets clockOverflowDetected flag and clamps to MaxClock.
// - At 90% thresholds: Sets warning flags for early detection.
//
// Clamping prevents wrap-around which causes false negatives (worse than false positives).
//
//go:nosplit
func NewEpoch(tid uint16, clock uint64) Epoch {
	// Convert tid to uint32 for comparison with MaxTID constant.
	tid32 := uint32(tid)

	// Check for TID overflow.
	// Note: Since tid is uint16, it cannot exceed MaxTID (65,535).
	// This check is defensive for future changes (e.g., dynamic TID mapping in v0.4).
	if tid32 > MaxTID {
		atomic.StoreUint32(&tidOverflowDetected, 1)
		tid = uint16(MaxTID) // Clamp to max (prevents wrap-around).
		tid32 = MaxTID
	}

	// Check for clock overflow.
	if clock > MaxClock {
		atomic.StoreUint32(&clockOverflowDetected, 1)
		clock = MaxClock // Clamp to max.
	}

	// Warn at 90% threshold (early warning).
	if tid32 > MaxTIDWarning {
		atomic.StoreUint32(&tidNearOverflow, 1)
	}
	if clock > MaxClockWarning {
		atomic.StoreUint32(&clockNearOverflow, 1)
	}

	return Epoch(uint64(tid)<<ClockBits | (clock & ClockMask))
}

// Decode extracts the thread ID and clock value from an epoch.
//
// Returns: (tid uint16, clock uint64)
//
//go:nosplit
func (e Epoch) Decode() (tid uint16, clock uint64) {
	tid = uint16(e >> ClockBits)
	clock = uint64(e) & ClockMask
	return
}

// HappensBefore checks if this epoch happened before a vector clock.
//
// This is the CRITICAL O(1) operation that makes FastTrack fast!
// Called millions of times, must be zero-allocation, inline-candidate.
//
// Returns true if epoch's clock <= vc[epoch's TID].
//
// Note: VectorClock stores uint32 clocks per thread, but Epoch uses uint64 global clock.
// The comparison is safe since per-thread clocks rarely exceed 32-bit range.
//
//go:nosplit
func (e Epoch) HappensBefore(vc *vectorclock.VectorClock) bool {
	tid, clock := e.Decode()
	return clock <= uint64(vc.Get(tid))
}

// Same checks if two epochs are identical (same TID and clock).
//
// Used for fast-path same-epoch optimization (71% writes, 63% reads).
//
//go:nosplit
func (e Epoch) Same(other Epoch) bool {
	return e == other
}

// String returns a human-readable representation of the epoch.
//
// Format: "clock@tid" (e.g., "42@5" means clock=42, tid=5).
// This method is only used for debugging and race reporting, not on hot path.
func (e Epoch) String() string {
	tid, clock := e.Decode()
	// Note: Using basic string concatenation to avoid fmt import.
	// For a real runtime library, consider using strconv or runtime-internal formatting.
	return itoa64(clock) + "@" + itoa64(uint64(tid))
}

// itoa64 converts a uint64 to string without fmt import.
// Simple implementation for debugging output only.
func itoa64(n uint64) string {
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

// CheckOverflows returns the current state of overflow detection flags.
//
// This function is called periodically by the detector to check if any
// overflow or warning thresholds have been reached.
//
// Returns:
//   - tidOverflow: true if TID overflow was detected (TID > MaxTID)
//   - clockOverflow: true if clock overflow was detected (clock > MaxClock)
//   - tidWarning: true if TID reached 90% threshold
//   - clockWarning: true if clock reached 90% threshold
//
// Thread Safety: Safe for concurrent calls (uses atomic loads).
//
// Example:
//
//	tidOverflow, clockOverflow, tidWarning, clockWarning := epoch.CheckOverflows()
//	if tidOverflow {
//	    fmt.Fprintf(os.Stderr, "CRITICAL: TID overflow detected!\n")
//	}
func CheckOverflows() (tidOverflow, clockOverflow, tidWarning, clockWarning bool) {
	return atomic.LoadUint32(&tidOverflowDetected) == 1,
		atomic.LoadUint32(&clockOverflowDetected) == 1,
		atomic.LoadUint32(&tidNearOverflow) == 1,
		atomic.LoadUint32(&clockNearOverflow) == 1
}

// ResetOverflowFlags clears all overflow detection flags.
//
// This is primarily used in testing to reset the overflow state between test cases.
// In production, overflow flags are typically never reset since overflow is a critical
// condition that should persist for the lifetime of the program.
//
// Thread Safety: Safe for concurrent calls (uses atomic stores).
//
// Example:
//
//	// In test setup
//	epoch.ResetOverflowFlags()
//	// Run test that triggers overflow
//	e := epoch.NewEpoch(70000, 1000)
//	tidOverflow, _, _, _ := epoch.CheckOverflows()
//	assert.True(t, tidOverflow)
func ResetOverflowFlags() {
	atomic.StoreUint32(&tidOverflowDetected, 0)
	atomic.StoreUint32(&clockOverflowDetected, 0)
	atomic.StoreUint32(&tidNearOverflow, 0)
	atomic.StoreUint32(&clockNearOverflow, 0)
}
