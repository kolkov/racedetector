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
// COMPLEX NUMBER OPERATIONS (8 tests)
// =============================================================================

// TestGoRace_Complex64Assignment tests concurrent complex64 assignment.
func TestGoRace_Complex64Assignment(t *testing.T) {
	Init()
	defer Fini()

	var c complex64
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(cAddr)
		c = 1 + 2i
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(cAddr)
		c = 3 + 4i
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent complex64 assignment")
	}
}

// TestGoNoRace_Complex64Assignment tests synchronized complex64 assignment.
func TestGoNoRace_Complex64Assignment(t *testing.T) {
	Init()
	defer Fini()

	var c complex64
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(cAddr)
		c = 1 + 2i
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(cAddr)
	_ = c
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized complex64 assignment")
	}
}

// TestGoRace_Complex128Assignment tests concurrent complex128 assignment.
func TestGoRace_Complex128Assignment(t *testing.T) {
	Init()
	defer Fini()

	var c complex128
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(cAddr)
		c = 1.5 + 2.5i
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(cAddr)
		c = 3.5 + 4.5i
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent complex128 assignment")
	}
}

// TestGoNoRace_Complex128Assignment tests synchronized complex128 assignment.
func TestGoNoRace_Complex128Assignment(t *testing.T) {
	Init()
	defer Fini()

	var c complex128
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(cAddr)
		c = 1.5 + 2.5i
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(cAddr)
	_ = c
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized complex128 assignment")
	}
}

// TestGoRace_ComplexArithmetic tests concurrent complex arithmetic.
func TestGoRace_ComplexArithmetic(t *testing.T) {
	Init()
	defer Fini()

	c := complex(1.0, 2.0)
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(cAddr)
		RaceRead(cAddr)
		c += complex(1, 0)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(cAddr)
		RaceRead(cAddr)
		c *= complex(2, 0)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent complex arithmetic")
	}
}

// TestGoNoRace_ComplexArithmetic tests synchronized complex arithmetic.
func TestGoNoRace_ComplexArithmetic(t *testing.T) {
	Init()
	defer Fini()

	c := complex(1.0, 2.0)
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(cAddr)
		RaceRead(cAddr)
		c += complex(1, 0)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(cAddr)
	_ = c
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized complex arithmetic")
	}
}

// TestGoRace_ComplexRealImag tests concurrent real/imag access.
func TestGoRace_ComplexRealImag(t *testing.T) {
	Init()
	defer Fini()

	c := complex(1.0, 2.0)
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(cAddr)
		c = complex(5, 6)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(cAddr)
		_ = real(c)
		_ = imag(c)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on real/imag with concurrent write")
	}
}

// TestGoNoRace_ComplexRealImag tests synchronized real/imag access.
func TestGoNoRace_ComplexRealImag(t *testing.T) {
	Init()
	defer Fini()

	c := complex(1.0, 2.0)
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	cAddr := uintptr(unsafe.Pointer(&c))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(cAddr)
		_ = real(c)
		_ = imag(c)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(cAddr)
	c = complex(5, 6)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized real/imag access")
	}
}
