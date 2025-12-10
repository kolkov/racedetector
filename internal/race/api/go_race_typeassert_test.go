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
// TYPE ASSERTION OPERATIONS (10 tests)
// =============================================================================

// TestGoRace_InterfaceTypeAssert tests concurrent type assertion.
func TestGoRace_InterfaceTypeAssert(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(ifaceAddr)
		iface = "string"
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(ifaceAddr)
		_, _ = iface.(int)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent type assertion")
	}
}

// TestGoNoRace_InterfaceTypeAssert tests synchronized type assertion.
func TestGoNoRace_InterfaceTypeAssert(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(ifaceAddr)
		_, _ = iface.(int)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(ifaceAddr)
	iface = "string"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized type assertion")
	}
}

// TestGoRace_TypeSwitchConcurrent tests concurrent type switch.
func TestGoRace_TypeSwitchConcurrent(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(ifaceAddr)
		iface = 3.14
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(ifaceAddr)
		switch iface.(type) {
		case int:
		case string:
		}
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent type switch")
	}
}

// TestGoNoRace_TypeSwitchSync tests synchronized type switch.
func TestGoNoRace_TypeSwitchSync(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(ifaceAddr)
		switch iface.(type) {
		case int:
		case string:
		}
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(ifaceAddr)
	iface = "hello"
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized type switch")
	}
}

// TestGoRace_EmptyInterface tests concurrent empty interface assignment.
func TestGoRace_EmptyInterface(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(ifaceAddr)
		iface = 42
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(ifaceAddr)
		iface = "string"
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent empty interface assignment")
	}
}

// TestGoNoRace_EmptyInterface tests synchronized empty interface.
func TestGoNoRace_EmptyInterface(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(ifaceAddr)
		iface = 42
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(ifaceAddr)
	_ = iface
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized empty interface")
	}
}

// Reader interface for testing.
type Reader interface {
	Read() int
}

// MyReader implements Reader.
type MyReader struct {
	value int
}

func (m *MyReader) Read() int {
	return m.value
}

// TestGoRace_InterfaceMethod tests concurrent interface method call.
func TestGoRace_InterfaceMethod(t *testing.T) {
	Init()
	defer Fini()

	reader := &MyReader{value: 10}
	var iface Reader = reader
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	valueAddr := uintptr(unsafe.Pointer(&reader.value))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(valueAddr)
		reader.value = 20
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(valueAddr)
		_ = iface.Read()
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on interface method with concurrent write")
	}
}

// TestGoNoRace_InterfaceMethod tests synchronized interface method.
func TestGoNoRace_InterfaceMethod(t *testing.T) {
	Init()
	defer Fini()

	reader := &MyReader{value: 10}
	var iface Reader = reader
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	valueAddr := uintptr(unsafe.Pointer(&reader.value))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(valueAddr)
		_ = iface.Read()
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(valueAddr)
	reader.value = 20
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized interface method")
	}
}

// TestGoRace_InterfaceNil tests concurrent nil interface check.
func TestGoRace_InterfaceNil(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(ifaceAddr)
		iface = nil
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceRead(ifaceAddr)
		_ = (iface == nil)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on nil interface check")
	}
}

// TestGoNoRace_InterfaceNil tests synchronized nil interface check.
func TestGoNoRace_InterfaceNil(t *testing.T) {
	Init()
	defer Fini()

	var iface interface{} = 42
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	ifaceAddr := uintptr(unsafe.Pointer(&iface))

	go func() {
		RaceAcquire(muAddr)
		RaceRead(ifaceAddr)
		_ = (iface == nil)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceWrite(ifaceAddr)
	iface = nil
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized nil interface check")
	}
}
