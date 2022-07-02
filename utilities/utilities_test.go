package utilities

import (
	"sync"
	"testing"
	"time"
)

func TestReadAfterCloseOnBufferedChannel(t *testing.T) {
	communication := make(chan int, 100)

	maxC := 0

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		counter := 0
		for range make([]int, 50) {
			communication <- counter
			counter++
		}
		close(communication)
		wg.Done()
	}()

	go func() {
		time.Sleep(2 * time.Second)
		for c := range communication {
			maxC = c
		}
		wg.Done()
	}()

	wg.Wait()
	if maxC != 49 {
		t.Fatalf("Did not read all sent items from a buffered channel after channel.")
	}
}
