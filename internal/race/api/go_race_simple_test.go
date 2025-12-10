// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains basic race tests demonstrating known limitations.
package api

import (
	"runtime"
	"testing"
	"time"
)

// =============================================================================
// BASIC RACE TESTS (Known Limitations)
// =============================================================================

// TestGoRace_SimpleWriteWrite - Two goroutines write to same variable without sync.
// This tests the most basic race condition: two concurrent writes without any
// synchronization primitives.
// KNOWN LIMITATION: Accesses without Acquire/Release scope are not properly tracked.
func TestGoRace_SimpleWriteWrite(t *testing.T) {
	t.Skip("KNOWN LIMITATION: unsynchronized accesses without Acquire/Release scope not tracked (see v0.6.0 roadmap)")
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)
	done := make(chan bool, 2)
	start := make(chan struct{}) // Barrier to ensure concurrent access

	go func() {
		<-start                    // Wait for start signal
		simulateAccess(addr, true) // write
		done <- true
	}()

	go func() {
		<-start                    // Wait for start signal
		simulateAccess(addr, true) // write (race!)
		done <- true
	}()

	// Small delay to ensure goroutines are waiting on the channel
	time.Sleep(time.Millisecond)
	close(start) // Start both goroutines simultaneously

	<-done
	<-done

	races := RacesDetected()
	if races == 0 {
		t.Errorf("False negative: failed to detect write-write race (races=%d)", races)
	}
}

// TestGoRace_ReadWriteRace - Read-write race without synchronization.
// KNOWN LIMITATION: Accesses without Acquire/Release scope are not properly tracked.
func TestGoRace_ReadWriteRace(t *testing.T) {
	t.Skip("KNOWN LIMITATION: unsynchronized accesses without Acquire/Release scope not tracked (see v0.6.0 roadmap)")
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)
	done := make(chan bool, 2)
	start := make(chan struct{}) // Barrier to ensure concurrent access

	go func() {
		<-start                     // Wait for start signal
		runtime.Gosched()           // Increase chance of concurrent execution
		simulateAccess(addr, false) // read
		done <- true
	}()

	go func() {
		<-start                    // Wait for start signal
		runtime.Gosched()          // Increase chance of concurrent execution
		simulateAccess(addr, true) // write (race with read!)
		done <- true
	}()

	// Small delay to ensure goroutines are waiting on the channel
	time.Sleep(time.Millisecond)
	close(start) // Start both goroutines simultaneously

	<-done
	<-done

	races := RacesDetected()
	if races == 0 {
		t.Errorf("False negative: failed to detect read-write race (races=%d)", races)
	}
}

// TestGoNoRace_ReadRead - Concurrent reads are safe.
func TestGoNoRace_ReadRead(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)
	done := make(chan bool, 2)

	// Initialize x first
	simulateAccess(addr, true)

	// Small delay to ensure first access is recorded
	time.Sleep(time.Millisecond)

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addr, false) // read
		done <- true
	}()

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addr, false) // read
		done <- true
	}()

	<-done
	<-done

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in concurrent reads")
	}
}
