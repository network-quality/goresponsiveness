package ms

import (
	"reflect"
	"testing"

	"github.com/network-quality/goresponsiveness/utilities"
)

func Test_InfiniteValues(test *testing.T) {
	series := NewInfiniteMathematicalSeries[float64]()
	shouldMatch := make([]float64, 0)
	previous := float64(1.0)
	for _ = range utilities.Iota(1, 80) {
		previous *= 1.059
		series.AddElement(float64(previous))
		shouldMatch = append(shouldMatch, previous)
	}

	if !reflect.DeepEqual(shouldMatch, series.Values()) {
		test.Fatalf("Values() on infinite mathematical series does not work.")
	}
}
func Test_InfiniteSequentialIncreasesAlwaysLessThan(test *testing.T) {
	series := NewInfiniteMathematicalSeries[float64]()
	previous := float64(1.0)
	for _ = range utilities.Iota(1, 80) {
		previous *= 1.059
		series.AddElement(float64(previous))
	}
	if islt, maxSeqIncrease := series.AllSequentialIncreasesLessThan(6.0); !islt {
		test.Fatalf("(infinite) Sequential increases are not always less than 6.0 (%f).", maxSeqIncrease)
	}
}
func Test_CappedTooFewInstantsSequentialIncreasesLessThanAlwaysFalse(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](500)
	series.AddElement(0.0)
	if islt, _ := series.AllSequentialIncreasesLessThan(6.0); islt {
		test.Fatalf("(infinite) 0 elements in a series should always yield false when asking if sequential increases are less than a value.")
	}
}

func Test_Infinite_degenerate_percentile_too_high(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int]()
	if series.Percentile(101) != 0 {
		test.Fatalf("(infinite) Series percentile of 101 failed.")
	}
}
func Test_Infinite_degenerate_percentile_too_low(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int]()
	if series.Percentile(-1) != 0 {
		test.Fatalf("(infinite) Series percentile of -1 failed.")
	}
}
func Test_Infinite90_percentile(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int]()
	series.AddElement(10)
	series.AddElement(9)
	series.AddElement(8)
	series.AddElement(7)
	series.AddElement(6)
	series.AddElement(5)
	series.AddElement(4)
	series.AddElement(3)
	series.AddElement(2)
	series.AddElement(1)

	if series.Percentile(90) != 10 {
		test.Fatalf("(infinite) Series 90th percentile of 0 ... 10 failed: Expected 10 got %v.", series.Percentile(90))
	}
}

func Test_Infinite90_percentile_reversed(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int64]()
	series.AddElement(1)
	series.AddElement(2)
	series.AddElement(3)
	series.AddElement(4)
	series.AddElement(5)
	series.AddElement(6)
	series.AddElement(7)
	series.AddElement(8)
	series.AddElement(9)
	series.AddElement(10)

	if series.Percentile(90) != 10 {
		test.Fatalf("(infinite) Series 90th percentile of 0 ... 10 failed: Expected 10 got %v.", series.Percentile(90))
	}
}

func Test_Infinite50_percentile_jumbled(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int64]()
	series.AddElement(7)
	series.AddElement(2)
	series.AddElement(15)
	series.AddElement(27)
	series.AddElement(5)
	series.AddElement(52)
	series.AddElement(18)
	series.AddElement(23)
	series.AddElement(11)
	series.AddElement(12)

	if series.Percentile(50) != 15 {
		test.Fatalf("(infinite) Series 50 percentile of a jumble of numbers failed: Expected 15 got %v.", series.Percentile(50))
	}
}

func Test_InfiniteDoubleSidedTrimmedMean_jumbled(test *testing.T) {
	series := NewInfiniteMathematicalSeries[int64]()
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
	series.AddElement(22)
	series.AddElement(17)
	series.AddElement(14)
	series.AddElement(9)
	series.AddElement(100)
	series.AddElement(72)
	series.AddElement(91)
	series.AddElement(43)
	series.AddElement(37)
	series.AddElement(62)

	trimmed := series.DoubleSidedTrim(10)

	if trimmed.Len() != 16 {
		test.Fatalf("Capped series is not of the proper size. Expected %v and got %v", 16, trimmed.Len())
	}

	prev := int64(0)
	for _, v := range trimmed.Values() {
		if !(prev <= v) {
			test.Fatalf("Not sorted: %v is not less than or equal to %v\n", prev, v)
		}
		prev = v
	}
}

func Test_CappedSequentialIncreasesAlwaysLessThan(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](40)
	previous := float64(1.0)
	for _ = range utilities.Iota(1, 80) {
		previous *= 1.059
		series.AddElement(float64(previous))
	}
	if islt, maxSeqIncrease := series.AllSequentialIncreasesLessThan(6.0); !islt {
		test.Fatalf("Sequential increases are not always less than 6.0 (%f).", maxSeqIncrease)
	}
}

func Test_CappedSequentialIncreasesAlwaysLessThanWithWraparound(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](20)
	previous := float64(1.0)
	for range utilities.Iota(1, 20) {
		previous *= 1.15
		series.AddElement(float64(previous))
	}

	// All those measurements should be ejected by the following
	// loop!
	for range utilities.Iota(1, 20) {
		previous *= 1.10
		series.AddElement(float64(previous))
	}

	if islt, maxSeqIncrease := series.AllSequentialIncreasesLessThan(11.0); !islt {
		test.Fatalf("Sequential increases are not always less than 11.0 in wraparound situation (%f v 11.0).", maxSeqIncrease)
	}
}

