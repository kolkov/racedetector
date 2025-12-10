// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Common goroutine ID extraction utilities.
//
// This file contains platform-independent code used by both the fast
// (assembly) and slow (runtime.Stack) paths for goroutine ID extraction.
//
// The actual getGoroutineIDFast() function is provided by:
//   - goid_fast.go: Assembly-optimized path (Go 1.23-1.25, amd64/arm64)
//   - goid_fallback.go: Stack parsing path (all other configurations)
//
// API:
//   - getGoroutineID(): Main entry point, uses fast path
//   - getGoroutineIDFast(): Provided by goid_fast.go or goid_fallback.go
//   - getGoroutineIDSlow(): Always available, uses runtime.Stack parsing
//   - parseGID(): Parses goroutine ID from stack trace bytes

package api

import "runtime"

// getGoroutineID returns the current goroutine ID.
//
// This is the main entry point for goroutine ID extraction. It delegates
// to getGoroutineIDFast() which uses the best available implementation:
//   - Assembly fast path on supported platforms (~1-2ns)
//   - Stack parsing fallback on other platforms (~1500ns)
//
// Returns:
//   - int64: Goroutine ID (always positive, unique per goroutine)
func getGoroutineID() int64 {
	return getGoroutineIDFast()
}

// getGoroutineIDSlow extracts goroutine ID by parsing runtime.Stack output.
//
// This is the universal fallback method that works on all Go versions and
// architectures. It parses the first line of the stack trace to extract
// the goroutine ID.
//
// Stack trace format: "goroutine 123 [running]:\n..."
//
// Performance: ~1500ns per call (dominated by runtime.Stack allocation).
//
// This function is called:
//   - Directly by goid_fallback.go on unsupported platforms
//   - As fallback if assembly path returns nil g pointer
//   - For testing/validation against assembly implementation
//
// Returns:
//   - int64: Goroutine ID (always positive), or 0 if parsing fails
func getGoroutineIDSlow() int64 {
	// Allocate buffer for stack trace.
	// We only need the first line, so 64 bytes is sufficient.
	// Format: "goroutine 123 [running]:\n..."
	var buf [64]byte

	// Get stack trace for current goroutine only (all=false).
	n := runtime.Stack(buf[:], false)

	// Parse goroutine ID from the buffer.
	return parseGID(buf[:n])
}

// parseGID extracts the goroutine ID from stack trace bytes.
//
// Expected format: "goroutine 123 [running]:..."
// Returns the numeric ID (123 in this example) or 0 if parsing fails.
//
// This function is optimized for minimal allocations:
//   - No string conversion
//   - No regex
//   - Direct byte parsing
//
// Parameters:
//   - buf: Stack trace bytes from runtime.Stack
//
// Returns:
//   - int64: Parsed goroutine ID, or 0 if format is invalid
func parseGID(buf []byte) int64 {
	// Expected prefix: "goroutine "
	const prefix = "goroutine "
	const prefixLen = 10 // len("goroutine ")

	// Verify buffer has expected prefix.
	if len(buf) < prefixLen {
		return 0
	}

	// Fast prefix check (uses string conversion but avoids regex).
	// Safe: we already verified len(buf) >= prefixLen above.
	if string(buf[:prefixLen]) != prefix {
		return 0
	}

	// Parse numeric goroutine ID.
	// Format after prefix: "123 [running]:..."
	var gid int64
	for i := prefixLen; i < len(buf); i++ {
		//nolint:gosec // G602: i is always < len(buf) due to loop condition
		c := buf[i]
		if c >= '0' && c <= '9' {
			gid = gid*10 + int64(c-'0')
		} else {
			// Non-digit terminates the ID (usually space before "[running]")
			break
		}
	}

	return gid
}
