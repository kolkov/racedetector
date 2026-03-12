// Package main demonstrates CORRECT channel-based synchronization.
//
// This program shows how Go channels provide safe communication between
// goroutines without explicit locks. Channels have built-in synchronization,
// so properly used channels should NOT trigger race detection.
//
// Usage:
//
//	racedetector run main.go
//
// Expected: No data races detected (channel synchronization is safe)
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== Channel-Based Synchronization Demo ===")
	fmt.Println()

	// Demo 1: Simple producer-consumer
	demo1ProducerConsumer()

	fmt.Println()

	// Demo 2: Worker pool pattern
	demo2WorkerPool()

	fmt.Println()

	// Demo 3: Fan-out/Fan-in pattern
	demo3FanOutFanIn()

	fmt.Println()
	fmt.Println("=== Demo Complete ===")
}

// demo1ProducerConsumer demonstrates basic channel communication
func demo1ProducerConsumer() {
	fmt.Println("--- Demo 1: Producer-Consumer ---")

	// Buffered channel for communication
	ch := make(chan int, 10)

	// Producer goroutine
	go func() {
		fmt.Println("Producer: Starting to send values...")
		for i := 1; i <= 10; i++ {
			ch <- i // Send to channel (safe: channel handles synchronization)
			fmt.Printf("Producer: Sent %d\n", i)
			time.Sleep(10 * time.Millisecond) // Simulate work
		}
		close(ch) // Signal completion
		fmt.Println("Producer: Finished, channel closed")
	}()

	// Consumer goroutine (main)
	fmt.Println("Consumer: Starting to receive values...")
	sum := 0
	for val := range ch { // Receive from channel (safe: synchronized by channel)
		sum += val
		fmt.Printf("Consumer: Received %d (sum = %d)\n", val, sum)
	}

	fmt.Printf("Consumer: Final sum = %d (expected 55)\n", sum)
	if sum == 55 {
		fmt.Println("✓ SUCCESS: Correct result (no race)")
	}
}

// demo2WorkerPool demonstrates the worker pool pattern
func demo2WorkerPool() {
	fmt.Println("--- Demo 2: Worker Pool ---")

	const numWorkers = 3
	const numJobs = 10

	jobs := make(chan int, numJobs)
	results := make(chan int, numJobs)

	// Start worker pool
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fmt.Printf("Worker %d: Started\n", id)

			// Process jobs from channel (safe: channel synchronization)
			for job := range jobs {
				fmt.Printf("Worker %d: Processing job %d\n", id, job)
				result := job * 2 // Simulate processing
				results <- result // Send result (safe: channel synchronization)
				time.Sleep(50 * time.Millisecond)
			}

			fmt.Printf("Worker %d: Finished\n", id)
		}(w)
	}

	// Send jobs
	fmt.Println("Dispatcher: Sending jobs...")
	go func() {
		for j := 1; j <= numJobs; j++ {
			jobs <- j
		}
		close(jobs) // Signal no more jobs
		fmt.Println("Dispatcher: All jobs sent")
	}()

	// Collect results in separate goroutine
	go func() {
		wg.Wait()      // Wait for all workers
		close(results) // Close results channel
	}()

	// Receive results (main goroutine)
	fmt.Println("Collector: Waiting for results...")
	var allResults []int
	for result := range results { // Safe: synchronized by channel
		allResults = append(allResults, result)
		fmt.Printf("Collector: Got result %d\n", result)
	}

	fmt.Printf("Collector: Received %d results (expected %d)\n",
		len(allResults), numJobs)
	if len(allResults) == numJobs {
		fmt.Println("✓ SUCCESS: All results collected (no race)")
	}
}

// demo3FanOutFanIn demonstrates fan-out/fan-in pattern
func demo3FanOutFanIn() {
	fmt.Println("--- Demo 3: Fan-Out/Fan-In ---")

	// Input channel
	input := make(chan int, 5)

	// Fan-out: Multiple workers processing same input
	const numWorkers = 3
	outputs := make([]chan int, numWorkers)

	for i := 0; i < numWorkers; i++ {
		outputs[i] = make(chan int)
		go func(id int, in <-chan int, out chan<- int) {
			fmt.Printf("Fan-out worker %d: Started\n", id)
			for val := range in { // Safe: channel synchronization
				result := val * val // Square the value
				fmt.Printf("Fan-out worker %d: %d -> %d\n", id, val, result)
				out <- result // Safe: channel synchronization
			}
			close(out)
			fmt.Printf("Fan-out worker %d: Finished\n", id)
		}(i, input, outputs[i])
	}

	// Fan-in: Merge results from multiple workers
	merged := make(chan int)
	go func() {
		var wg sync.WaitGroup
		for _, out := range outputs {
			wg.Add(1)
			go func(ch <-chan int) {
				defer wg.Done()
				for val := range ch { // Safe: channel synchronization
					merged <- val // Safe: channel synchronization
				}
			}(out)
		}
		wg.Wait()
		close(merged)
		fmt.Println("Fan-in: All workers completed")
	}()

	// Send input values
	go func() {
		fmt.Println("Input: Sending values...")
		for i := 1; i <= 5; i++ {
			input <- i
		}
		close(input)
		fmt.Println("Input: All values sent")
	}()

	// Collect merged results
	fmt.Println("Output: Collecting results...")
	var results []int
	for val := range merged { // Safe: channel synchronization
		results = append(results, val)
		fmt.Printf("Output: Received %d\n", val)
	}

	fmt.Printf("Output: Collected %d results (expected 5)\n", len(results))
	if len(results) == 5 {
		fmt.Println("✓ SUCCESS: All results received (no race)")
	}
}
