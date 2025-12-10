// Copyright 2025 The racedetector Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"runtime"
	"sync"
	"testing"
)

// TestGetGoroutineID_Basic tests basic goroutine ID extraction.
func TestGetGoroutineID_Basic(t *testing.T) {
	gid := getGoroutineID()

	// GID should be positive (goroutines start at 1, main is 1).
	if gid <= 0 {
		t.Errorf("getGoroutineID() returned non-positive ID: %d", gid)
	}

	// Call again - should return same ID in same goroutine.
	gid2 := getGoroutineID()
	if gid != gid2 {
		t.Errorf("getGoroutineID() not stable: first=%d, second=%d", gid, gid2)
	}
}

// TestGetGoroutineID_FastVsSlow validates fast and slow paths match.
//
// This is CRITICAL: if fast and slow paths disagree, the race detector
// will malfunction (goroutines will be tracked incorrectly).
func TestGetGoroutineID_FastVsSlow(t *testing.T) {
	// Get ID via fast path (uses assembly on amd64).
	fast := getGoroutineIDFast()

	// Get ID via slow path (always uses runtime.Stack parsing).
	slow := getGoroutineIDSlow()

	// They MUST match exactly.
	if fast != slow {
		t.Errorf("Fast and slow paths disagree! fast=%d, slow=%d", fast, slow)
		t.Error("This indicates incorrect goid offset in assembly code.")
		t.Error("Run tools/calc_goid_offset.go to verify offset for your Go version.")
	}
}

// TestGetGoroutineID_MultipleGoroutines tests ID extraction across many goroutines.
func TestGetGoroutineID_MultipleGoroutines(t *testing.T) {
	const numGoroutines = 100

	// Channel to collect GIDs from goroutines.
	gidChan := make(chan int64, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Each goroutine extracts its own GID.
			gid := getGoroutineID()

			// GID must be positive.
			if gid <= 0 {
				t.Errorf("Goroutine got non-positive ID: %d", gid)
				return
			}

			// Send GID to channel.
			gidChan <- gid
		}()
	}

	// Wait for all goroutines to finish.
	wg.Wait()
	close(gidChan)

	// Collect all GIDs.
	gids := make([]int64, 0, numGoroutines)
	for gid := range gidChan {
		gids = append(gids, gid)
	}

	// We should have exactly numGoroutines GIDs.
	if len(gids) != numGoroutines {
		t.Fatalf("Expected %d GIDs, got %d", numGoroutines, len(gids))
	}

	// All GIDs should be unique (no duplicates).
	// Build a set to check uniqueness.
	seen := make(map[int64]bool)
	for _, gid := range gids {
		if seen[gid] {
			t.Errorf("Duplicate GID detected: %d", gid)
		}
		seen[gid] = true
	}
}

// TestGetGoroutineID_Concurrent tests concurrent GID extraction.
//
// This stresses the TLS access mechanism to ensure no races or corruption.
func TestGetGoroutineID_Concurrent(t *testing.T) {
	const numGoroutines = 50
	const numIterations = 1000

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get initial GID for this goroutine.
			expectedGID := getGoroutineID()

			// Call many times - should always return same ID.
			for j := 0; j < numIterations; j++ {
				gid := getGoroutineID()
				if gid != expectedGID {
					t.Errorf("GID changed during execution! expected=%d, got=%d", expectedGID, gid)
					return
				}
			}
		}()
	}

	wg.Wait()
}

// TestGetGoroutineID_FastVsSlow_Concurrent validates consistency under load.
func TestGetGoroutineID_FastVsSlow_Concurrent(t *testing.T) {
	const numGoroutines = 20
	const numIterations = 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				fast := getGoroutineIDFast()
				slow := getGoroutineIDSlow()

				if fast != slow {
					t.Errorf("Fast/slow mismatch in goroutine! fast=%d, slow=%d", fast, slow)
					return
				}
			}
		}()
	}

	wg.Wait()
}

// TestGetGoroutineID_MainGoroutine tests main goroutine ID.
//
// By convention, the main goroutine typically has ID 1, though this
// is not guaranteed by Go runtime. We just verify it's positive.
func TestGetGoroutineID_MainGoroutine(t *testing.T) {
	// This test runs in the test goroutine (which might not be main).
	// Just verify we can extract a valid GID.
	gid := getGoroutineID()

	if gid <= 0 {
		t.Errorf("Main/test goroutine has non-positive ID: %d", gid)
	}
}

