// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains struct, array, slice, map, pointer, and interface tests.
package api

import (
	"sync"
	"testing"
	"unsafe"
)

func TestGoNoRace_StructDifferentFields(t *testing.T) {
	Init()
	defer Fini()

	type Point struct {
		x, y int
	}
	var p Point
	addrX := uintptr(unsafe.Pointer(&p.x))
	addrY := uintptr(unsafe.Pointer(&p.y))
	ch := make(chan bool, 2)

	go func() {
		RaceWrite(addrX) // p.x = 1
		ch <- true
	}()

	go func() {
		RaceWrite(addrY) // p.y = 2
		ch <- true
	}()

	<-ch
	<-ch

	// Different fields at different addresses - no race.
	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different struct fields")
	}
}

func TestGoRace_StructSameField(t *testing.T) {
	Init()
	defer Fini()

	type Point struct {
		x, y int
	}
	var p Point
	_ = p.y // silence unused field linter
	addrX := uintptr(unsafe.Pointer(&p.x))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrX) // p.x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addrX) // p.x = 2 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same field, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in same struct field")
	}
}

func TestGoNoRace_ArrayDifferentIndices(t *testing.T) {
	Init()
	defer Fini()

	var arr [10]int
	addr0 := uintptr(unsafe.Pointer(&arr[0]))
	addr1 := uintptr(unsafe.Pointer(&arr[1]))
	ch := make(chan bool, 2)

	go func() {
		RaceWrite(addr0) // arr[0] = 1
		ch <- true
	}()

	go func() {
		RaceWrite(addr1) // arr[1] = 2
		ch <- true
	}()

	<-ch
	<-ch

	// Different indices at different addresses - no race.
	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different array indices")
	}
}

func TestGoRace_ArraySameIndex(t *testing.T) {
	Init()
	defer Fini()

	var arr [10]int
	addr0 := uintptr(unsafe.Pointer(&arr[0]))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr0) // arr[0] = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr0) // arr[0] = 2 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same index, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in same array index")
	}
}

func TestGoNoRace_SliceByteAppend(t *testing.T) {
	Init()
	defer Fini()

	var s1, s2 []byte
	addr1 := uintptr(unsafe.Pointer(&s1))
	addr2 := uintptr(unsafe.Pointer(&s2))
	ch := make(chan bool, 2)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr1) // s1 = append(s1, 'a')
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr2) // s2 = append(s2, 'b')
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch
	_ = s1
	_ = s2

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different slice writes")
	}
}

func TestGoRace_SliceSameAppend(t *testing.T) {
	Init()
	defer Fini()

	var s []byte
	addr := uintptr(unsafe.Pointer(&s))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // s = append(s, 'a')
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // s = append(s, 'b') - different mutex!
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same slice, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in same slice append")
	}
}

func TestGoNoRace_SliceLen(t *testing.T) {
	Init()
	defer Fini()

	a := make([]bool, 10)
	addrA0 := uintptr(unsafe.Pointer(&a[0]))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrA0) // a[0] = true
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	// len(a) doesn't access elements - safe
	_ = len(a)

	if RacesDetected() > 0 {
		t.Errorf("False positive: len() shouldn't race with element write")
	}
}

func TestGoNoRace_SliceCap(t *testing.T) {
	Init()
	defer Fini()

	a := make([]uint64, 100)
	addr := uintptr(unsafe.Pointer(&a[50]))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // a[50] = 123
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	// cap(a) doesn't access elements - safe
	_ = cap(a)

	if RacesDetected() > 0 {
		t.Errorf("False positive: cap() shouldn't race with element write")
	}
}

func TestGoNoRace_SliceDifferentIndices(t *testing.T) {
	Init()
	defer Fini()

	a := make([]int, 10)
	addr0 := uintptr(unsafe.Pointer(&a[0]))
	addr1 := uintptr(unsafe.Pointer(&a[1]))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr0) // a[0] = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr1) // _ = a[1]
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: different slice indices are safe")
	}
}

func TestGoRace_SliceSameIndex(t *testing.T) {
	Init()
	defer Fini()

	a := make([]int, 10)
	addr := uintptr(unsafe.Pointer(&a[1]))
	ch := make(chan bool)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // a[1] = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // a[1] = 2 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: same slice index with different mutexes should race")
	}
}

func TestGoNoRace_SliceAppendCopy(t *testing.T) {
	Init()
	defer Fini()

	s := make([]int, 0, 10)
	addr := uintptr(unsafe.Pointer(&s))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // s = append(s, 1)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // s2 := make([]int, len(s)); copy(s2, s)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: append/copy with mutex is safe")
	}
}

