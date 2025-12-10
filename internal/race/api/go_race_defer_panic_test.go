// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

package api

import (
	"sync"
	"testing"
	"unsafe"
)

// =============================================================================
// DEFER AND PANIC OPERATIONS (10 tests)
// =============================================================================

// TestGoRace_DeferVariable tests concurrent access to variable in defer.
func TestGoRace_DeferVariable(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(mu1Addr)
			RaceWrite(xAddr)
			x = 1
			RaceRelease(mu1Addr)
			ch <- true
		}()
		<-start
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		x = 2
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent access in defer")
	}
}

// TestGoNoRace_DeferVariable tests synchronized variable in defer.
func TestGoNoRace_DeferVariable(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(muAddr)
			RaceWrite(xAddr)
			x = 1
			RaceRelease(muAddr)
			done <- true
		}()
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized defer variable")
	}
}

// TestGoRace_DeferClosure tests concurrent access in deferred closure.
func TestGoRace_DeferClosure(t *testing.T) {
	Init()
	defer Fini()

	x := 0
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(mu1Addr)
			RaceWrite(xAddr)
			x++
			RaceRelease(mu1Addr)
			ch <- true
		}()
		<-start
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		x++
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent access in deferred closure")
	}
}

// TestGoNoRace_DeferClosure tests synchronized deferred closure.
func TestGoNoRace_DeferClosure(t *testing.T) {
	Init()
	defer Fini()

	x := 0
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(muAddr)
			RaceWrite(xAddr)
			x++
			RaceRelease(muAddr)
			done <- true
		}()
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized deferred closure")
	}
}

// TestGoRace_PanicRecover tests concurrent access during panic/recover.
func TestGoRace_PanicRecover(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				RaceAcquire(mu1Addr)
				RaceWrite(xAddr)
				x = 1
				RaceRelease(mu1Addr)
			}
			ch <- true
		}()
		<-start
		panic("test panic")
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		x = 2
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race during panic/recover")
	}
}

// TestGoNoRace_PanicRecover tests synchronized panic/recover.
func TestGoNoRace_PanicRecover(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				RaceAcquire(muAddr)
				RaceWrite(xAddr)
				x = 1
				RaceRelease(muAddr)
			}
			done <- true
		}()
		panic("test panic")
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized panic/recover")
	}
}

// TestGoRace_DeferOrder tests concurrent access in multiple defers.
func TestGoRace_DeferOrder(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(mu1Addr)
			RaceWrite(xAddr)
			x = 1
			RaceRelease(mu1Addr)
		}()
		defer func() {
			RaceAcquire(mu1Addr)
			RaceWrite(xAddr)
			x = 2
			RaceRelease(mu1Addr)
			ch <- true
		}()
		<-start
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		x = 3
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race with multiple defers")
	}
}

// TestGoNoRace_DeferOrder tests synchronized multiple defers.
func TestGoNoRace_DeferOrder(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		defer func() {
			RaceAcquire(muAddr)
			RaceWrite(xAddr)
			x = 1
			RaceRelease(muAddr)
			done <- true // Signal AFTER all defers complete (LIFO order)
		}()
		defer func() {
			RaceAcquire(muAddr)
			RaceWrite(xAddr)
			x = 2
			RaceRelease(muAddr)
		}()
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized multiple defers")
	}
}

// TestGoRace_DeferPointer tests concurrent pointer access in defer.
func TestGoRace_DeferPointer(t *testing.T) {
	Init()
	defer Fini()

	x := new(int)
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	xAddr := uintptr(unsafe.Pointer(x))

	go func() {
		defer func() {
			RaceAcquire(mu1Addr)
			RaceWrite(xAddr)
			*x = 1
			RaceRelease(mu1Addr)
			ch <- true
		}()
		<-start
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		*x = 2
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on pointer in defer")
	}
}

// TestGoNoRace_DeferPointer tests synchronized pointer in defer.
func TestGoNoRace_DeferPointer(t *testing.T) {
	Init()
	defer Fini()

	x := new(int)
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(x))

	go func() {
		defer func() {
			RaceAcquire(muAddr)
			RaceWrite(xAddr)
			*x = 1
			RaceRelease(muAddr)
			done <- true
		}()
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = *x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized pointer in defer")
	}
}
