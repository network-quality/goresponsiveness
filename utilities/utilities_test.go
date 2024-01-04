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
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

func TestIota(t *testing.T) {
	r := Iota(6, 15)

	l := 6
	for _, vr := range r {
		if vr != l {
			log.Fatalf("Iota failed: expected %d, got %d\n", l, vr)
		}
		l++
	}
}

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

func TestWaitWithContext(t *testing.T) {
	ctxt, canceller := context.WithCancel(context.Background())
	never_true := func() bool { return false }
	mu := sync.Mutex{}
	cond := sync.NewCond(&mu)

	wg := sync.WaitGroup{}

	wg.Add(3)

	go func() {
		ContextSignaler(ctxt, 500*time.Millisecond, &never_true, cond)
		wg.Done()
	}()

	go func() {
		WaitWithContext(ctxt, &never_true, &mu, cond)
		wg.Done()
	}()

	go func() {
		time.Sleep(2 * time.Second)
		canceller()
		wg.Done()
	}()

	wg.Wait()
}

func TestPerSecondToInterval(t *testing.T) {
	if time.Second != PerSecondToInterval(1) {
		t.Fatalf("A number of nanoseconds is not equal to a second!")
	}

	if time.Second/2 != PerSecondToInterval(2) {
		t.Fatalf("Something that happens twice per second should happen every 5000ns.")
	}
}

func TestTrim(t *testing.T) {
	elements := Iota(1, 101)

	trimmedElements := TrimBy(elements, 75)

	trimmedLength := len(trimmedElements)
	trimmedLast := trimmedElements[trimmedLength-1]

	if trimmedLength != 75 || trimmedLast != 75 {
		t.Fatalf("When trimming, the length should be 75 but it is %d and/or the last element should be 75 but it is %d", trimmedLength, trimmedLast)
	}
}

func TestTrim2(t *testing.T) {
	elements := Iota(1, 11)

	trimmedElements := TrimBy(elements, 75)

	trimmedLength := len(trimmedElements)
	trimmedLast := trimmedElements[trimmedLength-1]

	if trimmedLength != 7 || trimmedLast != 7 {
		t.Fatalf("When trimming, the length should be 7 but it is %d and/or the last element should be 7 but it is %d", trimmedLength, trimmedLast)
	}
}

func TestTrim3(t *testing.T) {
	elements := Iota(1, 6)

	trimmedElements := TrimBy(elements, 101)

	trimmedLength := len(trimmedElements)
	trimmedLast := trimmedElements[trimmedLength-1]

	if trimmedLength != 5 || trimmedLast != 5 {
		t.Fatalf("When trimming, the length should be 5 but it is %d and/or the last element should be 5 but it is %d", trimmedLength, trimmedLast)
	}
}

func TestTrim4(t *testing.T) {
	elements := Iota(1, 11)

	trimmedElements := TrimBy(elements, 81)

	trimmedLength := len(trimmedElements)
	trimmedLast := trimmedElements[trimmedLength-1]

	if trimmedLength != 8 || trimmedLast != 8 {
		t.Fatalf("When trimming, the length should be 8 but it is %d and/or the last element should be 8 but it is %d", trimmedLength, trimmedLast)
	}
}

func TestTrimmedMean(t *testing.T) {
	expected := 2.5
	elements := []int{5, 4, 3, 2, 1}

	result, elements := TrimmedMean(elements, 80)
	if result != expected || len(elements) != 4 || elements[len(elements)-1] != 4 {
		t.Fatalf("The trimmed mean result %v does not match the expected value %v", result, expected)
	}
}

func TestIndentStringOneNewline(t *testing.T) {
	output := "This is my output\n"

	indendentedOutput := IndentOutput(output, 3, "--")

	if indendentedOutput != "------This is my output\n" {
		t.Fatalf("I expected the indented output to be ####%v#### but got ####%v####!", output, indendentedOutput)
	}
}

func TestIndentStringMultipleNewlines(t *testing.T) {
	output := "This is my output\n\n"

	indendentedOutput := IndentOutput(output, 3, "--")

	if indendentedOutput != "------This is my output\n------\n" {
		t.Fatalf("I expected the indented output to be ####%v#### but got ####%v####!", output, indendentedOutput)
	}
}
