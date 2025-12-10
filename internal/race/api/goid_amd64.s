// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.23,!go1.26,amd64

// Assembly implementation for fast goroutine ID extraction on amd64.
//
// This file provides direct access to the Go runtime's g struct pointer
// via Thread Local Storage (TLS). The g pointer is then used to extract
// the goid field at a known offset.
//
// Performance: ~1-2ns per call (vs ~1500ns for runtime.Stack parsing).
//
// Go Version Support:
//   - Go 1.23: Supported (offset verified)
//   - Go 1.24: Supported (offset verified)
//   - Go 1.25: Supported (offset verified)
//   - Go 1.26+: Requires verification of g struct layout
//
// Architecture: amd64 (x86-64) only.
// For arm64 and other architectures, see goid_arm64.s and goid_fallback.go.

#include "textflag.h"

// func getg() uintptr
//
// Returns the current goroutine's g struct pointer from TLS as uintptr.
//
// Implementation Details:
//   - On amd64, Go runtime stores g pointer in TLS
//   - TLS is accessed via the pseudo-register (TLS)
//   - The g pointer is at offset 0 from TLS base
//
// Flags:
//   NOSPLIT - Critical: prevents stack growth during execution.
//             Race detector hot path must not trigger stack splits.
//
// Register Usage:
//   R14 - Temporary for g pointer (callee-saved, safe to use)
//
// Returns:
//   ret+0(FP) - g pointer as uintptr (8 bytes)
//
TEXT ·getg(SB),NOSPLIT,$0-8
	// Load g pointer from Thread Local Storage.
	// (TLS) is a pseudo-register that Go assembler translates to
	// appropriate TLS access for the platform (FS segment on Linux/macOS,
	// TEB on Windows).
	MOVQ (TLS), R14

	// Store result in return slot.
	MOVQ R14, ret+0(FP)

	RET
