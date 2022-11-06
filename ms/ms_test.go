package ms

import (
	"testing"

	"github.com/network-quality/goresponsiveness/utilities"
)

func Test_TooFewInstantsSequentialIncreasesLessThanAlwaysFalse(test *testing.T) {
	series := NewMathematicalSeries[float64](500)
	series.AddElement(0.0)
	if islt, _ := series.AllSequentialIncreasesLessThan(6.0); islt {
		test.Fatalf("Too few instants should always yield false when asking if sequential increases are less than a value.")
	}
}

func Test_SequentialIncreasesAlwaysLessThan(test *testing.T) {
	series := NewMathematicalSeries[float64](40)
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
	series := NewMathematicalSeries[float64](20)
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
	series := NewMathematicalSeries[float64](20)
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

func Test_StandardDeviationLessThan_Float(test *testing.T) {
	series := NewMathematicalSeries[float64](5)
	// 5.7, 1.0, 8.6, 7.4, 2.2
	series.AddElement(5.7)
	series.AddElement(1.0)
	series.AddElement(8.6)
	series.AddElement(7.4)
	series.AddElement(2.2)

	if islt, sd := series.StandardDeviationLessThan(2.94); !islt {
		test.Fatalf("Standard deviation max calculation failed: %v.", sd)
	} else {
		test.Logf("Standard deviation calculation result: %v", sd)
	}
}
