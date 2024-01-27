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
package series

import (
	"reflect"
	"sync"
	"testing"
	"time"

	RPMTesting "github.com/network-quality/goresponsiveness/testing"
	"github.com/network-quality/goresponsiveness/utilities"
)

func TestNextIndex(t *testing.T) {
	wsi := newWindowSeriesWindowOnlyImpl[int, int](4)

	// Calling internal functions must be done with the lock held!
	wsi.lock.Lock()
	defer wsi.lock.Unlock()

	idx := wsi.nextIndex(wsi.latestIndex)
	if idx != 1 {
		t.Fatalf("nextIndex is wrong (1)")
	}
	wsi.latestIndex = idx

	idx = wsi.nextIndex(wsi.latestIndex)
	if idx != 2 {
		t.Fatalf("nextIndex is wrong (2)")
	}
	wsi.latestIndex = idx

	idx = wsi.nextIndex(wsi.latestIndex)
	if idx != 3 {
		t.Fatalf("nextIndex is wrong (3)")
	}
	wsi.latestIndex = idx

	idx = wsi.nextIndex(wsi.latestIndex)
	if idx != 0 {
		t.Fatalf("nextIndex is wrong (0)")
	}
	wsi.latestIndex = idx

	idx = wsi.nextIndex(wsi.latestIndex)
	if idx != 1 {
		t.Fatalf("nextIndex is wrong (1)")
	}
	wsi.latestIndex = idx
}

func TestNextIndexUnlocked(t *testing.T) {
	wsi := newWindowSeriesWindowOnlyImpl[int, int](4)

	panicingTest := func() {
		wsi.nextIndex(wsi.latestIndex)
	}

	if !RPMTesting.DidPanic(panicingTest) {
		t.Fatalf("Expected a call to nextIndex (without the lock held) to panic but it did not")
	}
}

func TestSimpleWindowComplete(t *testing.T) {
	wsi := newWindowSeriesWindowOnlyImpl[int, int](4)
	if wsi.Complete() {
		t.Fatalf("Window should not be complete.")
	}
	wsfImpl := newWindowSeriesForeverImpl[int, int]()
	wsfImpl.Reserve(1)
	if wsfImpl.Complete() {
		t.Fatalf("Window should not be complete.")
	}
}

func TestSimpleReserve(t *testing.T) {
	wswoImpl := newWindowSeriesWindowOnlyImpl[int, int](4)
	result := wswoImpl.Reserve(0)
	if result != nil {
		t.Fatalf("Reserving 1 should be a-ok!")
	}
	wsfImpl := newWindowSeriesForeverImpl[int, int]()
	result = wsfImpl.Reserve(0)
	if result != nil {
		t.Fatalf("Reserving 1 should be a-ok!")
	}
}

func Test_ForeverValues(test *testing.T) {
	series := newWindowSeriesForeverImpl[float64, int]()
	shouldMatch := make([]utilities.Optional[float64], 0)
	previous := float64(1.0)
	for i := range utilities.Iota(1, 81) {
		previous *= 1.059
		series.Reserve(i)
		series.Fill(i, float64(previous))
		shouldMatch = append(shouldMatch, utilities.Some[float64](previous))
	}

	if !reflect.DeepEqual(shouldMatch, series.GetValues()) {
		test.Fatalf("Values() on infinite mathematical series does not work.")
	}
}

func Test_WindowOnly_no_values_getvalues(test *testing.T) {
	expectedLen := 5
	series := newWindowSeriesWindowOnlyImpl[float64, int](5)
	result := series.GetValues()
	allZeros := true
	for _, v := range result {
		if utilities.IsSome(v) {
			allZeros = false
			break
		}
	}
	if len(result) != expectedLen {
		test.Fatalf("GetValues of empty window-only series returned list with incorrect size.")
	}
	if !allZeros {
		test.Fatalf("GetValues of empty window-only series returned list with some values.")
	}
}

