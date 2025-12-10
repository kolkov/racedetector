// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains advanced pattern tests.
package api

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

// TestGoRace_FuncArgument - race on function argument.
func TestGoRace_FuncArgument(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate emptyFunc(x) - reads x
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // read x for func arg
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write x
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&x), true) // x = 1
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: function argument")
	}
}

// TestGoRace_FuncArgument2 - race on function parameter.
func TestGoRace_FuncArgument2(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), true) // x = 42
		RaceRelease(mu1Addr)
		ch <- true
	}()
	go func() {
		// Pass x to function (reads x)
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x), false) // func(y int)(x)
		RaceRelease(mu2Addr)
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: function parameter")
	}
}

// TestGoRace_Sprint - race on fmt.Sprint argument.
func TestGoRace_Sprint(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate fmt.Sprint(x) - reads x
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // fmt.Sprint(x)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write x
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&x), true) // x = 1
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: fmt.Sprint argument")
	}
}

// TestGoRace_ArrayCopyAssign - race on array copy assignment.
func TestGoRace_ArrayCopyAssign(t *testing.T) {
	Init()
	defer Fini()

	var a [5]int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a[3]), true) // a[3] = 1
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Array copy reads all elements
	RaceAcquire(mu2Addr)
	for i := 0; i < 5; i++ {
		simulateAccess(addrOf(&a[i]), false) // a = [5]int{...}
	}
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: array copy")
	}
}

// TestGoRace_StructRW - race on struct read-write.
func TestGoRace_StructRW(t *testing.T) {
	Init()
	defer Fini()

	var px, py int // Simulate Point struct
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Write struct: p = Point{1, 1}
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&px), true)
		simulateAccess(addrOf(&py), true)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read struct: q = p
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&px), false)
	simulateAccess(addrOf(&py), false)
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: struct read-write")
	}
}

// TestGoRace_StructFieldRWSameField - race on same struct field.
func TestGoRace_StructFieldRWSameField(t *testing.T) {
	Init()
	defer Fini()

	var px int // Simulate Point.x
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&px), true) // p.x = 1
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read same field
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&px), false) // _ = p.x
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: struct field")
	}
}

// TestGoNoRace_StructFieldRW - different fields, no race.
func TestGoNoRace_StructFieldRW(t *testing.T) {
	Init()
	defer Fini()

	var px, py int // Simulate Point.x, Point.y
	ch := make(chan bool, 1)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&px), true) // p.x = 1
		RaceRelease(muAddr)
		ch <- true
	}()

	// Write different field
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&py), true) // p.y = 1
	RaceRelease(muAddr)
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: different struct fields")
	}
}

// TestGoRace_PointerStructField - race on pointer struct field.
func TestGoRace_PointerStructField(t *testing.T) {
	Init()
	defer Fini()

	var px int // Simulate (*Point).x
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&px), true) // p.x = 1 (where p is *Point)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read same field
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&px), false) // _ = p.x
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: pointer struct field")
	}
}

// TestGoRace_NestedStructField - race on nested struct field.
func TestGoRace_NestedStructField(t *testing.T) {
	Init()
	defer Fini()

	var px int // Simulate NamedPoint.p.x
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		runtime.Gosched()
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&px), true) // p.p.x = 1
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		runtime.Gosched()
		// Read nested field
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&px), false) // _ = p.p.x
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start)
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: nested struct field")
	}
}

// TestGoRace_EmptyInterfaceWW - race on empty interface.
func TestGoRace_EmptyInterfaceWW(t *testing.T) {
	Init()
	defer Fini()

	var a int // Simulate any interface
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a = 1
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), true) // a = 2
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start) // Start both concurrently
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: empty interface write-write")
	}
}

// TestGoRace_TypedInterfaceWW - race on typed interface.
func TestGoRace_TypedInterfaceWW(t *testing.T) {
	Init()
	defer Fini()

	var a int // Simulate Writer interface
	ch := make(chan bool, 2)
	start := make(chan struct{})
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a = DummyWriter{1}
		RaceRelease(mu1Addr)
		ch <- true
	}()

	go func() {
		<-start
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), true) // a = DummyWriter{2}
		RaceRelease(mu2Addr)
		ch <- true
	}()

	close(start) // Start both concurrently
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: typed interface write-write")
	}
}

// TestGoRace_InterfaceCompare - race on interface comparison.
func TestGoRace_InterfaceCompare(t *testing.T) {
	Init()
	defer Fini()

	var a, b int // Simulate Writer interfaces
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a = DummyWriter{1}
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read for comparison
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&a), false) // _ = a == b
	simulateAccess(addrOf(&b), false)
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: interface comparison")
	}
}

// TestGoRace_InterfaceCompareNil - race on interface nil comparison.
func TestGoRace_InterfaceCompareNil(t *testing.T) {
	Init()
	defer Fini()

	var a int // Simulate Writer interface
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a = DummyWriter{1}
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read for nil comparison
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&a), false) // _ = a == nil
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: interface nil comparison")
	}
}

// TestGoRace_ArrayIndexExpr - race on array index expression.
func TestGoRace_ArrayIndexExpr(t *testing.T) {
	Init()
	defer Fini()

	var arr [10]int
	var idx int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Read index variable
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&idx), false) // arr[idx]
		simulateAccess(addrOf(&arr[5]), false)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write index variable
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&idx), true) // idx = 5
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: array index expression")
	}
}

// TestGoRace_AppendSlice - race on append to slice.
func TestGoRace_AppendSlice(t *testing.T) {
	Init()
	defer Fini()

	var sliceLen int // Simulate slice length/capacity
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Read slice for append
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&sliceLen), false) // s = append(s, x)
		simulateAccess(addrOf(&sliceLen), true)  // update after append
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read slice concurrently
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&sliceLen), false) // _ = s[0]
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: append slice")
	}
}

// TestGoRace_SliceHeader - race on slice header.
func TestGoRace_SliceHeader(t *testing.T) {
	Init()
	defer Fini()

	var slicePtr int // Simulate slice data pointer
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Modify slice header
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&slicePtr), true) // s = make([]int, 10)
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Read slice header
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&slicePtr), false) // len(s)
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: slice header")
	}
}

// TestGoRace_DeferStatement - race with defer.
func TestGoRace_DeferStatement(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate defer func() { x++ }()
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // read in defer
		simulateAccess(addrOf(&x), true)  // x++
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write x
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&x), true) // x = 10
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: defer statement")
	}
}

// TestGoNoRace_DeferWithChannel - defer with channel sync.
func TestGoNoRace_DeferWithChannel(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	go func() {
		// Simulate defer with mutex
		RaceAcquire(muAddr)
		simulateAccess(addrOf(&x), false) // read in defer
		simulateAccess(addrOf(&x), true)  // x++
		RaceRelease(muAddr)
		ch <- true
	}()
	<-ch

	// Write x after goroutine completes
	RaceAcquire(muAddr)
	simulateAccess(addrOf(&x), true) // x = 10
	RaceRelease(muAddr)

	if RacesDetected() > 0 {
		t.Errorf("False positive: defer with sync")
	}
}

// TestGoRace_ClosureCapturedVar - race on closure captured variable.
func TestGoRace_ClosureCapturedVar(t *testing.T) {
	Init()
	defer Fini()

	var x int
	ch := make(chan bool, 1)
	var mu1, mu2 sync.Mutex
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Closure captures x
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x), false) // read x in closure
		RaceRelease(mu1Addr)
		ch <- true
	}()

	// Write x
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&x), true) // x = 42
	RaceRelease(mu2Addr)
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race: closure capture")
	}
}
