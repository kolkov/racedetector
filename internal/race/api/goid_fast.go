// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.23 && !go1.26 && (amd64 || arm64)

// Fast goroutine ID extraction using assembly for supported platforms.
//
// This file provides the optimized path for goroutine ID extraction on
// amd64 and arm64 architectures running Go 1.23-1.25. It uses assembly
// to directly access the runtime's g struct pointer and extract the goid
// field at a known offset.
//
// Performance: ~1-2ns per call (vs ~1500ns for runtime.Stack parsing).
//
// Architecture Support:
//   - amd64: Uses TLS to access g pointer
//   - arm64: Uses dedicated g register (R28)
//
// Go Version Support:
//   - Go 1.23: goid offset = 152 bytes
//   - Go 1.24: goid offset = 152 bytes
//   - Go 1.25: goid offset = 152 bytes
//   - Go 1.26+: Requires verification (build tag excludes)
//
// The offset is calculated from the runtime.g struct layout:
//
//	Field          Size    Cumulative Offset
//	-----          ----    -----------------
//	stack          16      0
//	stackguard0    8       16
//	stackguard1    8       24
//	_panic         8       32
//	_defer         8       40
//	m              8       48
//	sched (gobuf)  48      56  (sp, pc, g, ctxt, lr, bp = 6×8)
//	syscallsp      8       104
//	syscallpc      8       112
//	syscallbp      8       120
//	stktopsp       8       128
//	param          8       136
//	atomicstatus   4       144
//	stackLock      4       148
//	goid           8       152 ← TARGET
//
// IMPORTANT: If Go runtime changes the g struct layout, this offset
// must be updated. The build tag !go1.26 ensures we don't accidentally
// use incorrect offset on newer Go versions.

package api

import (
	"unsafe"
)

// goidOffset is the byte offset of the goid field within runtime.g struct.
//
// This value is verified for Go 1.23, 1.24, and 1.25 on both amd64 and arm64.
// The offset is the same across these versions and architectures.
//
// CRITICAL: This offset MUST be verified when adding support for new Go versions.
// Use the tools/calc_goid_offset.go utility or manually inspect runtime/runtime2.go.
const goidOffset = 152

// getg returns the current goroutine's g struct pointer.
// Implemented in assembly (goid_amd64.s or goid_arm64.s).
//
//go:noescape
func getg() uintptr

// getGoroutineIDFast extracts the goroutine ID using assembly fast path.
//
// This is the optimized implementation for amd64/arm64 on Go 1.23-1.25.
// It directly reads the goid field from the runtime.g struct.
//
// Performance: ~1-2ns per call (single assembly call + pointer arithmetic).
//
// Safety:
//   - NOSPLIT in assembly: Cannot grow stack (critical for race detector)
//   - No allocations: Pure pointer arithmetic
//   - Thread-safe: Each goroutine accesses its own TLS/g register
//
// Fallback:
//   - If g pointer is nil (should never happen), falls back to slow path
//   - On unsupported platforms/versions, goid_fallback.go is used instead
//
// Returns:
//   - int64: Goroutine ID (always positive, unique per goroutine)
//
//go:nosplit
//go:nocheckptr
func getGoroutineIDFast() int64 {
	// Get g pointer from assembly stub.
	gptr := getg()

	// Safety check: nil g pointer indicates serious runtime issue.
	// This should never happen in normal operation, but we handle it
	// gracefully by falling back to the slow path.
	if gptr == 0 {
		return getGoroutineIDSlow()
	}

	// Extract goid field at known offset.
	// The goid field is a uint64 at offset 152 bytes from g pointer.
	//
	// We use a two-step conversion to satisfy go vet:
	//   1. Convert gptr+offset to unsafe.Pointer (via uintptr arithmetic)
	//   2. Convert to *uint64 and dereference
	//
	// Note: go vet warns about uintptr→unsafe.Pointer conversion because
	// the GC could move the object between computing the address and using it.
	// However, this is safe here because:
	//   - The g struct is pinned in memory (never moved by GC)
	//   - gptr comes directly from assembly (getg())
	//   - We only read the goid field, never write
	//   - This pattern is used by other goid libraries (petermattis/goid, etc.)
	//
	// The nolint directive is necessary because this is legitimate low-level code.
	//nolint:gosec // G103: Intentional unsafe pointer arithmetic for runtime access
	goid := *(*uint64)(unsafe.Pointer(gptr + goidOffset))

	// Return as int64 for API consistency with rest of race detector.
	// Goroutine IDs are always positive and fit comfortably in int64.
	//nolint:gosec // G115: Safe conversion - goid values never exceed int64 max
	return int64(goid)
}
