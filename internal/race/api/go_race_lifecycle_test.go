// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file.

// Package api contains goroutine lifecycle, defer, global, and object lifetime tests.
package api

import (
	"sync"
	"testing"
	"unsafe"
)

func TestGoNoRace_GoroutineReturn(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = x (after goroutine done)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race after goroutine completion")
	}
}

func TestGoNoRace_MultiGoroutine(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	var wg sync.WaitGroup
	var mu sync.Mutex
	const N = 5

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(val int) {
			_ = val // silence unused param linter
			defer wg.Done()
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceWrite(addr) // x = val
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = x (after all goroutines done)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in multi-goroutine with mutex")
	}
}

func TestGoRace_MultiGoroutineDifferentMutex(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	var wg sync.WaitGroup
	mutexes := make([]sync.Mutex, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mutexes[idx].Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mutexes[idx])))
			RaceWrite(addr) // x = idx (different mutex for each!)
			RaceRelease(uintptr(unsafe.Pointer(&mutexes[idx])))
			mutexes[idx].Unlock()
		}(i)
	}

	wg.Wait()

	// Each goroutine uses different mutex - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race with different mutexes")
	}
}

func TestGoNoRace_DeferSync(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		defer func() {
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceWrite(addr) // x = 2 in defer
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
			done <- true
		}()
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = x (after goroutine done)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in defer sync")
	}
}

func TestGoNoRace_GlobalVar(t *testing.T) {
	Init()
	defer Fini()

	// Simulate global variable
	var globalX int
	addr := uintptr(unsafe.Pointer(&globalX))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // globalX = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = globalX
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in global variable sync")
	}
}

func TestGoRace_GlobalVarNoSync(t *testing.T) {
	Init()
	defer Fini()

	var globalX int
	addr := uintptr(unsafe.Pointer(&globalX))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceRead(addr) // y = globalX
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceWrite(addr) // globalX = 1 (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Read and write with different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in global variable")
	}
}

func TestGoNoRace_StringConcat(t *testing.T) {
	Init()
	defer Fini()

	var s string
	addr := uintptr(unsafe.Pointer(&s))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // s = "hello"
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceWrite(addr) // s = s + " world"
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in string concatenation")
	}
}

func TestGoNoRace_Counter(t *testing.T) {
	Init()
	defer Fini()

	var counter int
	addr := uintptr(unsafe.Pointer(&counter))
	var wg sync.WaitGroup
	var mu sync.Mutex
	const N = 10

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceRead(addr)  // tmp = counter
			RaceWrite(addr) // counter = tmp + 1
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
		}()
	}

	wg.Wait()
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = counter
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in protected counter")
	}
}

func TestGoRace_CounterNoMutex(t *testing.T) {
	Init()
	defer Fini()

	var counter int
	addr := uintptr(unsafe.Pointer(&counter))
	var wg sync.WaitGroup
	mutexes := make([]sync.Mutex, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mutexes[idx].Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mutexes[idx])))
			RaceRead(addr)  // tmp = counter
			RaceWrite(addr) // counter++ (different mutex!)
			RaceRelease(uintptr(unsafe.Pointer(&mutexes[idx])))
			mutexes[idx].Unlock()
		}(i)
	}

	wg.Wait()

	// Different mutexes for same counter - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect race in counter")
	}
}

func TestGoNoRace_TimerCallback(t *testing.T) {
	Init()
	defer Fini()

	var x int
	addr := uintptr(unsafe.Pointer(&x))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // x = 1 (simulating timer callback)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // _ = x (after timer done)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in timer callback sync")
	}
}

func TestGoNoRace_Int32RW(t *testing.T) {
	Init()
	defer Fini()

	var x int32
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
		t.Errorf("False positive: detected race in int32 access")
	}
}

func TestGoRace_Int32RW(t *testing.T) {
	Init()
	defer Fini()

	var x int32
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // x = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // _ = x (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Write and read with different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect int32 race")
	}
}

func TestGoNoRace_Float64RW(t *testing.T) {
	Init()
	defer Fini()

	var x float64
	addr := uintptr(unsafe.Pointer(&x))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // x = 3.14
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
		t.Errorf("False positive: detected race in float64 access")
	}
}

func TestGoNoRace_StackPattern(t *testing.T) {
	Init()
	defer Fini()

	stack := make([]int, 0)
	addr := uintptr(unsafe.Pointer(&stack))
	done := make(chan bool)
	var mu sync.Mutex

	// Push
	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // stack = append(stack, 1)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	// Pop
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr)  // len(stack)
	RaceWrite(addr) // stack = stack[:len-1]
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in stack operations")
	}
}

func TestGoNoRace_QueuePattern(t *testing.T) {
	Init()
	defer Fini()

	queue := make([]int, 0)
	addr := uintptr(unsafe.Pointer(&queue))
	done := make(chan bool)
	var mu sync.Mutex

	// Enqueue
	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // queue = append(queue, item)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	// Dequeue
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr)  // queue[0]
	RaceWrite(addr) // queue = queue[1:]
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in queue operations")
	}
}

func TestGoNoRace_ErrorReturn(t *testing.T) {
	Init()
	defer Fini()

	var err error
	addr := uintptr(unsafe.Pointer(&err))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // err = errors.New("error")
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // if err != nil
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in error return")
	}
}

func TestGoNoRace_BoolFlag(t *testing.T) {
	Init()
	defer Fini()

	var done bool
	addr := uintptr(unsafe.Pointer(&done))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // done = true
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // if done
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: detected race in bool flag")
	}
}

