// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains comparison operation race tests.
// These tests are ported from Go 1.25.3 runtime/race/output_test.go.
// They test race detection in non-inlined comparison operations.
package api

import (
	"sync"
	"testing"
	"unsafe"
)

// =============================================================================
// NON-INLINE COMPARISON OPERATIONS (Go 1.25.3 additions)
// =============================================================================
// These test scenarios were added in Go 1.25+ to verify race detection
// in comparison operations that are too large to inline.

// TestGoRace_NonInlineArrayCompare tests race detection when comparing
// large arrays. Go cannot inline comparison of arrays >= 1024 bytes,
// so the comparison becomes a function call that must be instrumented.
func TestGoRace_NonInlineArrayCompare(t *testing.T) {
	Init()
	defer Fini()

	// Large array (1024 bytes) - comparison cannot be inlined.
	var x [1024]byte
	xAddr := uintptr(unsafe.Pointer(&x[0]))

	started := make(chan struct{})
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		close(started)
		// One goroutine reads x via comparison.
		var y [1024]byte
		RaceAcquire(mu1Addr)
		// Reading all bytes of x for comparison.
		for i := 0; i < len(x); i++ {
			RaceRead(xAddr + uintptr(i))
		}
		_ = x == y // This comparison reads x.
		RaceRelease(mu1Addr)
		ch <- true
	}()

	<-started

	// Main goroutine writes to x.
	RaceAcquire(mu2Addr)
	idx := 512 // Middle of array.
	RaceWrite(xAddr + uintptr(idx))
	x[idx]++
	RaceRelease(mu2Addr)

	<-ch

	// Race expected: concurrent read (comparison) and write.
	if RacesDetected() == 0 {
		t.Errorf("Expected race on non-inline array comparison")
	}
}

// TestGoNoRace_NonInlineArrayCompare tests synchronized large array comparison.
func TestGoNoRace_NonInlineArrayCompare(t *testing.T) {
	Init()
	defer Fini()

	var x [1024]byte
	xAddr := uintptr(unsafe.Pointer(&x[0]))

	ready := make(chan struct{})
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		<-ready
		// Then: compare under same lock (happens-after write).
		var y [1024]byte
		RaceAcquire(muAddr)
		RaceRead(xAddr)
		_ = x == y
		RaceRelease(muAddr)
		done <- true
	}()

	// First: write under lock.
	RaceAcquire(muAddr)
	RaceWrite(xAddr)
	x[0] = 42
	RaceRelease(muAddr)

	close(ready) // Signal goroutine to proceed.
	<-done

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized non-inline array comparison")
	}
}

// LargeStruct contains a large array field for testing struct comparison.
type LargeStruct struct {
	a [1024]byte
}

// TestGoRace_NonInlineStructCompare tests race detection when comparing
// large structs. Similar to arrays, structs with large fields cannot
// have their comparisons inlined.
func TestGoRace_NonInlineStructCompare(t *testing.T) {
	Init()
	defer Fini()

	var x LargeStruct
	xAddr := uintptr(unsafe.Pointer(&x.a[0]))

	started := make(chan struct{})
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		close(started)
		// One goroutine reads x via struct comparison.
		var y LargeStruct
		RaceAcquire(mu1Addr)
		// Reading all bytes of x.a for comparison.
		for i := 0; i < len(x.a); i++ {
			RaceRead(xAddr + uintptr(i))
		}
		_ = x == y // This comparison reads x.
		RaceRelease(mu1Addr)
		ch <- true
	}()

	<-started

	// Main goroutine writes to x.a.
	RaceAcquire(mu2Addr)
	idx := 512 // Middle of array.
	RaceWrite(xAddr + uintptr(idx))
	x.a[idx]++
	RaceRelease(mu2Addr)

	<-ch

	// Race expected: concurrent read (comparison) and write.
	if RacesDetected() == 0 {
		t.Errorf("Expected race on non-inline struct comparison")
	}
}

// TestGoNoRace_NonInlineStructCompare tests synchronized large struct comparison.
func TestGoNoRace_NonInlineStructCompare(t *testing.T) {
	Init()
	defer Fini()

	var x LargeStruct
	xAddr := uintptr(unsafe.Pointer(&x.a[0]))

	ready := make(chan struct{})
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		<-ready
		// Then: compare under same lock (happens-after write).
		var y LargeStruct
		RaceAcquire(muAddr)
		RaceRead(xAddr)
		_ = x == y
		RaceRelease(muAddr)
		done <- true
	}()

	// First: write under lock.
	RaceAcquire(muAddr)
	RaceWrite(xAddr)
	x.a[0] = 42
	RaceRelease(muAddr)

	close(ready) // Signal goroutine to proceed.
	<-done

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized non-inline struct comparison")
	}
}
