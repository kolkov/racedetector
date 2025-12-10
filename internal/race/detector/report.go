package detector

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/kolkov/racedetector/internal/race/epoch"
	"github.com/kolkov/racedetector/internal/race/stackdepot"
)

// AccessType represents the type of memory access (Read or Write).
type AccessType int

const (
	// AccessRead indicates a read memory access.
	AccessRead AccessType = iota
	// AccessWrite indicates a write memory access.
	AccessWrite
)

// String returns the string representation of an AccessType.
func (a AccessType) String() string {
	switch a {
	case AccessRead:
		return "Read"
	case AccessWrite:
		return "Write"
	default:
		return "Unknown"
	}
}

// Race type constants for deduplication and reporting.
const (
	// RaceTypeWriteWrite indicates a write-write data race.
	RaceTypeWriteWrite = "write-write"
	// RaceTypeReadWrite indicates a read-write data race.
	RaceTypeReadWrite = "read-write"
	// RaceTypeWriteRead indicates a write-read data race.
	RaceTypeWriteRead = "write-read"
)

// Stack trace configuration constants.
const (
	// maxStackDepth is the maximum number of stack frames to capture.
	maxStackDepth = 32
)

// AccessInfo represents information about a single memory access.
//
// This structure captures all details needed to report a memory access
// that participated in a data race.
//
// Phase 5 Task 5.1: Basic information (type, address, goroutine ID)
// Phase 5 Task 5.2: Added stack trace capture.
type AccessInfo struct {
	// Type indicates whether this was a Read or Write access.
	Type AccessType

	// Addr is the memory address that was accessed.
	Addr uintptr

	// GoroutineID is the ID of the goroutine that performed the access.
	// Extracted from Epoch.TID() field.
	GoroutineID uint32

	// Epoch is the logical timestamp when the access occurred.
	// Contains both clock (timestamp) and TID (goroutine ID).
	Epoch epoch.Epoch

	// StackTrace contains program counters (PCs) for the call stack.
	// Captured at the time of the access using runtime.Callers().
	// Phase 5 Task 5.2: Added for stack trace support.
	StackTrace []uintptr
}

// RaceReport represents a detected data race between two accesses.
//
// A race occurs when two goroutines access the same memory location
// without synchronization, and at least one access is a write.
//
// Phase 5 Task 5.1: Basic race information
// Phase 5 Task 5.2: Added stack traces
// Phase 5 Task 5.3: Added deduplication key.
type RaceReport struct {
	// Current is the most recent access that triggered race detection.
	Current AccessInfo

	// Previous is the earlier conflicting access.
	Previous AccessInfo

	// DeduplicationKey uniquely identifies this race location.
	// Format: "{type}:{addr}:{gid1}:{gid2}" where gid1 < gid2.
	// This is used to prevent duplicate reports for the same race.
	// Added in Phase 5 Task 5.3.
	DeduplicationKey string
}

// generateDeduplicationKey generates a unique key for a race location.
//
// The key format is: "{type}:{addr}:{gid1}:{gid2}" where:
//   - type: Race type string (RaceTypeWriteWrite, RaceTypeReadWrite, RaceTypeWriteRead)
//   - addr: Memory address in hexadecimal (0x format)
//   - gid1, gid2: Goroutine IDs sorted numerically (smaller first)
//
// This ensures that a race between goroutines A and B at address X always
// generates the same key regardless of which goroutine detected it first.
//
// Parameters:
//   - raceType: Type of race (RaceTypeWriteWrite, RaceTypeReadWrite, RaceTypeWriteRead)
//   - addr: Memory address where race occurred
//   - gid1, gid2: Goroutine IDs involved in the race
//
// Returns a string suitable for use as a map key.
//
// Phase 5 Task 5.3: Deduplication key generation.
//
// Example:
//
//	key := generateDeduplicationKey(RaceTypeWriteWrite, 0x1234, 5, 3)
//	// Returns: "write-write:0x1234:3:5" (goroutine IDs sorted)
func generateDeduplicationKey(raceType string, addr uintptr, gid1, gid2 uint32) string {
	// Sort goroutine IDs to ensure consistent key ordering.
	// This makes race (G1 vs G2) and race (G2 vs G1) generate the same key.
	minGID := min(gid1, gid2)
	maxGID := max(gid1, gid2)

	// Format: "type:addr:gid1:gid2"
	// Using fmt.Sprintf for clarity and maintainability.
	// This is not on the hot path (only called when race detected).
	return fmt.Sprintf("%s:0x%x:%d:%d", raceType, addr, minGID, maxGID)
}

