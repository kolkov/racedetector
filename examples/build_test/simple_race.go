package main

import (
	"fmt"
	"sync"
)

func main() {
	var x int
	var wg sync.WaitGroup

	// Goroutine 1: Write to x
	wg.Add(1)
	go func() {
		defer wg.Done()
		x = 1
	}()

	// Goroutine 2: Read from x (potential race!)
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("x =", x)
	}()

	// Wait for goroutines to complete
	wg.Wait()
	fmt.Println("Done!")
}
