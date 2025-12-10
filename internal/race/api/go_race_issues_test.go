// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains issue regression tests.
package api

import (
	"sync"
	"testing"
	"unsafe"
)

// TestGoRace_Issue12664 - global variable concurrent access.
// Based on https://github.com/golang/go/issues/12664
func TestGoRace_Issue12664(t *testing.T) {
	Init()
	defer Fini()

	var globalStr int // Represent string as int
	c := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&globalStr), true) // globalStr = "bye"
		RaceRelease(mu1Addr)
		close(c)
	}()

	// Read with different mutex (RACE!)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&globalStr), false) // print(globalStr)
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: global variable with different mutexes")
	}
}

// TestGoRace_Issue12664_InterfaceAssign - interface assignment race.
func TestGoRace_Issue12664_InterfaceAssign(t *testing.T) {
	Init()
	defer Fini()

	var globalInt int
	c := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&globalInt), true) // globalInt = 1
		RaceRelease(mu1Addr)
		close(c)
	}()

	// Read global for interface conversion (different mutex - RACE!)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&globalInt), false) // var i MyI = globalInt
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: interface assignment")
	}
}

// TestGoRace_Issue12664_TypeAssertion - type assertion race.
func TestGoRace_Issue12664_TypeAssertion(t *testing.T) {
	Init()
	defer Fini()

	var globalInt int
	c := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&globalInt), true) // globalInt = 1
		RaceRelease(mu1Addr)
		close(c)
	}()

	// Type assertion reads global (different mutex - RACE!)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&globalInt), false) // globalInt = i.(MyT)
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: type assertion")
	}
}

// TestGoNoRace_IOSync - I/O operations provide sync.
func TestGoNoRace_IOSync(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&x), true) // x = 42
		RaceRelease(muAddr)
		done <- true // I/O operation (channel send)
	}()

	<-done // I/O operation (channel receive) provides happens-before
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&x), false) // read x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: I/O sync through channel")
	}
}

// TestGoNoRace_HTTPHandlerSync - HTTP handler sync.
func TestGoNoRace_HTTPHandlerSync(t *testing.T) {
	Init()
	defer Fini()

	var handlerData int
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	// Simulate HTTP request handler
	handler := func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&handlerData), true) // handlerData++
		RaceRelease(muAddr)
	}

	// Multiple handler calls serialized by mutex
	for i := 0; i < 3; i++ {
		handler()
	}

	if RacesDetected() > 0 {
		t.Errorf("False positive: handler with mutex")
	}
}

// TestGoRace_MapConcurrentReadWrite - map concurrent read/write.
func TestGoRace_MapConcurrentReadWrite(t *testing.T) {
	Init()
	defer Fini()

	var mapVal int
	c := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&mapVal), true) // m["a"]["x"] = "y"
		RaceRelease(mu1Addr)
		close(c)
	}()

	// Read map value (different mutex - RACE!)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&mapVal), false) // read m["a"]["b"]
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: map read/write with different mutexes")
	}
}

// TestGoRace_RangeLoopVariable - race on range loop variable.
func TestGoRace_RangeLoopVariable(t *testing.T) {
	Init()
	defer Fini()

	var sharedIdx, sharedVal int
	done := make(chan bool, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	// Simulate range loop with shared i, v
	for iter := 0; iter < 2; iter++ {
		go func(iterNum int) {
			if iterNum == 0 {
				RaceAcquire(mu1Addr)
				simulateAccess(addrOf(&sharedIdx), true)  // write shared i
				simulateAccess(addrOf(&sharedVal), false) // read shared v
				RaceRelease(mu1Addr)
			} else {
				RaceAcquire(mu2Addr)
				simulateAccess(addrOf(&sharedVal), true) // write shared v
				RaceRelease(mu2Addr)
			}
			done <- true
		}(iter)
	}
	<-done
	<-done

	if RacesDetected() == 0 {
		t.Errorf("Expected race: range loop variable")
	}
}

// TestGoRace_CompoundAssignment - race on compound assignment.
func TestGoRace_CompoundAssignment(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // read x
		simulateAccess(addrOf(&x), true)  // x += 1
		RaceRelease(mu1Addr)
		ch <- true
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x), false) // read x
		simulateAccess(addrOf(&x), true)  // x += 2
		RaceRelease(mu2Addr)
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: compound assignment with different mutexes")
	}
}

