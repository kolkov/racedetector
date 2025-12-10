// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.23,!go1.26,arm64

// Assembly implementation for fast goroutine ID extraction on arm64.
//
// This file provides direct access to the Go runtime's g struct pointer
// via the dedicated g register (R28). On arm64, Go reserves R28 specifically
// for the current goroutine pointer, making access extremely efficient.
//
// Performance: ~1-2ns per call (vs ~1500ns for runtime.Stack parsing).
//
// Go Version Support:
//   - Go 1.23: Supported (offset verified)
//   - Go 1.24: Supported (offset verified)
//   - Go 1.25: Supported (offset verified)
//   - Go 1.26+: Requires verification of g struct layout
//
// Architecture: arm64 (AArch64) only.
// For amd64 and other architectures, see goid_amd64.s and goid_fallback.go.

#include "textflag.h"

// func getg() uintptr
//
// Returns the current goroutine's g struct pointer from the g register.
//
// Implementation Details:
//   - On arm64, Go runtime reserves R28 for the g pointer
//   - The assembler uses 'g' as an alias for R28
//   - No TLS access needed - direct register read
//
// Flags:
//   NOSPLIT - Critical: prevents stack growth during execution.
//             Race detector hot path must not trigger stack splits.
//
// Register Usage:
//   g (R28) - Go runtime's dedicated goroutine pointer register
//   R0      - Return value (first result register on arm64)
//
// Returns:
//   ret+0(FP) - g pointer as uintptr (8 bytes)
//
TEXT ·getg(SB),NOSPLIT,$0-8
	// On arm64, the g pointer is stored in a dedicated register.
	// The Go assembler uses 'g' as an alias for R28.
	// This is the fastest possible access - just a register move.
	MOVD g, R0

	// Store result in return slot.
	MOVD R0, ret+0(FP)

	RET