func Test_WindowOnlySequentialIncreasesAlwaysLessThan(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](10)
	previous := float64(1.0)
	for i := range utilities.Iota(1, 11) {
		previous *= 1.5
		series.Reserve(i)
		series.Fill(i, float64(previous))
	}
	if complete, islt, maxSeqIncrease := AllSequentialIncreasesLessThan[float64, int](series,
		100); !complete || maxSeqIncrease != 50.0 || !islt {
		test.Fatalf(
			"(Window Only) Sequential increases are not always less than 100 (%v, %v, %f ).",
			complete, islt, maxSeqIncrease,
		)
	}
}

func Test_WindowOnlyTooFewInstantsSequentialIncreasesLessThanAlwaysFalse(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](500)
	series.Reserve(1)
	series.Fill(1, 0.0)
	if complete, islt, _ := AllSequentialIncreasesLessThan[float64, int](series, 0.0); complete || islt {
		test.Fatalf(
			"(Window Only) 0 elements in a series should always yield false when asking if sequential increases are less than a value.",
		)
	}
}

func Test_Forever_Complete(test *testing.T) {
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Fill(1, 10)
	if !series.Complete() {
		test.Fatalf("(infinite) Series with one element and a window size of 1 is not complete.")
	}
}

func Test_Forever_CompleteN(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](10)
	previous := float64(1.0)
	for i := range utilities.Iota(1, 11) {
		previous *= 1.5
		series.Reserve(i)
		series.Fill(i, float64(previous))
	}
	if !series.Complete() {
		test.Fatalf("(infinite) Series with one element and a window size of 2 is complete.")
	}
}

func Test_Forever_degenerate_percentile_too_high(test *testing.T) {
	series := newWindowSeriesForeverImpl[int, int]()
	if complete, result := Percentile[int, int](series, 101); !complete || result != 0.0 {
		test.Fatalf("(infinite) Series percentile of 101 failed.")
	}
}

func Test_Forever_degenerate_percentile_too_low(test *testing.T) {
	series := newWindowSeriesForeverImpl[int, int]()
	if complete, result := Percentile[int, int](series, 0); !complete || result != 0.0 {
		test.Fatalf("(infinite) Series percentile of -1 failed.")
	}
}

func Test_Forever_degenerate_percentile_no_values(test *testing.T) {
	series := newWindowSeriesForeverImpl[int, int]()
	if complete, p := Percentile[int, int](series, 50); !complete || p != 0 {
		test.Fatalf("empty series percentile of 50 failed.")
	}
}

///////////