// TestGetGoroutineID_NewlyCreated tests GID extraction in newly spawned goroutines.
func TestGetGoroutineID_NewlyCreated(t *testing.T) {
	const numRounds = 10

	for round := 0; round < numRounds; round++ {
		done := make(chan int64)

		// Spawn a new goroutine.
		go func() {
			// Extract GID immediately after creation.
			gid := getGoroutineID()
			done <- gid
		}()

		// Wait for result.
		gid := <-done

		// GID should be positive.
		if gid <= 0 {
			t.Errorf("Round %d: Newly created goroutine has non-positive ID: %d", round, gid)
		}
	}
}

// TestGetGoroutineID_StabilityExtended tests GID doesn't change during goroutine lifetime.
func TestGetGoroutineID_StabilityExtended(t *testing.T) {
	const numChecks = 10000

	// Get initial GID.
	initialGID := getGoroutineID()

	// Call getGoroutineID many times - should never change.
	for i := 0; i < numChecks; i++ {
		gid := getGoroutineID()
		if gid != initialGID {
			t.Fatalf("GID changed during test! initial=%d, iteration %d got %d",
				initialGID, i, gid)
		}

		// Occasionally yield to scheduler to make test more realistic.
		if i%100 == 0 {
			runtime.Gosched()
		}
	}
}

// TestGetGoroutineID_AfterBlocking tests GID after blocking operations.
func TestGetGoroutineID_AfterBlocking(t *testing.T) {
	// Get GID before blocking.
	gidBefore := getGoroutineID()

	// Block on channel.
	ch := make(chan int)
	go func() {
		ch <- 42
	}()
	<-ch

	// GID should be unchanged after blocking.
	gidAfter := getGoroutineID()
	if gidBefore != gidAfter {
		t.Errorf("GID changed after blocking! before=%d, after=%d", gidBefore, gidAfter)
	}

	// Block on mutex.
	var mu sync.Mutex
	mu.Lock()
	go func() {
		// Try to acquire lock (will block briefly).
		mu.Lock()
		defer mu.Unlock()
	}()
	runtime.Gosched() // Let other goroutine try to acquire.
	mu.Unlock()

	// GID should still be unchanged.
	gidAfter2 := getGoroutineID()
	if gidBefore != gidAfter2 {
		t.Errorf("GID changed after mutex blocking! before=%d, after=%d", gidBefore, gidAfter2)
	}
}

// TestParseGID tests the runtime.Stack parsing logic.
func TestParseGID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "standard format",
			input:    "goroutine 1 [running]:",
			expected: 1,
		},
		{
			name:     "large GID",
			input:    "goroutine 999999 [running]:",
			expected: 999999,
		},
		{
			name:     "with stack trace",
			input:    "goroutine 42 [running]:\nmain.main()\n\t/path/to/main.go:10",
			expected: 42,
		},
		{
			name:     "different state",
			input:    "goroutine 123 [chan receive]:",
			expected: 123,
		},
		{
			name:     "invalid - no number",
			input:    "goroutine  [running]:",
			expected: 0,
		},
		{
			name:     "invalid - wrong prefix",
			input:    "thread 123 [running]:",
			expected: 0,
		},
		{
			name:     "invalid - empty",
			input:    "",
			expected: 0,
		},
		{
			name:     "invalid - too short",
			input:    "goroutine",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGID([]byte(tt.input))
			if result != tt.expected {
				t.Errorf("parseGID(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetGoroutineID_NoAllocations verifies fast path has zero allocations.
//
// This is critical for performance - the fast path must not allocate.
// Uses outrigdev/goid library which provides assembly-optimized path
// for Go 1.23+ on amd64/arm64.
func TestGetGoroutineID_NoAllocations(t *testing.T) {
	// Warm up
	for i := 0; i < 100; i++ {
		_ = getGoroutineIDFast()
	}

	// Measure allocations
	allocs := testing.AllocsPerRun(1000, func() {
		_ = getGoroutineIDFast()
	})

	if allocs > 0 {
		t.Errorf("getGoroutineIDFast() allocates %.2f times per call (expected 0)", allocs)
	}
}

// TestGetGoroutineIDSlow_HasAllocations verifies slow path allocates as expected.
func TestGetGoroutineIDSlow_HasAllocations(t *testing.T) {
	// Measure allocations.
	allocs := testing.AllocsPerRun(100, func() {
		_ = getGoroutineIDSlow()
	})

	// Should allocate exactly once per call (the 64-byte buffer).
	if allocs < 0.9 || allocs > 1.1 {
		t.Errorf("getGoroutineIDSlow() allocates %.2f times per call (expected ~1)", allocs)
	}
}
