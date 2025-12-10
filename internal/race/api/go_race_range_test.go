// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

package api

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

// =============================================================================
// RANGE AND LOOP OPERATIONS (12 tests)
// =============================================================================

// TestGoRace_RangeSlice tests concurrent range over slice with write.
func TestGoRace_RangeSlice(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2, 3}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	elem0Addr := uintptr(unsafe.Pointer(&s[0]))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(elem0Addr)
		s[0] = 10
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		for i := range s {
			RaceRead(uintptr(unsafe.Pointer(&s[i])))
			_ = s[i]
		}
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on range over slice with concurrent write")
	}
}

// TestGoNoRace_RangeSliceIteration tests synchronized range over slice.
func TestGoNoRace_RangeSliceIteration(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2, 3}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		for _, v := range s {
			_ = v
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(uintptr(unsafe.Pointer(&s[0])))
	s[0] = 10
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized range over slice")
	}
}

// TestGoRace_RangeMap tests concurrent range over map with write.
func TestGoRace_RangeMap(t *testing.T) {
	Init()
	defer Fini()

	m := map[string]int{"a": 1, "b": 2}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	mAddr := uintptr(unsafe.Pointer(&m))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(mAddr)
		m["c"] = 3
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(mAddr)
		for k, v := range m {
			_, _ = k, v
		}
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on range over map with concurrent write")
	}
}

// TestGoNoRace_RangeMap tests synchronized range over map.
func TestGoNoRace_RangeMap(t *testing.T) {
	Init()
	defer Fini()

	m := map[string]int{"a": 1, "b": 2}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	mAddr := uintptr(unsafe.Pointer(&m))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(mAddr)
		for k, v := range m {
			_, _ = k, v
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(mAddr)
	m["c"] = 3
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized range over map")
	}
}

// TestGoRace_RangeChannel tests concurrent range over channel.
func TestGoRace_RangeChannel(t *testing.T) {
	Init()
	defer Fini()

	var result int
	ch := make(chan int, 3)
	done := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	resultAddr := uintptr(unsafe.Pointer(&result))

	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		for v := range ch {
			RaceWrite(resultAddr)
			result += v
		}
		RaceRelease(mu1Addr)
		done <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(resultAddr)
		result = 100
		RaceRelease(mu2Addr)
		done <- true
	}()

	close(start)
	<-done
	<-done

	if RacesDetected() == 0 {
		t.Errorf("Expected race on range over channel with concurrent write")
	}
}

// TestGoNoRace_RangeChannel tests synchronized range over channel.
func TestGoNoRace_RangeChannel(t *testing.T) {
	Init()
	defer Fini()

	var result int
	ch := make(chan int, 3)
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	resultAddr := uintptr(unsafe.Pointer(&result))

	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	go func() {
		RaceAcquire(muAddr)
		for v := range ch {
			RaceWrite(resultAddr)
			result += v
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(resultAddr)
	_ = result
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized range over channel")
	}
}

// TestGoRace_ForLoopVariable tests concurrent access to loop variable.
func TestGoRace_ForLoopVariable(t *testing.T) {
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
		<-start
		RaceAcquire(mu1Addr)
		for i := 0; i < 3; i++ {
			RaceWrite(xAddr)
			x = i
		}
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(xAddr)
		x = 10
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on loop variable with concurrent write")
	}
}

// TestGoNoRace_ForLoopVariable tests synchronized loop variable.
func TestGoNoRace_ForLoopVariable(t *testing.T) {
	Init()
	defer Fini()

	var x int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	xAddr := uintptr(unsafe.Pointer(&x))

	go func() {
		RaceAcquire(muAddr)
		for i := 0; i < 3; i++ {
			RaceWrite(xAddr)
			x = i
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(xAddr)
	_ = x
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized loop variable")
	}
}

// TestGoRace_ForLoopCondition tests concurrent access in loop condition.
func TestGoRace_ForLoopCondition(t *testing.T) {
	Init()
	defer Fini()

	limit := 10
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	limitAddr := uintptr(unsafe.Pointer(&limit))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		for i := 0; i < limit; i++ {
			RaceRead(limitAddr)
			_ = i
		}
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(limitAddr)
		limit = 20
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on loop condition with concurrent write")
	}
}

// TestGoNoRace_ForLoopCondition tests synchronized loop condition.
func TestGoNoRace_ForLoopCondition(t *testing.T) {
	Init()
	defer Fini()

	limit := 10
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	limitAddr := uintptr(unsafe.Pointer(&limit))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(limitAddr)
		for i := 0; i < limit; i++ {
			_ = i
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(limitAddr)
	limit = 20
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized loop condition")
	}
}

// TestGoRace_RangeString tests concurrent range over string with write.
func TestGoRace_RangeString(t *testing.T) {
	Init()
	defer Fini()

	s := "hello"
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		runtime.Gosched() // Increase chance of concurrent execution.
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		s = "world"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		runtime.Gosched() // Increase chance of concurrent execution.
		RaceAcquire(mu2Addr)
		RaceRead(sAddr)
		for _, r := range s {
			_ = r
		}
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on range over string with concurrent write")
	}
}

// TestGoNoRace_RangeString tests synchronized range over string.
func TestGoNoRace_RangeString(t *testing.T) {
	Init()
	defer Fini()

	s := "hello"
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(sAddr)
		for _, r := range s {
			_ = r
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(sAddr)
	s = "world"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized range over string")
	}
}