func Test_Forever90_percentile(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 10)
	series.Fill(9, 9)
	series.Fill(8, 8)
	series.Fill(7, 7)
	series.Fill(6, 6)
	series.Fill(5, 5)
	series.Fill(4, 4)
	series.Fill(3, 3)
	series.Fill(2, 2)
	series.Fill(1, 1)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever90_WindowOnly_percentile(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 10)
	series.Fill(9, 9)
	series.Fill(8, 8)
	series.Fill(7, 7)
	series.Fill(6, 6)
	series.Fill(5, 5)
	series.Fill(4, 4)
	series.Fill(3, 3)
	series.Fill(2, 2)
	series.Fill(1, 1)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever90_percentile_reversed(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 1)
	series.Fill(9, 2)
	series.Fill(8, 3)
	series.Fill(7, 4)
	series.Fill(6, 5)
	series.Fill(5, 6)
	series.Fill(4, 7)
	series.Fill(3, 8)
	series.Fill(2, 9)
	series.Fill(1, 10)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever50_percentile_jumbled(test *testing.T) {
	var expected int64 = 15
	series := newWindowSeriesForeverImpl[int64, int]()

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(1, 7)
	series.Fill(2, 2)
	series.Fill(3, 15)
	series.Fill(4, 27)
	series.Fill(5, 5)
	series.Fill(6, 52)
	series.Fill(7, 18)
	series.Fill(8, 23)
	series.Fill(9, 11)
	series.Fill(10, 12)

	if complete, result := Percentile[int64, int](series, 50); !complete || result != expected {
		test.Fatalf(
			"Series 50 percentile of a jumble of numbers failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever90_partial_percentile(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 10)
	series.Fill(9, 9)
	series.Fill(8, 8)
	series.Fill(7, 7)
	series.Fill(6, 6)
	series.Fill(5, 5)
	series.Fill(4, 4)
	series.Fill(3, 3)
	series.Fill(2, 2)
	series.Fill(1, 1)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever90_partial_percentile_reversed(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesForeverImpl[int, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 1)
	series.Fill(9, 2)
	series.Fill(8, 3)
	series.Fill(7, 4)
	series.Fill(6, 5)
	series.Fill(5, 6)
	series.Fill(4, 7)
	series.Fill(3, 8)
	series.Fill(2, 9)
	series.Fill(1, 10)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_Forever50_partial_percentile_jumbled(test *testing.T) {
	var expected int64 = 15
	series := newWindowSeriesForeverImpl[int64, int]()

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(1, 7)
	series.Fill(2, 2)
	series.Fill(3, 15)
	series.Fill(4, 27)
	series.Fill(5, 5)
	series.Fill(6, 52)
	series.Fill(7, 18)
	series.Fill(8, 23)
	series.Fill(9, 11)
	series.Fill(10, 12)

	if complete, result := Percentile[int64, int](series, 50); !complete || result != expected {
		test.Fatalf(
			"Series 50 percentile of a jumble of numbers failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

///////////////////////

func Test_WindowOnlySequentialIncreasesAlwaysLessThanWithWraparound(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](20)
	previous := float64(1.0)
	for i := range utilities.Iota(1, 21) {
		previous *= 1.15
		series.Reserve(i)
		series.Fill(1, float64(previous))
	}

	// All those measurements should be ejected by the following
	// loop!
	for i := range utilities.Iota(1, 21) {
		previous *= 1.10
		series.Reserve(i + 20)
		series.Fill(i+20, float64(previous))
	}

	if complete, islt, maxSeqIncrease := AllSequentialIncreasesLessThan[float64, int](series,
		11.0); !complete || !islt || !utilities.ApproximatelyEqual(maxSeqIncrease, 10, 0.1) {
		test.Fatalf(
			"Sequential increases are not always less than 11.0 in wraparound situation (%f v 11.0).",
			maxSeqIncrease,
		)
	}
}

func Test_WindowOnlySequentialIncreasesAlwaysLessThanWithWraparoundInverse(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](20)
	previous := float64(1.0)
	i := 0
	for i = range utilities.Iota(1, 21) {
		previous *= 1.15
		series.Reserve(i)
		series.Fill(i, float64(previous))
	}

	// *Not* all those measurements should be ejected by the following
	// loop!
	for j := range utilities.Iota(1, 16) {
		previous *= 1.10
		series.Reserve(i + j)
		series.Fill(i+j, float64(previous))
	}

	if complete, islt, maxSeqIncrease := AllSequentialIncreasesLessThan[float64, int](series, 11.0); complete == false || islt {
		test.Fatalf(
			"Sequential increases are (unexpectedly) always less than 11.0 in wraparound situation: %f v 11.0.",
			maxSeqIncrease,
		)
	}
}

func Test_WindowOnlyStandardDeviationIncompleteCalculation(test *testing.T) {
	expected := 2.93
	series := newWindowSeriesWindowOnlyImpl[float64, int](6)
	// 5.7, 1.0, 8.6, 7.4, 2.2
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 5.7)
	series.Fill(2, 1.0)
	series.Fill(3, 8.6)
	series.Fill(4, 7.4)
	series.Fill(5, 2.2)

	if complete, sd := SeriesStandardDeviation[float64, int](series); complete != false ||
		!utilities.ApproximatelyEqual(sd, expected, 0.01) {
		test.Fatalf("Standard deviation max calculation failed: Expected: %v; Actual: %v.", expected, sd)
	} else {
		test.Logf("Standard deviation calculation result: %v", sd)
	}
}

func Test_WindowOnlyStandardDeviationCalculation(test *testing.T) {
	expected := 2.93
	series := newWindowSeriesWindowOnlyImpl[float64, int](5)
	// 5.7, 1.0, 8.6, 7.4, 2.2
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)
	series.Reserve(11)
	series.Reserve(12)
	series.Reserve(13)

	series.Fill(1, 5.7)
	series.Fill(2, 5.7)
	series.Fill(3, 5.7)
	series.Fill(4, 5.7)
	series.Fill(5, 5.7)
	series.Fill(6, 5.7)
	series.Fill(7, 5.7)
	series.Fill(8, 5.7)
	series.Fill(9, 5.7)
	series.Fill(10, 1.0)
	series.Fill(11, 8.6)
	series.Fill(12, 7.4)
	series.Fill(13, 2.2)

	if complete, sd := SeriesStandardDeviation[float64, int](series); complete != true ||
		!utilities.ApproximatelyEqual(sd, expected, 0.01) {
		test.Fatalf("Standard deviation max calculation failed: Expected: %v; Actual: %v.", expected, sd)
	}
}

func Test_WindowOnlyStandardDeviationCalculation2(test *testing.T) {
	expected := 1.41
	series := newWindowSeriesWindowOnlyImpl[float64, int](5)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	if _, sd := SeriesStandardDeviation[float64, int](series); !utilities.ApproximatelyEqual(sd, expected, 0.01) {
		test.Fatalf("Standard deviation max calculation failed: Expected: %v; Actual: %v.", expected, sd)
	} else {
		test.Logf("Standard deviation calculation result: %v", sd)
	}
}

func Test_WindowOnlyRotatingValues(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[int, int](5)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Reserve(6)
	series.Reserve(7)

	series.Fill(1, 1)
	series.Fill(2, 2)
	series.Fill(3, 3)
	series.Fill(4, 4)
	series.Fill(5, 5)

	series.Fill(6, 6)
	series.Fill(7, 7)

	actual := utilities.Fmap(series.GetValues(), func(i utilities.Optional[int]) int {
		return utilities.GetSome[int](i)
	})
	if !reflect.DeepEqual([]int{7, 6, 5, 4, 3}, actual) {
		test.Fatalf("Adding values does not properly erase earlier values.")
	}
}

func Test_WindowOnly_degenerate_percentile_too_high(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[int, int](21)
	if complete, p := Percentile[int, int](series, 101); complete != false || p != 0 {
		test.Fatalf("Series percentile of 101 failed.")
	}
}

func Test_WindowOnly_degenerate_percentile_too_low(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[int, int](21)
	if complete, p := Percentile[int, int](series, 0); complete != false || p != 0 {
		test.Fatalf("Series percentile of -1 failed.")
	}
}

func Test_WindowOnly_degenerate_percentile_no_values(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[int, int](0)
	if complete, p := Percentile[int, int](series, 50); !complete || p != 0 {
		test.Fatalf("empty series percentile of 50 failed.")
	}
}

func Test_WindowOnly90_percentile(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesWindowOnlyImpl[int, int](10)
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 10)
	series.Fill(9, 9)
	series.Fill(8, 8)
	series.Fill(7, 7)
	series.Fill(6, 6)
	series.Fill(5, 5)
	series.Fill(4, 4)
	series.Fill(3, 3)
	series.Fill(2, 2)
	series.Fill(1, 1)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_WindowOnly90_percentile_reversed(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesWindowOnlyImpl[int, int](10)
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 1)
	series.Fill(9, 2)
	series.Fill(8, 3)
	series.Fill(7, 4)
	series.Fill(6, 5)
	series.Fill(5, 6)
	series.Fill(4, 7)
	series.Fill(3, 8)
	series.Fill(2, 9)
	series.Fill(1, 10)

	if complete, result := Percentile[int, int](series, 90); !complete || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_WindowOnly50_percentile_jumbled(test *testing.T) {
	var expected int64 = 15
	series := newWindowSeriesWindowOnlyImpl[int64, int](10)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(1, 7)
	series.Fill(2, 2)
	series.Fill(3, 15)
	series.Fill(4, 27)
	series.Fill(5, 5)
	series.Fill(6, 52)
	series.Fill(7, 18)
	series.Fill(8, 23)
	series.Fill(9, 11)
	series.Fill(10, 12)

	if complete, result := Percentile[int64, int](series, 50); !complete || result != expected {
		test.Fatalf(
			"Series 50 percentile of a jumble of numbers failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_WindowOnly90_partial_percentile(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesWindowOnlyImpl[int, int](20)
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 10)
	series.Fill(9, 9)
	series.Fill(8, 8)
	series.Fill(7, 7)
	series.Fill(6, 6)
	series.Fill(5, 5)
	series.Fill(4, 4)
	series.Fill(3, 3)
	series.Fill(2, 2)
	series.Fill(1, 1)

	if complete, result := Percentile[int, int](series, 90); complete != false || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_WindowOnly90_partial_percentile_reversed(test *testing.T) {
	var expected int = 10
	series := newWindowSeriesWindowOnlyImpl[int, int](20)
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(10, 1)
	series.Fill(9, 2)
	series.Fill(8, 3)
	series.Fill(7, 4)
	series.Fill(6, 5)
	series.Fill(5, 6)
	series.Fill(4, 7)
	series.Fill(3, 8)
	series.Fill(2, 9)
	series.Fill(1, 10)

	if complete, result := Percentile[int, int](series, 90); complete != false || result != expected {
		test.Fatalf(
			"Series 90th percentile of 0 ... 10 failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

func Test_WindowOnly50_partial_percentile_jumbled(test *testing.T) {
	var expected int64 = 15
	series := newWindowSeriesWindowOnlyImpl[int64, int](20)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)
	series.Reserve(7)
	series.Reserve(8)
	series.Reserve(9)
	series.Reserve(10)

	series.Fill(1, 7)
	series.Fill(2, 2)
	series.Fill(3, 15)
	series.Fill(4, 27)
	series.Fill(5, 5)
	series.Fill(6, 52)
	series.Fill(7, 18)
	series.Fill(8, 23)
	series.Fill(9, 11)
	series.Fill(10, 12)

	if complete, result := Percentile[int64, int](series, 50); complete != false || result != expected {
		test.Fatalf(
			"Series 50 percentile of a jumble of numbers failed: (Complete: %v) Expected %v got %v.", complete, expected, result)
	}
}

/*

func Test_WindowOnlyDoubleSidedTrimmedMean_jumbled(test *testing.T) {
	expected := 8
	series := newWindowSeriesWindowOnlyImpl[int64, int](10)
	series.AddElement(7)
	series.AddElement(2)
	series.AddElement(15)
	series.AddElement(27)
	series.AddElement(5)
	series.AddElement(5)
	series.AddElement(52)
	series.AddElement(18)
	series.AddElement(23)
	series.AddElement(11)
	series.AddElement(12)

	trimmed := series.DoubleSidedTrim(10)

	if trimmed.Len() != expected {
		test.Fatalf(
			"WindowOnly series is not of the proper size. Expected %v and got %v",
			expected,
			trimmed.Len(),
		)
	}

	prev := int64(0)
	for _, v := range trimmed.Values() {
		if !(prev <= v) {
			test.Fatalf("Not sorted: %v is not less than or equal to %v\n", prev, v)
		}
		prev = v
	}
}

func Test_WindowOnlyAverage(test *testing.T) {
	expected := 1.0082230220488836e+08
	series := newWindowSeriesWindowOnlyImpl[float64, int](4)
	series.AddElement(9.94747772516195e+07)
	series.AddElement(9.991286984703423e+07)
	series.AddElement(1.0285437111086299e+08)
	series.AddElement(1.0104719061003672e+08)
	if average := series.CalculateAverage(); !utilities.ApproximatelyEqual(average, 0.01, expected) {
		test.Fatalf(
			"Expected: %v; Actual: %v.", average, expected,
		)
	}
}
*/

func Test_ForeverStandardDeviationIncompleteCalculation(test *testing.T) {
	foreverExpected := 2.90
	series := newWindowSeriesForeverImpl[float64, int]()
	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)
	series.Reserve(6)

	series.Fill(1, 5.7)
	series.Fill(2, 1.0)
	series.Fill(3, 8.6)
	series.Fill(4, 7.4)
	series.Fill(5, 2.2)
	series.Fill(6, 8.0)

	if complete, sd := SeriesStandardDeviation[float64, int](series); !complete ||
		!utilities.ApproximatelyEqual(sd, foreverExpected, 0.01) {
		test.Fatalf("Standard deviation max calculation failed: Expected: %v; Actual: %v.", foreverExpected, sd)
	}
}

func Test_ForeverStandardDeviationCalculation2(test *testing.T) {
	expected := 1.41
	series := newWindowSeriesForeverImpl[float64, int]()

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	if _, sd := SeriesStandardDeviation[float64, int](series); !utilities.ApproximatelyEqual(sd, expected, 0.01) {
		test.Fatalf("Standard deviation(series) max calculation(series) failed: Expected: %v; Actual: %v.", expected, sd)
	}
}

func Test_ForeverLocking(test *testing.T) {
	series := newWindowSeriesForeverImpl[float64, int]()
	testFail := false

	series.Reserve(1)
	series.Reserve(2)

	series.Fill(1, 8)
	series.Fill(2, 9)

	wg := sync.WaitGroup{}

	counter := 0

	wg.Add(2)
	go func() {
		series.ForEach(func(b int, d *utilities.Optional[float64]) {
			// All of these ++s should be done under a single lock of the lock and, therefore,
			// the ForEach below should not start until both buckets are ForEach'd over!
			counter++
			// Make this a long wait so we know that there is no chance for a race and that
			// we are really testing what we mean to test!
			time.Sleep(time.Second * 5)
		})
		wg.Done()
	}()

	time.Sleep(1 * time.Second)

	go func() {
		series.ForEach(func(b int, d *utilities.Optional[float64]) {
			if counter != 2 {
				testFail = true
			}
		})
		wg.Done()
	}()

	wg.Wait()

	if testFail {
		test.Fatalf("Mutual exclusion checks did not properly lock out parallel ForEach operations.")
	}
}

func Test_ForeverGetBucketBounds(test *testing.T) {
	series := newWindowSeriesForeverImpl[float64, int]()

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	lower, upper := series.GetBucketBounds()
	if lower != 1 || upper != 5 {
		test.Fatalf("expected a lower of 1 and upper of 5; got %v and %v, respectively!\n", lower, upper)
	}
}

func Test_WindowGetBucketBounds(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](3)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	lower, upper := series.GetBucketBounds()
	if lower != 3 || upper != 5 {
		test.Fatalf("expected a lower of 3 and upper of 5; got %v and %v, respectively!\n", lower, upper)
	}
}

func Test_ForeverBucketBoundsEmpty(test *testing.T) {
	series := newWindowSeriesForeverImpl[float64, int]()

	lower, upper := series.GetBucketBounds()
	if lower != 0 || upper != 0 {
		test.Fatalf("expected a lower of 0 and upper of 0; got %v and %v, respectively!\n", lower, upper)
	}
}

func Test_ForeverTrimmingBucketBounds(test *testing.T) {
	series := newWindowSeriesForeverImpl[float64, int]()

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	series.SetTrimmingBucketBounds(3, 5)

	trimmedValues := series.GetValues()
	if len(trimmedValues) != 3 {
		test.Fatalf("Expected that the list would have 3 elements but it only had %v!\n", len(trimmedValues))
	}
	if utilities.GetSome(trimmedValues[0]) != 10 || utilities.GetSome(trimmedValues[1]) != 11 ||
		utilities.GetSome(trimmedValues[2]) != 12 {
		test.Fatalf("Expected values are not the actual values.\n")
	}
}

func Test_WindowTrimmingBucketBounds(test *testing.T) {
	series := newWindowSeriesWindowOnlyImpl[float64, int](5)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	series.SetTrimmingBucketBounds(3, 5)

	trimmedValues := series.GetValues()
	if len(trimmedValues) != 3 {
		test.Fatalf("Expected that the list would have 3 elements but it only had %v!\n", len(trimmedValues))
	}
	if utilities.GetSome(trimmedValues[0]) != 12 || utilities.GetSome(trimmedValues[1]) != 11 ||
		utilities.GetSome(trimmedValues[2]) != 10 {
		test.Fatalf("Expected values are not the actual values.\n")
	}
}

func Test_ForeverBoundedAppend(test *testing.T) {
	appending_series := NewWindowSeries[float64, int](Forever, 0)
	baseSeries := NewWindowSeries[float64, int](Forever, 0)

	baseSeries.Reserve(1)
	baseSeries.Fill(1, 1)
	baseSeries.Reserve(2)
	baseSeries.Fill(2, 2)
	baseSeries.Reserve(3)
	baseSeries.Fill(3, 3)

	appending_series.Reserve(4)
	appending_series.Reserve(5)
	appending_series.Reserve(6)
	appending_series.Reserve(7)
	appending_series.Reserve(8)

	appending_series.Fill(4, 8)
	appending_series.Fill(5, 9)
	appending_series.Fill(6, 10)
	appending_series.Fill(7, 11)
	appending_series.Fill(8, 12)

	appending_series.SetTrimmingBucketBounds(6, 8)

	baseSeries.BoundedAppend(&appending_series)

	if len(baseSeries.GetValues()) != 6 {
		test.Fatalf("The base series should have 6 values, but it actually has %v (bounded test)", len(baseSeries.GetValues()))
	}

	baseSeriesValues := baseSeries.GetValues()
	if utilities.GetSome(baseSeriesValues[0]) != 1 ||
		utilities.GetSome(baseSeriesValues[1]) != 2 ||
		utilities.GetSome(baseSeriesValues[2]) != 3 ||
		utilities.GetSome(baseSeriesValues[3]) != 10 ||
		utilities.GetSome(baseSeriesValues[4]) != 11 ||
		utilities.GetSome(baseSeriesValues[5]) != 12 {
		test.Fatalf("The values that should be in a series with bounded append are not there.")
	}
	baseSeries = NewWindowSeries[float64, int](Forever, 0)
	baseSeries.Append(&appending_series)
	if len(baseSeries.GetValues()) != 5 {
		test.Fatalf("The base series should have 5 values, but it actually has %v (unbounded test)", len(baseSeries.GetValues()))
	}
}

func Test_ForeverExtractBounded(test *testing.T) {
	series := NewWindowSeries[float64, int](Forever, 0)

	series.Reserve(1)
	series.Reserve(2)
	series.Reserve(3)
	series.Reserve(4)
	series.Reserve(5)

	series.Fill(1, 8)
	series.Fill(2, 9)
	series.Fill(3, 10)
	series.Fill(4, 11)
	series.Fill(5, 12)

	extracted := series.ExtractBoundedSeries()

	if len(extracted.GetValues()) != 5 {
		test.Fatalf("Expected the extracted list to have 5 values but it really has %v", len(extracted.GetValues()))
	}

	series.SetTrimmingBucketBounds(3, 5)
	extracted = series.ExtractBoundedSeries()
	if len(extracted.GetValues()) != 3 {
		test.Fatalf("Expected the extracted list to have 3 values but it really has %v", len(extracted.GetValues()))
	}
}
