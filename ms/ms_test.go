package ms

import (
	"reflect"
	"testing"

	"github.com/network-quality/goresponsiveness/utilities"
)

func Test_TooFewInstantsSequentialIncreasesLessThanAlwaysFalse(test *testing.T) {
	series := NewCappedMathematicalSeries[float64](500)
	series.AddElement(0.0)
	if islt, _ := series.AllSequentialIncreasesLessThan(6.0); islt {
		test.Fatalf("Too few instants should always yield false when asking if sequential increases are less than a value.")
	}
}

func Test_SequentialIncreasesAlwaysLessThan(test *testing.T) {
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

func Test_SequentialIncreasesAlwaysLessThanWithWraparound(test *testing.T) {
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

func Test_SequentialIncreasesAlwaysLessThanWithWraparoundInverse(test *testing.T) {
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

func Test_StandardDeviationCalculation(test *testing.T) {
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

func Test_RotatingValues(test *testing.T) {
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
func Test_Size(test *testing.T) {
	series := NewCappedMathematicalSeries[int](5)

	series.AddElement(1)
	series.AddElement(2)
	series.AddElement(3)
	series.AddElement(4)
	series.AddElement(5)

	series.AddElement(6)
	series.AddElement(7)

	if series.Size() != 5 {
		test.Fatalf("Series size calculations failed.")
	}
}

func Test_degenerate_percentile_too_high(test *testing.T) {
	series := NewCappedMathematicalSeries[int](21)
	if series.Percentile(101) != 0 {
		test.Fatalf("Series percentile of 101 failed.")
	}
}
func Test_degenerate_percentile_too_low(test *testing.T) {
	series := NewCappedMathematicalSeries[int](21)
	if series.Percentile(-1) != 0 {
		test.Fatalf("Series percentile of -1 failed.")
	}
}
func Test_90_percentile(test *testing.T) {
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

func Test_90_percentile_reversed(test *testing.T) {
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

func Test_50_percentile_jumbled(test *testing.T) {
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
