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
// METHOD CALL OPERATIONS (10 tests)
// =============================================================================

type Counter struct {
	count int
}

func (c *Counter) Increment() {
	c.count++
}

func (c *Counter) Get() int {
	return c.count
}

func (c *Counter) Set(val int) {
	c.count = val
}

// TestGoRace_MethodReceiver tests concurrent method calls on same receiver.
func TestGoRace_MethodReceiver(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		c.Increment()
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		c.Increment()
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent method calls")
	}
}

// TestGoNoRace_MethodReceiverSync tests synchronized method calls.
func TestGoNoRace_MethodReceiverSync(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		c.Increment()
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(countAddr)
	_ = c.Get()
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized method calls")
	}
}

// TestGoRace_MethodValue tests concurrent method value calls.
func TestGoRace_MethodValue(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	increment := c.Increment
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		increment()
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		increment()
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent method value calls")
	}
}

// TestGoNoRace_MethodValue tests synchronized method value.
func TestGoNoRace_MethodValue(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	increment := c.Increment
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		increment()
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(countAddr)
	_ = c.count
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized method value")
	}
}

// TestGoRace_MethodExpression tests concurrent method expression.
func TestGoRace_MethodExpression(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	set := (*Counter).Set
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(countAddr)
		set(c, 10)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(countAddr)
		set(c, 20)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent method expression")
	}
}

// TestGoNoRace_MethodExpression tests synchronized method expression.
func TestGoNoRace_MethodExpression(t *testing.T) {
	Init()
	defer Fini()

	c := &Counter{}
	set := (*Counter).Set
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	countAddr := uintptr(unsafe.Pointer(&c.count))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(countAddr)
		set(c, 10)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(countAddr)
	_ = c.count
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized method expression")
	}
}

type EmbeddedCounter struct {
	*Counter
}

// TestGoRace_EmbeddedMethod tests concurrent embedded method calls.
func TestGoRace_EmbeddedMethod(t *testing.T) {
	Init()
	defer Fini()

	ec := &EmbeddedCounter{Counter: &Counter{}}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	countAddr := uintptr(unsafe.Pointer(&ec.count))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		ec.Increment()
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		ec.Increment()
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent embedded method calls")
	}
}

// TestGoNoRace_EmbeddedMethod tests synchronized embedded method.
func TestGoNoRace_EmbeddedMethod(t *testing.T) {
	Init()
	defer Fini()

	ec := &EmbeddedCounter{Counter: &Counter{}}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	countAddr := uintptr(unsafe.Pointer(&ec.count))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(countAddr)
		RaceRead(countAddr)
		ec.Increment()
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(countAddr)
	_ = ec.Get()
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized embedded method")
	}
}

type GenericContainer[T any] struct {
	value T
}

func (g *GenericContainer[T]) Set(val T) {
	g.value = val
}

func (g *GenericContainer[T]) Get() T {
	return g.value
}

// TestGoRace_GenericMethod tests concurrent generic method calls.
func TestGoRace_GenericMethod(t *testing.T) {
	Init()
	defer Fini()

	gc := &GenericContainer[int]{}
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))
	valueAddr := uintptr(unsafe.Pointer(&gc.value))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		RaceWrite(valueAddr)
		gc.Set(10)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		RaceWrite(valueAddr)
		gc.Set(20)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race on concurrent generic method calls")
	}
}

// TestGoNoRace_GenericMethod tests synchronized generic method.
func TestGoNoRace_GenericMethod(t *testing.T) {
	Init()
	defer Fini()

	gc := &GenericContainer[int]{}
	done := make(chan bool)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	valueAddr := uintptr(unsafe.Pointer(&gc.value))

	go func() {
		RaceAcquire(muAddr)
		RaceWrite(valueAddr)
		gc.Set(10)
		RaceRelease(muAddr)
		done <- true
	}()

	<-done

	RaceAcquire(muAddr)
	RaceRead(valueAddr)
	_ = gc.Get()
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive on synchronized generic method")
	}
}