func TestGoNoRace_SliceGrow(t *testing.T) {
	Init()
	defer Fini()

	var items []int
	addr := uintptr(unsafe.Pointer(&items))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // items = append(items, 1)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // len(items)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: slice grow with mutex is safe")
	}
}

func TestGoNoRace_MapDifferentKeys(t *testing.T) {
	Init()
	defer Fini()

	m := make(map[int]int)
	m[1] = 1
	m[2] = 2
	addr1 := uintptr(unsafe.Pointer(&m))
	ch := make(chan bool, 2)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr1) // m[1] = 10
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr1) // m[2] = 20
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different map keys")
	}
}

func TestGoRace_MapSameKey(t *testing.T) {
	Init()
	defer Fini()

	m := make(map[int]int)
	addr := uintptr(unsafe.Pointer(&m))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // m[1] = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // m[1] = 2 - different mutex!
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same map variable, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in same map key")
	}
}

func TestGoNoRace_MapConcurrentRead(t *testing.T) {
	Init()
	defer Fini()

	m := make(map[string]int)
	m["key"] = 42
	addr := uintptr(unsafe.Pointer(&m))
	ch := make(chan bool)
	var mu sync.Mutex

	// Initialize with mutex
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceWrite(addr)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	// Two goroutines read concurrently
	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRead(addr)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRead(addr)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: concurrent reads with mutex are safe")
	}
}

func TestGoNoRace_MapGrow(t *testing.T) {
	Init()
	defer Fini()

	m := make(map[string]int)
	addr := uintptr(unsafe.Pointer(&m))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // m["key"] = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // len(m)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: map grow with mutex is safe")
	}
}

func TestGoNoRace_InterfaceStore(t *testing.T) {
	Init()
	defer Fini()

	var x interface{}
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = x
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in interface access with mutex")
	}
}

func TestGoNoRace_PointerMessage(t *testing.T) {
	Init()
	defer Fini()

	type msg struct {
		x int
	}
	var m *msg
	addrM := uintptr(unsafe.Pointer(&m))
	c := make(chan bool)
	var mu sync.Mutex

	go func() {
		newMsg := &msg{1}
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrM) // m = newMsg
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		_ = newMsg
		c <- true
	}()

	<-c
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addrM) // _ = m
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in pointer message passing")
	}
}

func TestGoNoRace_PointerIndirection(t *testing.T) {
	Init()
	defer Fini()

	x := new(int)
	addr := uintptr(unsafe.Pointer(x))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // *x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = *x
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: pointer indirection with mutex is safe")
	}
}

func TestGoNoRace_StructFieldsIndependent(t *testing.T) {
	Init()
	defer Fini()

	type Data struct {
		x int
		y int
		z int
	}
	d := &Data{}
	addrX := uintptr(unsafe.Pointer(&d.x))
	addrY := uintptr(unsafe.Pointer(&d.y))
	addrZ := uintptr(unsafe.Pointer(&d.z))
	ch := make(chan bool, 3)
	var mu sync.Mutex

	// Three goroutines write to different fields with same mutex
	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrX)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrY)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrZ)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: different struct fields with same mutex are safe")
	}
}

func TestGoRace_StructSameFieldDifferentMutex(t *testing.T) {
	Init()
	defer Fini()

	type Data struct {
		x int
	}
	d := &Data{}
	addr := uintptr(unsafe.Pointer(&d.x))
	ch := make(chan bool)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr)
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // Same field, different mutex!
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: same struct field with different mutexes should race")
	}
}

func TestGoNoRace_CompNestedFields(t *testing.T) {
	Init()
	defer Fini()

	type P struct{ x, y int }
	type S struct{ s1, s2 P }
	var s S
	_ = s.s1 // silence unused field linter
	addrS2X := uintptr(unsafe.Pointer(&s.s2.x))
	addrS2Y := uintptr(unsafe.Pointer(&s.s2.y))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrS2X) // s.s2.x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceWrite(addrS2Y) // s.s2.y = 2 (different field!)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different nested fields")
	}
}

func TestGoRace_CompSameField(t *testing.T) {
	Init()
	defer Fini()

	type P struct{ x, y int }
	type S struct{ s1, s2 P }
	var s S
	_ = s.s1.x // silence unused field linter
	addrS2Y := uintptr(unsafe.Pointer(&s.s2.y))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrS2Y) // s.s2.y = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addrS2Y) // s.s2.y = 2 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same nested field, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in same nested field")
	}
}

func TestGoNoRace_CompPointerFields(t *testing.T) {
	Init()
	defer Fini()

	type P struct{ x, y int }
	type Ptr struct{ s1, s2 *P }
	p1 := &P{}
	p2 := &P{}
	ptr := Ptr{p1, p2}
	addrX := uintptr(unsafe.Pointer(&ptr.s1.x))
	addrY := uintptr(unsafe.Pointer(&ptr.s1.y))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrX) // ptr.s1.x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceWrite(addrY) // ptr.s1.y = 2 (different field)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in different pointer struct fields")
	}
}