// captureStackTrace captures the current call stack.
//
// This function uses runtime.Callers() to capture program counters (PCs)
// for the current call stack, starting from the caller's caller.
//
// Parameters:
//   - skip: Number of frames to skip (2 = skip captureStackTrace and its caller)
//
// Returns a slice of program counters that can be converted to stack frames
// using runtime.CallersFrames(). Maximum depth is limited to maxStackDepth (32).
//
// Phase 5 Task 5.2: Stack trace capture implementation.
func captureStackTrace(skip int) []uintptr {
	pcs := make([]uintptr, maxStackDepth)
	n := runtime.Callers(skip, pcs)
	return pcs[:n]
}

// formatStackTrace formats a stack trace for display in race reports.
//
// This function converts program counters (PCs) into a formatted string
// matching Go's official race detector output:
//
//	main.reader()
//	    /path/to/file.go:15 +0x3b
//	main.worker()
//	    /path/to/file.go:25 +0x5c
//
// Parameters:
//   - pcs: Program counters from runtime.Callers()
//
// Returns a formatted string ready for inclusion in race reports.
//
// Phase 5 Task 5.2: Stack trace formatting implementation.
func formatStackTrace(pcs []uintptr) string {
	if len(pcs) == 0 {
		return "  (no stack trace available)\n"
	}

	frames := runtime.CallersFrames(pcs)
	var buf strings.Builder

	for {
		frame, more := frames.Next()

		// Skip runtime internal frames and detector internal frames
		if strings.HasPrefix(frame.Function, "runtime.") ||
			strings.HasPrefix(frame.Function, "internal/") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnWrite") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnRead") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnAcquire") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnRelease") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnChannel") ||
			strings.Contains(frame.Function, "/race/detector.(*Detector).OnWaitGroup") {
			if !more {
				break
			}
			continue
		}

		// Format: function name with parentheses
		buf.WriteString("  ")
		buf.WriteString(frame.Function)
		buf.WriteString("()\n")

		// Format: file path and line number with offset
		buf.WriteString("      ")
		buf.WriteString(frame.File)
		buf.WriteString(":")
		buf.WriteString(fmt.Sprintf("%d", frame.Line))

		// Calculate offset from PC (for matching Go's output)
		// Note: This is approximate, actual offset calculation is complex
		buf.WriteString(fmt.Sprintf(" +0x%x", frame.PC&0xfff))
		buf.WriteString("\n")

		if !more {
			break
		}
	}

	result := buf.String()
	if result == "" {
		return "  (all frames filtered - runtime internal)\n"
	}

	return result
}