// TestGoNoRace_CompoundAssignmentSync - compound assignment with sync.
func TestGoNoRace_CompoundAssignmentSync(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&x), false) // read x
		simulateAccess(addrOf(&x), true)  // x += 1
		RaceRelease(muAddr)
		done <- true
	}()

	<-done // Wait for first goroutine - happens-before!

	RaceAcquire(muAddr)
	simulateAccess(addrOf(&x), false) // read x
	simulateAccess(addrOf(&x), true)  // x += 2
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: compound assignment with same mutex")
	}
}

// TestGoRace_BitwiseOr - race on bitwise OR.
func TestGoRace_BitwiseOr(t *testing.T) {
	Init()
	defer Fini()

	var x, y, z int
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), true)
		simulateAccess(addrOf(&y), false) // x = y | z
		simulateAccess(addrOf(&z), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&y), true) // y = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: bitwise OR operation")
	}
}

// TestGoRace_BitwiseXor - race on bitwise XOR.
func TestGoRace_BitwiseXor(t *testing.T) {
	Init()
	defer Fini()

	var x, y, z int
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), true)
		simulateAccess(addrOf(&y), false) // x = y ^ z
		simulateAccess(addrOf(&z), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&y), true) // y = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: bitwise XOR operation")
	}
}

// TestGoRace_LogicalAnd - race on logical AND.
func TestGoRace_LogicalAnd(t *testing.T) {
	Init()
	defer Fini()

	var a, b int // Use int to represent bool
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), false) // if a && b
		simulateAccess(addrOf(&b), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), true) // a = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: logical AND operation")
	}
}

// TestGoRace_LogicalOr - race on logical OR.
func TestGoRace_LogicalOr(t *testing.T) {
	Init()
	defer Fini()

	var a, b int // Use int to represent bool
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), false) // if a || b
		simulateAccess(addrOf(&b), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), true) // a = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: logical OR operation")
	}
}

// TestGoRace_CompareEqual - race on equality comparison.
func TestGoRace_CompareEqual(t *testing.T) {
	Init()
	defer Fini()

	var x, y int
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // if x == y
		simulateAccess(addrOf(&y), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x), true) // x = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: equality comparison")
	}
}

// TestGoRace_CompareLess - race on less-than comparison.
func TestGoRace_CompareLess(t *testing.T) {
	Init()
	defer Fini()

	var x, y int
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // if x < y
		simulateAccess(addrOf(&y), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x), true) // x = 1
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: less-than comparison")
	}
}

// TestGoRace_DereferenceWrite - race on pointer dereference write.
func TestGoRace_DereferenceWrite(t *testing.T) {
	Init()
	defer Fini()

	var x, y int
	ch := make(chan int, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), true) // *p = y (where p = &x)
		simulateAccess(addrOf(&y), false)
		RaceRelease(mu1Addr)
		ch <- 1
	}()
	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x), false) // read *p
		RaceRelease(mu2Addr)
		ch <- 1
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: pointer dereference write")
	}
}

// TestGoNoRace_DereferenceSync - pointer dereference with sync.
func TestGoNoRace_DereferenceSync(t *testing.T) {
	Init()
	defer Fini()

	var x, y int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&x), true) // *p = y (where p = &x)
		simulateAccess(addrOf(&y), false)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done // Wait for first goroutine - happens-before!

	RaceAcquire(muAddr)
	simulateAccess(addrOf(&x), false) // read *p
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: pointer dereference with same mutex")
	}
}
