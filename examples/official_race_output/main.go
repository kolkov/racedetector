package main

import (
	"fmt"
	"sync"
)

var counter int

func increment(wg *sync.WaitGroup) {
	defer wg.Done()
	counter++ // Race: concurrent write without synchronization
}

func main() {
	var wg sync.WaitGroup

	// Start two goroutines that race on 'counter'
	wg.Add(2)
	go increment(&wg)
	go increment(&wg)

	wg.Wait()
	fmt.Println("Counter:", counter)
}
