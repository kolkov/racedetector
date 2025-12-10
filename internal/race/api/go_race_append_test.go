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
// APPEND OPERATIONS (10 tests)
// =============================================================================

// TestGoRace_SliceAppend tests concurrent slice append.
func TestGoRace_SliceAppend(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2, 3}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 4)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 5)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent slice append")
	}
}

// TestGoNoRace_SliceAppend tests synchronized slice append.
func TestGoNoRace_SliceAppend(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2, 3}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 4)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(sAddr)
	RaceRead(sAddr)
	s = append(s, 5)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized slice append")
	}
}

// TestGoRace_AppendMultiple tests concurrent append with multiple elements.
func TestGoRace_AppendMultiple(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 2, 3, 4)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 5, 6, 7)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent append multiple")
	}
}

// TestGoNoRace_AppendMultiple tests synchronized append with multiple elements.
func TestGoNoRace_AppendMultiple(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 2, 3, 4)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = s[0]
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized append multiple")
	}
}

// TestGoRace_AppendSliceToSlice tests concurrent append of slice to slice.
func TestGoRace_AppendSliceToSlice(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, []int{3, 4}...)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, []int{5, 6}...)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent append slice to slice")
	}
}

// TestGoNoRace_AppendSliceToSlice tests synchronized append of slice to slice.
func TestGoNoRace_AppendSliceToSlice(t *testing.T) {
	Init()
	defer Fini()

	s := []int{1, 2}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, []int{3, 4}...)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = len(s)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized append slice to slice")
	}
}

// TestGoRace_AppendCapGrow tests concurrent append causing capacity growth.
func TestGoRace_AppendCapGrow(t *testing.T) {
	Init()
	defer Fini()

	s := make([]int, 0, 2)
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 1, 2, 3) // Will grow capacity
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 4, 5, 6) // Will grow capacity
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent append with capacity growth")
	}
}

// TestGoNoRace_AppendCapGrow tests synchronized append with capacity growth.
func TestGoNoRace_AppendCapGrow(t *testing.T) {
	Init()
	defer Fini()

	s := make([]int, 0, 2)
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s = append(s, 1, 2, 3)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = cap(s)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized append with capacity growth")
	}
}

// TestGoRace_AppendEmpty tests concurrent append to empty slice.
func TestGoRace_AppendEmpty(t *testing.T) {
	Init()
	defer Fini()

	var s []int
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(sAddr)
		s = append(s, 1)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		s = append(s, 2)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent append to empty slice")
	}
}

// TestGoNoRace_AppendEmpty tests synchronized append to empty slice.
func TestGoNoRace_AppendEmpty(t *testing.T) {
	Init()
	defer Fini()

	var s []int
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		s = append(s, 1)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = len(s)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized append to empty slice")
	}
}
