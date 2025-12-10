// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains atomic-like synchronization tests.
// These tests simulate atomic operations using mutex-based synchronization.
package api

import (
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

// TestGoNoRace_AtomicAddInt64 - atomic Add provides happens-before.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicAddInt64(t *testing.T) {
	Init()
	defer Fini()

	var x1, x2 int
	var s int64
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	ch := make(chan bool, 2)
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x1), true) // x1 = 1
		// Simulate atomic.AddInt64(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		finalVal := s
		RaceRelease(muAddr)
		mu.Unlock()

		if finalVal == 2 {
			simulateAccess(addrOf(&x2), true) // x2 = 1
		}
		ch <- true
	}()
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x2), true) // x2 = 1
		// Simulate atomic.AddInt64(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		finalVal := s
		RaceRelease(muAddr)
		mu.Unlock()

		if finalVal == 2 {
			simulateAccess(addrOf(&x1), true) // x1 = 1
		}
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Add should provide sync")
	}
}

// TestGoRace_AtomicAddInt64 - race when atomic check fails.
func TestGoRace_AtomicAddInt64(t *testing.T) {
	Init()
	defer Fini()

	var x1, x2 int
	var s int64
	var mu1, mu2 sync.Mutex // DIFFERENT mutexes!
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	ch := make(chan bool, 2)
	go func() {
		// x1 write protected by mu1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&x1), true) // x1 = 1
		RaceRelease(mu1Addr)

		// Atomic Add
		RaceAcquire(mu1Addr)
		s++
		finalVal := s
		RaceRelease(mu1Addr)

		if finalVal == 1 { // First goroutine
			// x2 write protected by WRONG mutex - RACE!
			RaceAcquire(mu1Addr)
			simulateAccess(addrOf(&x2), true) // x2 = 1 (RACE!)
			RaceRelease(mu1Addr)
		}
		ch <- true
	}()
	go func() {
		// x2 write protected by mu2
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&x2), true) // x2 = 1 (RACE!)
		RaceRelease(mu2Addr)

		// Atomic Add
		RaceAcquire(mu2Addr)
		s++
		finalVal := s
		RaceRelease(mu2Addr)

		if finalVal == 1 { // First goroutine
			// x1 write protected by WRONG mutex - RACE!
			RaceAcquire(mu2Addr)
			simulateAccess(addrOf(&x1), true) // x1 = 1
			RaceRelease(mu2Addr)
		}
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("Expected race detection with atomic")
	}
}

// TestGoNoRace_AtomicAddInt32 - atomic Add on int32.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicAddInt32(t *testing.T) {
	Init()
	defer Fini()

	var x1, x2 int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	ch := make(chan bool, 2)
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x1), true) // x1 = 1
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		finalVal := s
		RaceRelease(muAddr)
		mu.Unlock()

		if finalVal == 2 {
			simulateAccess(addrOf(&x2), true) // x2 = 1
		}
		ch <- true
	}()
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x2), true) // x2 = 1
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		finalVal := s
		RaceRelease(muAddr)
		mu.Unlock()

		if finalVal == 2 {
			simulateAccess(addrOf(&x1), true) // x1 = 1
		}
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Add int32 should sync")
	}
}

// TestGoNoRace_AtomicLoadAddInt32 - Load followed by Add.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicLoadAddInt32(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.AddInt32(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until atomic load sees 1
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		val := s
		RaceRelease(muAddr)
		mu.Unlock()
		if val == 1 {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Load/Add should sync")
	}
}

// TestGoNoRace_AtomicLoadStoreInt32 - Load and Store sync.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicLoadStoreInt32(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.StoreInt32(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s = 1
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until atomic load sees 1
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		val := s
		RaceRelease(muAddr)
		mu.Unlock()
		if val == 1 {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Load/Store should sync")
	}
}

// TestGoNoRace_AtomicStoreCASInt32 - Store then CAS.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicStoreCASInt32(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.StoreInt32(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s = 1
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until CAS succeeds
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 1 {
			s = 0
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()
		if swapped {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Store/CAS should sync")
	}
}

// TestGoNoRace_AtomicCASLoadInt32 - CAS then Load.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicCASLoadInt32(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.CompareAndSwapInt32(&s, 0, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		if s == 0 {
			s = 1
		}
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until load sees 1
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		val := s
		RaceRelease(muAddr)
		mu.Unlock()
		if val == 1 {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic CAS/Load should sync")
	}
}

// TestGoNoRace_AtomicCASCASInt32 - CAS followed by CAS.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicCASCASInt32(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.CompareAndSwapInt32(&s, 0, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		if s == 0 {
			s = 1
		}
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until CAS succeeds
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 1 {
			s = 0
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()
		if swapped {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic CAS/CAS should sync")
	}
}

// TestGoNoRace_AtomicCASCASInt32_2 - Two CAS competing.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicCASCASInt32_2(t *testing.T) {
	Init()
	defer Fini()

	var x1, x2 int
	var s int32
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	ch := make(chan bool, 2)
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x1), true) // x1 = 1
		// Simulate atomic.CompareAndSwapInt32(&s, 0, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 0 {
			s = 1
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()

		if !swapped {
			simulateAccess(addrOf(&x2), true) // x2 = 1
		}
		ch <- true
	}()
	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x2), true) // x2 = 1
		// Simulate atomic.CompareAndSwapInt32(&s, 0, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 0 {
			s = 1
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()

		if !swapped {
			simulateAccess(addrOf(&x1), true) // x1 = 1
		}
		ch <- true
	}()
	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic CAS competition should sync")
	}
}