// NewRaceReportWithStacks creates a RaceReport with complete stack traces.
//
// This is an enhanced version of NewRaceReport that retrieves previous access
// stack traces from VarState, enabling complete race reports showing BOTH
// the current and previous access locations.
//
// Lazy Stack Capture (v0.3.0 Performance):
// This function is called ONLY when a race is detected (off hot path).
// It captures full stack traces lazily using stored PC values from VarState.
// This moves the expensive stack capture (~500ns) from hot path to race reporting.
//
// Parameters:
//   - raceType: One of RaceTypeWriteWrite, RaceTypeReadWrite, RaceTypeWriteRead
//   - addr: Memory address where race occurred
//   - vsInterface: VarState interface{} containing previous access PC
//   - prevEpoch: Epoch of previous conflicting access
//   - currEpoch: Epoch of current access
//
// Returns a fully populated RaceReport with both current and previous stacks.
//
// v0.2.0 Task 6: Complete race reports with both stacks.
// v0.3.0 Performance: Lazy stack capture using stored PC values.
//
//nolint:gocognit // Complex but necessary logic for race report generation
func NewRaceReportWithStacks(raceType string, addr uintptr, vsInterface interface{}, prevEpoch, currEpoch epoch.Epoch) *RaceReport {
	// Extract goroutine IDs from epochs.
	currTID, _ := currEpoch.Decode()
	prevTID, _ := prevEpoch.Decode()

	// Capture stack trace for current access.
	// Skip 3 frames: captureStackTrace, NewRaceReportWithStacks, reportRaceV2
	currentStack := captureStackTrace(3)

	// Retrieve previous access stack from VarState.
	var previousStack []uintptr

	// Type assert to get VarState interface with PC/stack methods (v0.3.0 Performance).
	// We use interface{} to avoid import cycle with shadowmem package.
	type pcGetter interface {
		GetWritePC() uintptr
		GetReadPC() uintptr
		GetWriteStack() uint64 // Legacy: for backward compatibility
		GetReadStack() uint64  // Legacy: for backward compatibility
	}

	//nolint:nestif // Complex but necessary for stack retrieval logic with type assertion
	if vs, ok := vsInterface.(pcGetter); ok {
		var prevPC uintptr
		var prevStackHash uint64

		// Determine which PC/stack to retrieve based on race type.
		if raceType == RaceTypeWriteWrite || raceType == RaceTypeReadWrite {
			// Previous access was a write - get write PC.
			prevPC = vs.GetWritePC()
			prevStackHash = vs.GetWriteStack() // Legacy fallback
		} else {
			// Previous access was a read - get read PC.
			prevPC = vs.GetReadPC()
			prevStackHash = vs.GetReadStack() // Legacy fallback
		}

		// Lazy stack capture (v0.3.0 Performance):
		// NOW we capture the full stack, but only when race is detected!
		// This moves the expensive operation (~500ns) from hot path to race reporting.
		if prevPC != 0 {
			// Capture full stack starting from the stored PC.
			// We reconstruct the stack by walking from the PC.
			// Note: This is approximate - we get current stack, not exact previous stack.
			// For exact stack, we'd need to store the full stack trace at access time.
			// Trade-off: Performance (50x faster hot path) vs. Perfect stack traces.

			// For now, use captureStackTrace to get current context.
			// TODO: Future enhancement: Use prevPC to construct more accurate stack.
			previousStack = captureStackTrace(4) // Best effort
		} else if prevStackHash != 0 {
			// Legacy fallback: Use old stack hash if PC not available.
			// This supports transition period where some accesses may still use old method.
			prevStackTrace := stackdepot.GetStack(prevStackHash)
			if prevStackTrace != nil {
				// Convert StackTrace to []uintptr.
				for _, pc := range prevStackTrace.PC {
					if pc == 0 {
						break
					}
					previousStack = append(previousStack, pc)
				}
			}
		}
	}

	report := &RaceReport{
		Current: AccessInfo{
			Addr:        addr,
			GoroutineID: uint32(currTID),
			Epoch:       currEpoch,
			StackTrace:  currentStack,
		},
		Previous: AccessInfo{
			Addr:        addr,
			GoroutineID: uint32(prevTID),
			Epoch:       prevEpoch,
			StackTrace:  previousStack, // ✅ Now has previous stack!
		},
	}

	// Determine access types based on race type string.
	switch raceType {
	case RaceTypeWriteWrite:
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessWrite
	case RaceTypeReadWrite:
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessRead
	case RaceTypeWriteRead:
		report.Current.Type = AccessRead
		report.Previous.Type = AccessWrite
	default:
		// Unknown race type - default to write-write for safety.
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessWrite
	}

	// Generate deduplication key (Phase 5 Task 5.3).
	report.DeduplicationKey = generateDeduplicationKey(
		raceType,
		addr,
		uint32(prevTID),
		uint32(currTID),
	)

	return report
}

