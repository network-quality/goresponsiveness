package ma

import (
	"github.com/hawkinsw/goresponsiveness/saturating"
)

// Convert this to a Type Parameterized interface when they are available
// in Go (1.18).
type MovingAverage struct {
	intervals int
	instants  []uint64
	index     int
	divisor   *saturating.SaturatingInt
}

func NewMovingAverage(intervals int) *MovingAverage {
	return &MovingAverage{instants: make([]uint64, intervals), intervals: intervals, divisor: saturating.NewSaturatingInt(intervals)}
}

func (ma *MovingAverage) AddMeasurement(measurement uint64) {
	ma.instants[ma.index] = measurement
	ma.divisor.Add(1)
	ma.index = (ma.index + 1) % ma.intervals
}

func (ma *MovingAverage) CalculateAverage() float64 {
	total := uint64(0)
	for i := 0; i < ma.intervals; i++ {
		total += ma.instants[i]
	}
	return float64(total) / float64(ma.divisor.Value())
}