// TestGoNoRace_AtomicLoadInt64 - Load on int64.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicLoadInt64(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s int64
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.AddInt64(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s++
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until load sees 1
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		val := s
		RaceRelease(muAddr)
		mu.Unlock()
		if val == 1 {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Load int64 should sync")
	}
}

// TestGoNoRace_AtomicCASCASUInt64 - CAS on uint64.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicCASCASUInt64(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s uint64
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.CompareAndSwapUint64(&s, 0, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		if s == 0 {
			s = 1
		}
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until CAS succeeds
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 1 {
			s = 0
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()
		if swapped {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic CAS uint64 should sync")
	}
}

// TestGoNoRace_AtomicLoadStorePointer - Load/Store pointer.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicLoadStorePointer(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s unsafe.Pointer
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))
	y := 2
	p := unsafe.Pointer(&y)

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.StorePointer(&s, p)
		mu.Lock()
		RaceAcquire(muAddr)
		s = p
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until load sees p
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		val := s
		RaceRelease(muAddr)
		mu.Unlock()
		if val == p {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Load/Store pointer should sync")
	}
}

// TestGoNoRace_AtomicStoreCASUint64 - Store then CAS uint64.
//
// NOTE: Uses actual mutex to serialize RaceAcquire/RaceRelease calls for proper clock propagation.
func TestGoNoRace_AtomicStoreCASUint64(t *testing.T) {
	Init()
	defer Fini()

	var x int
	var s uint64
	var mu sync.Mutex
	muAddr := uintptr(unsafe.Pointer(&mu))

	RaceGoStart(0)
	go func() {
		defer RaceGoEnd()
		simulateAccess(addrOf(&x), true) // x = 2
		// Simulate atomic.StoreUint64(&s, 1)
		mu.Lock()
		RaceAcquire(muAddr)
		s = 1
		RaceRelease(muAddr)
		mu.Unlock()
	}()

	// Spin until CAS succeeds
	for {
		mu.Lock()
		RaceAcquire(muAddr)
		swapped := false
		if s == 1 {
			s = 0
			swapped = true
		}
		RaceRelease(muAddr)
		mu.Unlock()
		if swapped {
			break
		}
		runtime.Gosched()
	}
	simulateAccess(addrOf(&x), true) // x = 1

	if RacesDetected() > 0 {
		t.Errorf("False positive: atomic Store/CAS uint64 should sync")
	}
}

// TestGoRace_AtomicStoreLoad - race between atomic store and plain load.
func TestGoRace_AtomicStoreLoad(t *testing.T) {
	Init()
	defer Fini()

	c := make(chan bool)
	var a int
	var mu1, mu2 sync.Mutex // Different mutexes
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate atomic.StoreUint64(&a, 1) with mu1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a = 1
		RaceRelease(mu1Addr)
		c <- true
	}()

	// Plain load with DIFFERENT mutex (NO real synchronization) - RACE!
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&a), false) // _ = a
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: atomic store vs plain load")
	}
}

// TestGoRace_AtomicLoadStore - race between atomic load and plain store.
func TestGoRace_AtomicLoadStore(t *testing.T) {
	Init()
	defer Fini()

	c := make(chan bool)
	var a int
	var mu1, mu2 sync.Mutex // Different mutexes
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate atomic.LoadUint64(&a) with mu1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), false) // _ = a
		RaceRelease(mu1Addr)
		c <- true
	}()

	// Plain store with DIFFERENT mutex (NO real synchronization) - RACE!
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&a), true) // a = 1
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: atomic load vs plain store")
	}
}

// TestGoRace_AtomicAddLoad - race between atomic add and plain load.
func TestGoRace_AtomicAddLoad(t *testing.T) {
	Init()
	defer Fini()

	c := make(chan bool)
	var a int
	var mu1, mu2 sync.Mutex // Different mutexes
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		// Simulate atomic.AddUint64(&a, 1) with mu1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a++
		RaceRelease(mu1Addr)
		c <- true
	}()

	// Plain load with DIFFERENT mutex (NO real synchronization) - RACE!
	RaceAcquire(mu2Addr)
	simulateAccess(addrOf(&a), false) // _ = a
	RaceRelease(mu2Addr)
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: atomic add vs plain load")
	}
}

// TestGoRace_AtomicAddStore - race between atomic add and plain store.
func TestGoRace_AtomicAddStore(t *testing.T) {
	Init()
	defer Fini()

	c := make(chan bool, 2)
	start := make(chan struct{})
	var a int
	var mu1, mu2 sync.Mutex // Different mutexes
	mu1Addr := uintptr(unsafe.Pointer(&mu1))
	mu2Addr := uintptr(unsafe.Pointer(&mu2))

	go func() {
		<-start
		runtime.Gosched()
		// Simulate atomic.AddUint64(&a, 1) with mu1
		RaceAcquire(mu1Addr)
		simulateAccess(addrOf(&a), true) // a++
		RaceRelease(mu1Addr)
		c <- true
	}()

	go func() {
		<-start
		runtime.Gosched()
		// Plain store with DIFFERENT mutex (NO real synchronization) - RACE!
		RaceAcquire(mu2Addr)
		simulateAccess(addrOf(&a), true) // a = 42
		RaceRelease(mu2Addr)
		c <- true
	}()

	close(start) // Start both concurrently
	<-c
	<-c

	if RacesDetected() == 0 {
		t.Errorf("Expected race: atomic add vs plain store")
	}
}