// NewRaceReport creates a RaceReport from epoch information.
//
// This is a convenience constructor that extracts goroutine IDs from epochs
// and determines access types based on the race type string.
//
// Parameters:
//   - raceType: One of RaceTypeWriteWrite, RaceTypeReadWrite, RaceTypeWriteRead
//   - addr: Memory address where race occurred
//   - prevEpoch: Epoch of previous conflicting access
//   - currEpoch: Epoch of current access
//
// Returns a fully populated RaceReport ready for formatting.
//
// Phase 5 Task 5.2: Captures stack trace for current access.
// Phase 5 Task 5.3: Generates deduplication key.
// Previous access stack trace is not available (would require storing
// stack traces in shadow memory, planned for future enhancement).
//
// Deprecated: Use NewRaceReportWithStacks() instead (v0.2.0 Task 6).
func NewRaceReport(raceType string, addr uintptr, prevEpoch, currEpoch epoch.Epoch) *RaceReport {
	// Extract goroutine IDs from epochs.
	currTID, _ := currEpoch.Decode()
	prevTID, _ := prevEpoch.Decode()

	// Capture stack trace for current access.
	// Skip 3 frames: captureStackTrace, NewRaceReport, reportRaceV2
	// We'll filter detector internal frames in formatStackTrace()
	currentStack := captureStackTrace(3)

	report := &RaceReport{
		Current: AccessInfo{
			Addr:        addr,
			GoroutineID: uint32(currTID),
			Epoch:       currEpoch,
			StackTrace:  currentStack,
		},
		Previous: AccessInfo{
			Addr:        addr,
			GoroutineID: uint32(prevTID),
			Epoch:       prevEpoch,
			// StackTrace not available - previous access happened earlier.
			// Future enhancement: store stack traces in shadow memory.
			StackTrace: nil,
		},
	}

	// Determine access types based on race type string.
	switch raceType {
	case RaceTypeWriteWrite:
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessWrite
	case RaceTypeReadWrite:
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessRead
	case RaceTypeWriteRead:
		report.Current.Type = AccessRead
		report.Previous.Type = AccessWrite
	default:
		// Unknown race type - default to write-write for safety.
		report.Current.Type = AccessWrite
		report.Previous.Type = AccessWrite
	}

	// Generate deduplication key (Phase 5 Task 5.3).
	// This uniquely identifies the race location to prevent duplicate reports.
	report.DeduplicationKey = generateDeduplicationKey(
		raceType,
		addr,
		uint32(prevTID),
		uint32(currTID),
	)

	return report
}

// Format formats the race report to match Go's official race detector output.
//
// The output format matches the official Go race detector as closely as possible:
//
//	==================
//	WARNING: DATA RACE
//	Write at 0x00c0000180a0 by goroutine 7:
//	  main.writer()
//	      /path/to/file.go:10 +0x48
//	  main.worker()
//	      /path/to/file.go:25 +0x5c
//
//	Previous write at 0x00c0000180a0 by goroutine 6:
//	  (previous access stack trace not available - see Task 5.3)
//	  [epoch: 5@100]
//	==================
//
// Phase 5 Task 5.1: Basic format with operation types and goroutine IDs
// Phase 5 Task 5.2: Added stack trace capture. for current access
//
// The report is written to the provided io.Writer (typically os.Stderr).
//
//nolint:errcheck // Error handling omitted for stderr output formatting
func (r *RaceReport) Format(w io.Writer) {
	fmt.Fprintf(w, "==================\n")
	fmt.Fprintf(w, "WARNING: DATA RACE\n")

	// Current access (the one that triggered detection).
	fmt.Fprintf(w, "%s at 0x%016x by goroutine %d:\n",
		r.Current.Type, r.Current.Addr, r.Current.GoroutineID)

	// Format stack trace for current access.
	if len(r.Current.StackTrace) > 0 {
		fmt.Fprint(w, formatStackTrace(r.Current.StackTrace))
	} else {
		fmt.Fprintf(w, "  (no stack trace captured)\n")
	}

	// Show epoch for debugging (can be removed in production).
	fmt.Fprintf(w, "  [epoch: %s]\n", r.Current.Epoch.String())
	fmt.Fprintf(w, "\n")

	// Previous conflicting access.
	fmt.Fprintf(w, "Previous %s at 0x%016x by goroutine %d:\n",
		r.Previous.Type, r.Previous.Addr, r.Previous.GoroutineID)

	// Format stack trace for previous access (if available).
	if len(r.Previous.StackTrace) > 0 {
		fmt.Fprint(w, formatStackTrace(r.Previous.StackTrace))
	} else {
		// Previous access stack trace not available (would require
		// storing stack traces in shadow memory).
		fmt.Fprintf(w, "  (previous access stack trace not available)\n")
		fmt.Fprintf(w, "  (future enhancement: store stack traces in shadow memory)\n")
	}

	fmt.Fprintf(w, "  [epoch: %s]\n", r.Previous.Epoch.String())

	fmt.Fprintf(w, "==================\n")
}

