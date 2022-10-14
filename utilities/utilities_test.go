/*
 * This file is part of Go Responsiveness.
 *
 * Go Responsiveness is free software: you can redistribute it and/or modify it under
 * the terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 2 of the License, or (at your option) any later version.
 * Go Responsiveness is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with Go Responsiveness. If not, see <https://www.gnu.org/licenses/>.
 */
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

func TestOrTimeoutStopsInfiniteLoop(t *testing.T) {
	const TimeoutTime = 2 * time.Second
	infinity := func() {
		for {
		}
	}
	timeBefore := time.Now()
	OrTimeout(infinity, TimeoutTime)
	timeAfter := time.Now()
	if timeAfter.Sub(timeBefore) < TimeoutTime {
		t.Fatalf("OrTimeout failed to keep the infinite loop running for at least %v.", TimeoutTime)
	}
}

func TestFilenameAppend(t *testing.T) {
	const basename = "testing.csv"
	const expected = "testing-appended.csv"
	result := FilenameAppend(basename, "-appended")
	if expected != result {
		t.Fatalf("%s != %s for FilenameAppend.", expected, result)
	}
}