func TestGoRace_BoolFlagNoSync(t *testing.T) {
	Init()
	defer Fini()

	var done bool
	addr := uintptr(unsafe.Pointer(&done))
	ch := make(chan bool, 2)
	var mu1, mu2 sync.Mutex

	go func() {
		mu1.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu1)))
		RaceWrite(addr) // done = true
		RaceRelease(uintptr(unsafe.Pointer(&mu1)))
		mu1.Unlock()
		ch <- true
	}()

	go func() {
		mu2.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu2)))
		RaceRead(addr) // if done (different mutex!)
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	// Write and read with different mutexes - RACE!
	if RacesDetected() == 0 {
		t.Errorf("False negative: failed to detect bool flag race")
	}
}

func TestGoNoRace_ObjectCreate(t *testing.T) {
	Init()
	defer Fini()

	var obj int
	addr := uintptr(unsafe.Pointer(&obj))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // obj = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // use obj
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: object create with mutex is safe")
	}
}

func TestGoRace_ObjectCreateNoSync(t *testing.T) {
	Init()
	defer Fini()

	var obj int
	addr := uintptr(unsafe.Pointer(&obj))
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
		RaceRead(addr) // Different mutex!
		RaceRelease(uintptr(unsafe.Pointer(&mu2)))
		mu2.Unlock()
		ch <- true
	}()

	<-ch
	<-ch

	if RacesDetected() == 0 {
		t.Errorf("False negative: different mutexes should race")
	}
}

func TestGoNoRace_MethodCall(t *testing.T) {
	Init()
	defer Fini()

	type Counter struct {
		value int
	}
	c := &Counter{}
	addr := uintptr(unsafe.Pointer(&c.value))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceRead(addr)
		RaceWrite(addr) // c.value++
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr) // c.Value()
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: method call with mutex is safe")
	}
}

func TestGoNoRace_FieldUpdate(t *testing.T) {
	Init()
	defer Fini()

	type Config struct {
		timeout int
		retries int
	}
	cfg := &Config{}
	addrT := uintptr(unsafe.Pointer(&cfg.timeout))
	addrR := uintptr(unsafe.Pointer(&cfg.retries))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrT)
		RaceWrite(addrR)
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addrT)
	RaceRead(addrR)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: field update with mutex is safe")
	}
}

func TestGoNoRace_TimeoutPattern(t *testing.T) {
	Init()
	defer Fini()

	var result int
	addr := uintptr(unsafe.Pointer(&result))
	done := make(chan bool)
	var mu sync.Mutex

	go func() {
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addr) // result = compute()
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		done <- true
	}()

	<-done
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addr)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: timeout pattern is safe")
	}
}

func TestGoNoRace_RetryPattern(t *testing.T) {
	Init()
	defer Fini()

	var attempts, success int
	addrA := uintptr(unsafe.Pointer(&attempts))
	addrS := uintptr(unsafe.Pointer(&success))
	ch := make(chan bool)
	var mu sync.Mutex

	go func() {
		for i := 0; i < 3; i++ {
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceRead(addrA)
			RaceWrite(addrA) // attempts++
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
		}
		mu.Lock()
		RaceAcquire(uintptr(unsafe.Pointer(&mu)))
		RaceWrite(addrS) // success = 1
		RaceRelease(uintptr(unsafe.Pointer(&mu)))
		mu.Unlock()
		ch <- true
	}()

	<-ch
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	RaceRead(addrA)
	RaceRead(addrS)
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: retry pattern is safe")
	}
}

func TestGoNoRace_BatchProcess(t *testing.T) {
	Init()
	defer Fini()

	batch := make([]int, 5)
	ch := make(chan bool, 5)
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		idx := i
		go func() {
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			addr := uintptr(unsafe.Pointer(&batch[idx]))
			RaceWrite(addr)
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
			ch <- true
		}()
	}

	for i := 0; i < 5; i++ {
		<-ch
	}

	// Read all results
	mu.Lock()
	RaceAcquire(uintptr(unsafe.Pointer(&mu)))
	for i := 0; i < 5; i++ {
		addr := uintptr(unsafe.Pointer(&batch[i]))
		RaceRead(addr)
	}
	RaceRelease(uintptr(unsafe.Pointer(&mu)))
	mu.Unlock()

	if RacesDetected() > 0 {
		t.Errorf("False positive: batch process is safe")
	}
}

func TestGoNoRace_EventHandler(t *testing.T) {
	Init()
	defer Fini()

	var eventData int
	addr := uintptr(unsafe.Pointer(&eventData))
	events := make(chan int, 3)
	done := make(chan bool)
	var mu sync.Mutex

	// Event producer
	go func() {
		for i := 0; i < 3; i++ {
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceWrite(addr)
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
			events <- i
		}
		close(events)
	}()

	// Event handler
	go func() {
		for range events {
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceRead(addr)
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
		}
		done <- true
	}()

	<-done

	if RacesDetected() > 0 {
		t.Errorf("False positive: event handler is safe")
	}
}

func TestGoNoRace_ResourcePool(t *testing.T) {
	Init()
	defer Fini()

	pool := make([]int, 3)
	ch := make(chan bool, 3)
	var mu sync.Mutex

	// Acquire and release resources
	for i := 0; i < 3; i++ {
		idx := i
		go func() {
			// Acquire
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			addr := uintptr(unsafe.Pointer(&pool[idx]))
			RaceWrite(addr)
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()

			// Use resource...

			// Release
			mu.Lock()
			RaceAcquire(uintptr(unsafe.Pointer(&mu)))
			RaceWrite(addr)
			RaceRelease(uintptr(unsafe.Pointer(&mu)))
			mu.Unlock()
			ch <- true
		}()
	}

	for i := 0; i < 3; i++ {
		<-ch
	}

	if RacesDetected() > 0 {
		t.Errorf("False positive: resource pool is safe")
	}
}