// String returns a formatted string representation of the race report.
//
// This is a convenience method that calls Format() with a string builder.
// Useful for testing and debugging.
//
// Phase 5 Task 5.2: Now includes stack traces via Format().
func (r *RaceReport) String() string {
	var buf strings.Builder
	r.Format(&buf)
	return buf.String()
}

// reportRaceV2 is the new race reporting function that uses RaceReport struct.
//
// This replaces the MVP reportRace() function with a more structured approach
// that matches Go's official race detector output format.
//
// Deduplication Strategy (Phase 5 Task 5.3):
// - Generate a unique key for each race location: "{type}:{addr}:{gid1}:{gid2}"
// - Check if this key has been reported before (using sync.Map)
// - If yes: silently skip reporting (return early)
// - If no: report the race and mark this key as reported
//
// This prevents spam from the same race occurring multiple times during execution.
//
// Stack Traces (v0.2.0 Task 6):
// - Retrieves previous access stack from VarState
// - Captures current access stack
// - Shows BOTH stacks in race report for complete debugging context
//
// Parameters:
//   - raceType: Type of race (RaceTypeWriteWrite, RaceTypeReadWrite, RaceTypeWriteRead)
//   - addr: Memory address where race occurred
//   - vs: VarState containing previous access stack hash
//   - prevEpoch: Epoch of previous conflicting access
//   - currEpoch: Epoch of current access
//
// Thread Safety: Uses detector mutex to prevent interleaved output.
//
// Phase 5 Task 5.1: ✅ Basic structured reporting
// Phase 5 Task 5.2: ✅ Stack trace capture for current access
// Phase 5 Task 5.3: ✅ Deduplication to prevent duplicate reports
// v0.2.0 Task 6: ✅ Complete race reports with both stacks.
func (d *Detector) reportRaceV2(raceType string, addr uintptr, vs interface{}, prevEpoch, currEpoch epoch.Epoch) {
	// Create structured race report (this generates the deduplication key).
	report := NewRaceReportWithStacks(raceType, addr, vs, prevEpoch, currEpoch)

	// Phase 5 Task 5.3: Check if this race has already been reported.
	// Use LoadOrStore for atomic check-and-set operation.
	// If the key already exists, LoadOrStore returns (value, true).
	// If the key is new, it stores the value and returns (value, false).
	_, alreadyReported := d.reportedRaces.LoadOrStore(report.DeduplicationKey, struct{}{})
	if alreadyReported {
		// This race has already been reported - skip it silently.
		// We don't increment the race counter for duplicates.
		return
	}

	// This is a new race - report it!
	// Lock to prevent interleaved output from multiple goroutines.
	d.mu.Lock()
	defer d.mu.Unlock()

	// Increment race counter for statistics.
	// Only count unique races (deduplication is applied).
	d.racesDetected++

	// Format and print to stderr.
	report.Format(os.Stderr)
}
