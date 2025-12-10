// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains reflection and regression pattern tests.
package api

import (
	"sync"
	"testing"
	"unsafe"
)

// TestGoRace_ReflectRW - reflect read-write race.
func TestGoRace_ReflectRW(t *testing.T) {
	Init()
	defer Fini()

	var i int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate reflect.Set
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&i), true) // v.Elem().Set(1)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Simulate reflect.Int (read)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&i), false) // v.Elem().Int()
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: reflect read-write")
	}
}

// TestGoRace_ReflectWW - reflect write-write race.
func TestGoRace_ReflectWW(t *testing.T) {
	Init()
	defer Fini()

	var i int
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&i), true) // v.Elem().Set(1)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&i), true) // v.Elem().Set(2)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start) // Start both concurrently
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: reflect write-write")
	}
}

// TestGoRace_ReflectCopyWW - reflect.Copy write-write race.
func TestGoRace_ReflectCopyWW(t *testing.T) {
	Init()
	defer Fini()

	var arr [2]int // Simulate []byte slice
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&arr[0]), true) // reflect.Copy writes
		simulateAccess(addrOf(&arr[1]), true)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&arr[0]), true) // reflect.Copy writes
		simulateAccess(addrOf(&arr[1]), true)
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start) // Start both concurrently
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: reflect.Copy write-write")
	}
}

// TestGoRace_ReturnValue - race on return value.
func TestGoRace_ReturnValue(t *testing.T) {
	Init()
	defer Fini()

	var a int
	c := make(chan int)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	// Simulate return a with goroutine reading a
	RaceAcquire(mu1Addr)
	simulateAccess(addrOf(&a), true) // a = 42
	RaceRelease(mu1Addr)

	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), false) // read a in goroutine
		RaceRelease(mu2Addr)
		c <- 1
	}()

	// return a (reads a with mu1)
	RaceAcquire(mu1Addr)
	simulateAccess(addrOf(&a), false) // return a
	RaceRelease(mu1Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: return value")
	}
}

// TestGoNoRace_StackMethodCall - stack push/pop with sync.
func TestGoNoRace_StackMethodCall(t *testing.T) {
	Init()
	defer Fini()

	var stackTop int
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	done := make(chan bool)

	go func() {
		RaceAcquire(muAddr)
		// Simulate pass pointer to goroutine
		simulateAccess(addrOf(&stackTop), false)
		RaceRelease(muAddr)
		done <- true
	}()
	<-done

	// push operation
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&stackTop), true) // push
	RaceRelease(muAddr)

	// pop operation
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&stackTop), false) // pop (read)
	simulateAccess(addrOf(&stackTop), true)  // pop (update)
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: stack with mutex sync")
	}
}

// TestGoNoRace_ChannelFactory - channel factory pattern.
func TestGoNoRace_ChannelFactory(t *testing.T) {
	Init()
	defer Fini()

	var callCount int
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	ch := make(chan bool, 1)

	// Simulate makeChan()
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&callCount), true) // callCount++
	RaceRelease(muAddr)
	ch <- true

	// Simulate call() - reads from channel
	<-ch

	RaceAcquire(muAddr)
	simulateAccess(addrOf(&callCount), false) // check callCount
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: channel factory with sync")
	}
}

// TestGoRace_RecursiveMethod - race in recursive method.
func TestGoRace_RecursiveMethod(t *testing.T) {
	Init()
	defer Fini()

	var state int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate recursive call reading state
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&state), false) // read state in recursion
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write state with different mutex
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&state), true) // state = x
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: recursive method")
	}
}

// TestGoNoRace_InterfaceConversionLoop - interface conversion in loop.
func TestGoNoRace_InterfaceConversionLoop(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	// Simulate interface conversion in loop condition
	for i := 0; i < 1; i++ {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&x), false) // iface(x).Foo().b
		RaceRelease(muAddr)
	}

	if RacesDetected() > 0 {
		t.Errorf("False positive: interface conversion")
	}
}

// TestGoRace_UnaddressableMapLen - race on unaddressable map len.
func TestGoRace_UnaddressableMapLen(t *testing.T) {
	Init()
	defer Fini()

	var mapEntry int // Simulate m[0]
	ch := make(chan int, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate len(m[0])
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&mapEntry), false) // read for len
		RaceRelease(mu1Addr)
		ch <- 0
	}()

	// Simulate m[0][0] = 1 (write to map entry)
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&mapEntry), true) // m[0][0] = 1
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: unaddressable map len")
	}
}

// TestGoRace_StructReturn - race on struct return.
func TestGoRace_StructReturn(t *testing.T) {
	Init()
	defer Fini()

	var rectX, rectY int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate NewImage().min access
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&rectX), false) // read .min.x
		simulateAccess(addrOf(&rectY), false) // read .min.y
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write to struct fields
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&rectX), true) // min.x = 0
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: struct return access")
	}
}

// TestGoRace_InlineFunction - race in inlined function.
func TestGoRace_InlineFunction(t *testing.T) {
	Init()
	defer Fini()

	var data int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate inlined function access
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&data), false) // inlinetest(p).x
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write data
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&data), true) // data = val
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: inline function")
	}
}

// TestGoRace_ArraySliceIndex - race on array slice indexing.
func TestGoRace_ArraySliceIndex(t *testing.T) {
	Init()
	defer Fini()

	var v [10]int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate v[(i*4)/3] access
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&v[1]), false) // read v[calculated_index]
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write to array
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&v[1]), true) // v[1] = x
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: array slice index")
	}
}

// TestGoRace_NamedReturn - race on named return value.
func TestGoRace_NamedReturn(t *testing.T) {
	Init()
	defer Fini()

	var retVal int
	c := make(chan int)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	// Simulate function with named return
	RaceAcquire(mu1Addr)
	simulateAccess(addrOf(&retVal), true) // a = 42
	RaceRelease(mu1Addr)

	go func() {
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&retVal), false) // read a in goroutine
		RaceRelease(mu2Addr)
		c <- 1
	}()

	// return a (implicit copy)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: named return value")
	}
}

// TestGoNoRace_MethodReceiver - method receiver with sync.
func TestGoNoRace_MethodReceiver(t *testing.T) {
	Init()
	defer Fini()

	var receiverField int
	ch := make(chan bool, 2)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		// Simulate method call
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&receiverField), false) // t.method() reads t
		RaceRelease(muAddr)
		ch <- true
	}()

	go func() {
		// Another method call
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&receiverField), false) // t.method2() reads t
		RaceRelease(muAddr)
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: method receiver with sync")
	}
}

// TestGoRace_ShortVarDecl - race on short variable declaration.
func TestGoRace_ShortVarDecl(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate: y := x + 1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // read x
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write x
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&x), true) // x = 10
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: short variable declaration")
	}
}

// TestGoNoRace_BufferedChannelClose - buffered channel close sync.
func TestGoNoRace_BufferedChannelClose(t *testing.T) {
	Init()
	defer Fini()

	var data int
	ch := make(chan bool, 10)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&data), true) // data = 1
		RaceRelease(muAddr)
		ch <- true
	}()

	<-ch // Channel receive provides sync
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&data), false) // read data
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: buffered channel provides sync")
	}
}

// TestGoRace_TypeSwitch - race in type switch.
func TestGoRace_TypeSwitch(t *testing.T) {
	Init()
	defer Fini()

	var data int
	c := make(chan int, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate type switch reading
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&data), false) // switch i.(type)
		RaceRelease(mu1Addr)
		c <- 1
	}()

	// Write to data
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&data), true) // data = x
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: type switch")
	}
}