func TestGoRace_CompPointerSameField(t *testing.T) {
	Init()
	defer Fini()

	type P struct{ x, y int }
	type Ptr struct{ s1, s2 *P }
	p1 := &P{}
	p2 := &P{}
	ptr := Ptr{p1, p2}
	_ = ptr.s2.y // silence unused field linter
	addrS2X := uintptr(unsafe.Pointer(&ptr.s2.x))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrS2X) // ptr.s2.x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addrS2X) // ptr.s2.x = 2 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Same field via pointer, different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in pointer struct field")
	}
}

// ========== Batch 12: Additional mop_test.go patterns ==========

func TestGoRace_IntPointerRW(t *testing.T) {
	Init()
	defer Fini()

	var x int
	p := &x
	addr := uintptr(unsafe.Pointer(p))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // *p = 5
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // y = *p (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: pointer indirection race should be detected")
	}
}

func TestGoRace_StringRW(t *testing.T) {
	Init()
	defer Fini()

	var s string
	addr := uintptr(unsafe.Pointer(&s))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // s = "abacaba"
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // _ = s (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: string race should be detected")
	}
}

func TestGoRace_StringPtrRW(t *testing.T) {
	Init()
	defer Fini()

	var x string
	p := &x
	addr := uintptr(unsafe.Pointer(p))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // *p = "a"
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // _ = *p (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: string pointer race should be detected")
	}
}

func TestGoRace_Float64WW(t *testing.T) {
	Init()
	defer Fini()

	var x float64
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // x = 1.0
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // x = 2.0 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: float64 write-write race should be detected")
	}
}

func TestGoRace_Complex128WW(t *testing.T) {
	Init()
	defer Fini()

	var x complex128
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // x = 2 + 2i
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // x = 4 + 4i (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: complex128 write-write race should be detected")
	}
}

func TestGoRace_UnsafePtrRW(t *testing.T) {
	Init()
	defer Fini()

	var x, z int
	var p = unsafe.Pointer(&x)
	addr := uintptr(unsafe.Pointer(&p))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // p = unsafe.Pointer(&z)
		_ = z
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // y = *(*int)(p) (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: unsafe.Pointer race should be detected")
	}
}

func TestGoRace_FuncVariableRW(t *testing.T) {
	Init()
	defer Fini()

	var f func(int) int
	addr := uintptr(unsafe.Pointer(&f))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // f = func(x int) int { return x }
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // y := f(1) (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: function variable race should be detected")
	}
}

func TestGoRace_FuncVariableWW(t *testing.T) {
	Init()
	defer Fini()

	var f func(int) int
	addr := uintptr(unsafe.Pointer(&f))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // f = func(x int) int { return x }
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // f = func(x int) int { return x * x } (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: function variable write-write race should be detected")
	}
}

func TestGoNoRace_Blank(t *testing.T) {
	Init()
	defer Fini()

	var a [5]int
	addr0 := uintptr(unsafe.Pointer(&a[0]))
	addr1 := uintptr(unsafe.Pointer(&a[1]))
	addr2 := uintptr(unsafe.Pointer(&a[2]))
	addr3 := uintptr(unsafe.Pointer(&a[3]))
	ch := make(chan bool, 1)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRead(addr0) // _, _ = a[0], a[1]
		RaceRead(addr1)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr2) // _, _ = a[2], a[3]
	RaceRead(addr3)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	// Reads to different indices are safe
	if RacesDetected() > 0 {
		t.Errorf("False positive: blank reads to different indices are safe")
	}
}

func TestGoRace_AppendRW(t *testing.T) {
	Init()
	defer Fini()

	a := make([]int, 10)
	addr0 := uintptr(unsafe.Pointer(&a[0]))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceRead(addr0) // _ = append(a, 1) reads a[0]
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr0) // a[0] = 1 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: append race should be detected")
	}
}

func TestGoRace_AppendLenRW(t *testing.T) {
	Init()
	defer Fini()

	a := make([]int, 0)
	addrSlice := uintptr(unsafe.Pointer(&a))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrSlice) // a = append(a, 1)
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addrSlice) // _ = len(a) (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: append len race should be detected")
	}
}

func TestGoRace_AppendCapRW(t *testing.T) {
	Init()
	defer Fini()

	a := make([]int, 0)
	addrSlice := uintptr(unsafe.Pointer(&a))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrSlice) // a = append(a, 1)
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addrSlice) // _ = cap(a) (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: append cap race should be detected")
	}
}
