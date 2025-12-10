// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

package api

import (
	"sync"
	"testing"
	"time"
)

// TestGoStart_BasicInheritance verifies that child goroutine inherits
// parent's VectorClock and doesn't report false positive.
func TestGoStart_BasicInheritance(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)

	// Main writes to x.
	simulateAccess(addr, true) // write

	// Simulate GoStart: capture parent clock before spawn.
	RaceGoStart(0)

	done := make(chan bool)
	go func() {
		// Child reads x - should NOT be a race because
		// parent's write happened-before child's read via GoStart.
		simulateAccess(addr, false) // read
		RaceGoEnd()
		done <- true
	}()
	<-done

	if RacesDetected() > 0 {
		t.Errorf("False positive: GoStart should establish HB edge, got %d races",
			RacesDetected())
	}
}

// TestGoStart_NoFalseNegative verifies that actual races are still detected.
func TestGoStart_NoFalseNegative(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)
	done := make(chan bool)
	start := make(chan struct{})

	// Spawn child first with GoStart.
	RaceGoStart(0)
	go func() {
		<-start                    // Wait for start signal
		simulateAccess(addr, true) // Child write
		RaceGoEnd()
		done <- true
	}()

	// Small delay to ensure goroutine is waiting
	time.Sleep(time.Millisecond)
	close(start) // Release child

	// Main writes concurrently - this IS a race!
	simulateAccess(addr, true) // Main write
	<-done

	if RacesDetected() == 0 {
		t.Error("False negative: should detect concurrent writes")
	}
}

// TestGoStart_WriteAfterFork verifies that writes AFTER fork are not
// visible to child (they race).
func TestGoStart_WriteAfterFork(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)
	done := make(chan bool)
	start := make(chan struct{})

	// Parent calls GoStart (captures clock BEFORE the write below)
	RaceGoStart(0)

	go func() {
		<-start
		// Child reads x - this RACES with parent's write below
		// because parent wrote AFTER calling GoStart.
		simulateAccess(addr, false) // read
		RaceGoEnd()
		done <- true
	}()

	// Parent writes AFTER GoStart - child should NOT see this!
	time.Sleep(time.Millisecond)
	close(start)
	simulateAccess(addr, true) // write after fork
	<-done

	if RacesDetected() == 0 {
		t.Error("False negative: write after GoStart should race with child read")
	}
}

// TestGoStart_MultipleChildren verifies multiple children get separate clocks.
func TestGoStart_MultipleChildren(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addrX := addrOf(&x)

	// Main writes x
	simulateAccess(addrX, true)

	var wg sync.WaitGroup
	wg.Add(2)

	// Spawn child 1
	RaceGoStart(0)
	go func() {
		simulateAccess(addrX, false) // Child1 reads x - OK (HB from main via GoStart)
		RaceGoEnd()
		wg.Done()
	}()

	// Spawn child 2
	RaceGoStart(0)
	go func() {
		simulateAccess(addrX, false) // Child2 reads x - OK (HB from main via GoStart)
		RaceGoEnd()
		wg.Done()
	}()

	wg.Wait()

	// Read-read is safe, so no races expected
	if RacesDetected() > 0 {
		t.Errorf("False positive: multiple reads should not race, got %d", RacesDetected())
	}
}

// TestGoStart_ChainedSpawns verifies transitive HB through spawn chains.
func TestGoStart_ChainedSpawns(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := addrOf(&x)

	// Main writes x
	simulateAccess(addr, true) // clock {1:1}

	done1 := make(chan bool)
	done2 := make(chan bool)

	// Main spawns Child1
	RaceGoStart(0)
	go func() {
		// Child1 inherits main's clock, now {1:1, 2:1}

		// Child1 spawns Child2
		RaceGoStart(0)
		go func() {
			// Child2 inherits {1:1, 2:1}, now {1:1, 2:1, 3:1}
			simulateAccess(addr, false) // Read x - should see main's write
			RaceGoEnd()
			done2 <- true
		}()
		<-done2
		RaceGoEnd()
		done1 <- true
	}()
	<-done1

	if RacesDetected() > 0 {
		t.Errorf("False positive: chained spawns should establish transitive HB, got %d", RacesDetected())
	}
}

// TestGoStart_SpawnContextExpiry verifies TTL-based cleanup of spawn contexts.
func TestGoStart_SpawnContextExpiry(t *testing.T) {
	Init()
	defer Fini()

	// Create spawn context that will expire
	RaceGoStart(0)

	// Wait for TTL to expire (100ms + buffer)
	time.Sleep(150 * time.Millisecond)

	// Try to consume - should return nil (expired)
	clock := findAndConsumeSpawnContext()
	if clock != nil {
		t.Error("Spawn context should have expired after TTL")
	}
}

// TestGoStart_SpawnContextConsumption verifies context is consumed only once.
func TestGoStart_SpawnContextConsumption(t *testing.T) {
	Init()
	defer Fini()

	// Create one spawn context
	RaceGoStart(0)

	// First consumer gets the context
	clock1 := findAndConsumeSpawnContext()
	if clock1 == nil {
		t.Fatal("First consumer should get spawn context")
	}

	// Second consumer gets nothing (already consumed)
	clock2 := findAndConsumeSpawnContext()
	if clock2 != nil {
		t.Error("Second consumer should NOT get spawn context (already consumed)")
	}
}
