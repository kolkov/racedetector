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
// STRING OPERATIONS (10 tests)
// =============================================================================

// TestGoRace_StringAssignment tests concurrent string assignment.
func TestGoRace_StringAssignment(t *testing.T) {
	Init()
	defer Fini()

	var s string
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
		s = "goroutine1"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		s = "goroutine2"
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent string assignment")
	}
}

// TestGoNoRace_StringAssignment tests synchronized string assignment.
func TestGoNoRace_StringAssignment(t *testing.T) {
	Init()
	defer Fini()

	var s string
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		s = "goroutine"
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = s
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized string assignment")
	}
}

// TestGoRace_StringConcat tests concurrent string concatenation.
func TestGoRace_StringConcat(t *testing.T) {
	Init()
	defer Fini()

	s := "base"
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
		s += "1"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s += "2"
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent string concatenation")
	}
}

// TestGoNoRace_StringConcatSync tests synchronized string concatenation.
func TestGoNoRace_StringConcatSync(t *testing.T) {
	Init()
	defer Fini()

	s := "base"
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(sAddr)
		RaceRead(sAddr)
		s += "suffix"
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(sAddr)
	_ = s
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized string concat")
	}
}

// TestGoRace_StringComparison tests concurrent string comparison with write.
func TestGoRace_StringComparison(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
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
		s = "modified"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(sAddr)
		_ = (s == "test")
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on string comparison with concurrent write")
	}
}

// TestGoNoRace_StringComparison tests synchronized string comparison.
func TestGoNoRace_StringComparison(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(sAddr)
		_ = (s == "test")
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(sAddr)
	s = "modified"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized string comparison")
	}
}

// TestGoRace_StringLength tests concurrent string length read with write.
func TestGoRace_StringLength(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
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
		s = "modified_longer"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(sAddr)
		_ = len(s)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on string length with concurrent write")
	}
}

// TestGoNoRace_StringLength tests synchronized string length.
func TestGoNoRace_StringLength(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(sAddr)
		_ = len(s)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(sAddr)
	s = "modified"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized string length")
	}
}

// TestGoRace_StringIndex tests concurrent string indexing with write.
func TestGoRace_StringIndex(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
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
		s = "new"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(sAddr)
		_ = s[0]
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on string indexing with concurrent write")
	}
}

// TestGoNoRace_StringIndex tests synchronized string indexing.
func TestGoNoRace_StringIndex(t *testing.T) {
	Init()
	defer Fini()

	s := "test"
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	sAddr := uintptr(unsafe.Pointer(&s))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(sAddr)
		_ = s[0]
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(sAddr)
	s = "new"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized string indexing")
	}
}
