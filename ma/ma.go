package ma

import (
	"github.com/hawkinsw/goresponsiveness/saturating"
	"github.com/hawkinsw/goresponsiveness/utilities"
)

// Convert this to a Type Parameterized interface when they are available
// in Go (1.18).
type MovingAverage struct {
	intervals int
	instants  []float64
	index     int
	divisor   *saturating.SaturatingInt
}

func NewMovingAverage(intervals int) *MovingAverage {
	return &MovingAverage{instants: make([]float64, intervals), intervals: intervals, divisor: saturating.NewSaturatingInt(intervals)}
}

func (ma *MovingAverage) AddMeasurement(measurement float64) {
	ma.instants[ma.index] = measurement
	ma.divisor.Add(1)
	ma.index = (ma.index + 1) % ma.intervals
}

func (ma *MovingAverage) CalculateAverage() float64 {
	total := float64(0)
	for i := 0; i < ma.intervals; i++ {
		total += ma.instants[i]
	}
	return float64(total) / float64(ma.divisor.Value())
}

func (ma *MovingAverage) IncreasesLessThan(limit float64) bool {
	previous := ma.instants[0]
	for i := 1; i < ma.intervals; i++ {
		current := ma.instants[i]
		percentChange := utilities.SignedPercentDifference(current, previous)
		previous = current
		if percentChange > limit {
			return false
		}
	}
	return true
}