func Test_CappedSequentialIncreasesAlwaysLessThanWithWraparoundInverse(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](20)
	previous := float64(1.0)
	for range utilities.Iota(1, 20) {
		previous *= 1.15
		series.AddElement(float64(previous))
	}

	// *Not* all those measurements should be ejected by the following
	// loop!
	for range utilities.Iota(1, 15) {
		previous *= 1.10
		series.AddElement(float64(previous))
	}

	if islt, maxSeqIncrease := series.AllSequentialIncreasesLessThan(11.0); islt {
		test.Fatalf("Sequential increases are (unexpectedly) always less than 11.0 in wraparound situation: %f v 11.0.", maxSeqIncrease)
	}
}

func Test_CappedStandardDeviationCalculation(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](5)
	// 5.7, 1.0, 8.6, 7.4, 2.2
	series.AddElement(5.7)
	series.AddElement(1.0)
	series.AddElement(8.6)
	series.AddElement(7.4)
	series.AddElement(2.2)

	if _, sd := series.StandardDeviation(); !utilities.ApproximatelyEqual(2.93, sd, 0.01) {
		test.Fatalf("Standard deviation max calculation failed: %v.", sd)
	} else {
		test.Logf("Standard deviation calculation result: %v", sd)
	}
}

func Test_CappedRotatingValues(test *testing.T) {
	series := NewCappedMathematicalSeries[int](5)

	series.AddElement(1)
	series.AddElement(2)
	series.AddElement(3)
	series.AddElement(4)
	series.AddElement(5)

	series.AddElement(6)
	series.AddElement(7)

	if !reflect.DeepEqual([]int{6, 7, 3, 4, 5}, series.Values()) {
		test.Fatalf("Adding values does not properly erase earlier values.")
	}
}
func Test_CappedLen(test *testing.T) {
	series := NewCappedMathematicalSeries[int](5)

	series.AddElement(1)
	series.AddElement(2)
	series.AddElement(3)
	series.AddElement(4)
	series.AddElement(5)

	series.AddElement(6)
	series.AddElement(7)

	if series.Len() != 5 {
		test.Fatalf("Series size calculations failed.")
	}
}

func Test_Capped_degenerate_percentile_too_high(test *testing.T) {
	series := NewCappedMathematicalSeries[int](21)
	if series.Percentile(101) != 0 {
		test.Fatalf("Series percentile of 101 failed.")
	}
}
func Test_Capped_degenerate_percentile_too_low(test *testing.T) {
	series := NewCappedMathematicalSeries[int](21)
	if series.Percentile(-1) != 0 {
		test.Fatalf("Series percentile of -1 failed.")
	}
}
func Test_Capped90_percentile(test *testing.T) {
	series := NewCappedMathematicalSeries[int](10)
	series.AddElement(10)
	series.AddElement(9)
	series.AddElement(8)
	series.AddElement(7)
	series.AddElement(6)
	series.AddElement(5)
	series.AddElement(4)
	series.AddElement(3)
	series.AddElement(2)
	series.AddElement(1)

	if series.Percentile(90) != 10 {
		test.Fatalf("Series 90th percentile of 0 ... 10 failed: Expected 10 got %v.", series.Percentile(90))
	}
}

func Test_Capped90_percentile_reversed(test *testing.T) {
	series := NewCappedMathematicalSeries[int64](10)
	series.AddElement(1)
	series.AddElement(2)
	series.AddElement(3)
	series.AddElement(4)
	series.AddElement(5)
	series.AddElement(6)
	series.AddElement(7)
	series.AddElement(8)
	series.AddElement(9)
	series.AddElement(10)

	if series.Percentile(90) != 10 {
		test.Fatalf("Series 90th percentile of 0 ... 10 failed: Expected 10 got %v.", series.Percentile(90))
	}
}

func Test_Capped50_percentile_jumbled(test *testing.T) {
	series := NewCappedMathematicalSeries[int64](10)
	series.AddElement(7)
	series.AddElement(2)
	series.AddElement(15)
	series.AddElement(27)
	series.AddElement(5)
	series.AddElement(52)
	series.AddElement(18)
	series.AddElement(23)
	series.AddElement(11)
	series.AddElement(12)

	if series.Percentile(50) != 15 {
		test.Fatalf("Series 50 percentile of a jumble of numbers failed: Expected 15 got %v.", series.Percentile(50))
	}
}

func Test_CappedDoubleSidedTrimmedMean_jumbled(test *testing.T) {
	series := NewCappedMathematicalSeries[int64](10)
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

	if trimmed.Len() != 8 {
		test.Fatalf("Capped series is not of the proper size. Expected %v and got %v", 8, trimmed.Len())
	}

	prev := int64(0)
	for _, v := range trimmed.Values() {
		if !(prev <= v) {
			test.Fatalf("Not sorted: %v is not less than or equal to %v\n", prev, v)
		}
		prev = v
	}
}
