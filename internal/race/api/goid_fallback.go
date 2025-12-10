// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !go1.23 || go1.26 || !(amd64 || arm64)

// Fallback goroutine ID extraction for unsupported platforms.
//
// This file provides the slow path for goroutine ID extraction when
// the assembly-optimized implementation cannot be used:
//
//   - Go versions < 1.23 (runtime.g layout not verified)
//   - Go versions >= 1.26 (runtime.g layout may have changed)
//   - Architectures other than amd64/arm64 (no assembly implementation)
//
// Performance: ~1500ns per call (runtime.Stack parsing).
//
// Supported platforms (fallback to this):
//   - 386, arm, ppc64, ppc64le, mips, mips64, mips64le
//   - riscv64, s390x, wasm, loong64
//   - Any architecture on Go < 1.23 or Go >= 1.26
//
// The fallback uses runtime.Stack() to get the current goroutine's stack
// trace, then parses the first line to extract the goroutine ID.
// Format: "goroutine 123 [running]:\n..."

package api

// getGoroutineIDFast is the fallback implementation for unsupported platforms.
//
// On platforms without assembly optimization, this function simply delegates
// to the slow path using runtime.Stack parsing. The name "Fast" is kept for
// API consistency across build configurations.
//
// Performance: ~1500ns per call (same as getGoroutineIDSlow).
//
// This function is used when:
//   - Running on unsupported architecture (not amd64/arm64)
//   - Running on unsupported Go version (< 1.23 or >= 1.26)
//
// Returns:
//   - int64: Goroutine ID (always positive, unique per goroutine)
func getGoroutineIDFast() int64 {
	return getGoroutineIDSlow()
}
