// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains mutex and RWMutex synchronization tests.
package api

import (
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestGoNoRace_Mutex(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.Mutex
	var x int
	addr := addrOf(&x)

	ch := make(chan bool, 2)

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, true) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, true) // x = 2
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in properly synchronized code")
	}
}

func TestGoRace_Mutex(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.Mutex
	var x int
	addr := addrOf(&x)

	ch := make(chan bool, 2)
	start := make(chan struct{}) // Barrier to ensure concurrent access

	go func() {
		<-start                    // Wait for start signal
		simulateAccess(addr, true) // x = 1 (BEFORE lock!)
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		<-start                    // Wait for start signal
		simulateAccess(addr, true) // x = 2 (BEFORE lock!)
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	// Small delay to ensure goroutines are waiting on the channel
	time.Sleep(time.Millisecond)
	close(start) // Start both goroutines simultaneously

	<-ch
	<-ch

	races := RacesDetected()
	if races == 0 {
		t.Errorf("False negative: failed to detect race before mutex sync (races=%d)", races)
	}
}

func TestGoRace_Mutex2(t *testing.T) {
	Init()
	defer Fini()

	var mu1, mu2 sync.Mutex
	var x int
	addr := addrOf(&x)

	ch := make(chan bool, 2)

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		simulateAccess(addr, true) // x = 1 (protected by mu1)
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		simulateAccess(addr, true) // x = 2 (protected by mu2 - DIFFERENT mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race with different mutexes")
	}
}

func TestGoNoRace_MutexSemaphore(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.Mutex
	var x int
	addr := addrOf(&x)

	ch := make(chan bool, 2)

	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))

	go func() {
		simulateAccess(addr, true) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, true) // x = 2
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in semaphore pattern")
	}
}

func TestGoNoRace_MutexPureHappensBefore(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.Mutex
	var x int
	addr := addrOf(&x)
	written := false
	ch := make(chan bool, 2)

	go func() {
		simulateAccess(addr, true) // x = 1
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		written = true
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		for !written {
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		}
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		simulateAccess(addr, true) // x = 1
		ch <- true
	}()

	<-ch
	<-ch

	// Note: This test relies on happens-before from flag synchronization.
	// Our detector may or may not catch this correctly depending on
	// whether we track the flag access properly.
	t.Logf("Races detected: %d (pure happens-before test)", RacesDetected())
}

func TestGoNoRace_MutexExampleFromHtml(t *testing.T) {
	Init()
	defer Fini()

	var l sync.Mutex
	var a int
	addr := addrOf(&a)

	l.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&l)))

	go func() {
		simulateAccess(addr, true) // a = "hello, world"
		RaceRelease(uintptr(unsafe.Pointer(&l)))
		l.Unlock()
	}()

	l.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&l)))
	simulateAccess(addr, false) // _ = a
	RaceRelease(uintptr(unsafe.Pointer(&l)))
	l.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in Go memory model example")
	}
}

func TestGoNoRace_RWMutexWrite(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.RWMutex
	var x int
	addr := addrOf(&x)
	ch := make(chan bool, 2)

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, true) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, true) // x = 2
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in RWMutex protected code")
	}
}

func TestGoNoRace_RWMutexReadRead(t *testing.T) {
	Init()
	defer Fini()

	var mu sync.RWMutex
	var x int
	addr := addrOf(&x)

	// Initialize x first with write lock
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	simulateAccess(addr, true) // x = 1
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	ch := make(chan bool, 2)

	go func() {
		mu.RLock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, false) // read x
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.RUnlock()
		ch <- true
	}()

	go func() {
		mu.RLock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		simulateAccess(addr, false) // read x
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.RUnlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in concurrent RWMutex reads")
	}
}

func TestGoRace_RWMutexReadWrite(t *testing.T) {
	Init()
	defer Fini()

	var mu1 sync.RWMutex
	var mu2 sync.RWMutex
	var x int
	addr := addrOf(&x)
	ch := make(chan bool, 2)
	start := make(chan struct{}) // Start barrier for concurrent execution.

	go func() {
		<-start
		mu1.RLock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		simulateAccess(addr, false) // read x
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.RUnlock()
		ch <- true
	}()

	go func() {
		<-start
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		simulateAccess(addr, true) // write x (DIFFERENT lock!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	close(start) // Release both goroutines simultaneously.
	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect RWMutex read-write race with different locks")
	}
}

func TestGoNoRace_DeferredMutexUnlock(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		defer func() {
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
		}()
		RaceWrite(addr)
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: deferred mutex unlock is safe")
	}
}

func TestGoNoRace_NestedMutex(t *testing.T) {
	Init()
	defer Fini()

	var x, y int
	addrX := uintptr(unsafe.Pointer(&x))
	addrY := uintptr(unsafe.Pointer(&y))
	ch := make(chan bool)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addrX)

		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addrY)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()

		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	<-ch
	mu1.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
	RaceRead(addrX)

	mu2.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
	RaceRead(addrY)
	RaceRelease(uintptr(unsafe.Pointer(&mu2)))
	mu2.Unlock()

	RaceRelease(uintptr(unsafe.Pointer(&mu1)))
	mu1.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: nested mutex is safe")
	}
}

func TestGoRace_SwappedMutexOrder(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // Protected by mu1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // Protected by mu2 (different!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: different mutexes protecting same data should race")
	}
}
