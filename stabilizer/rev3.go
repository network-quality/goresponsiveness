package stabilizer

import (
	"fmt"
	"sync"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/ms"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/utilities"
)

type DataPointStabilizer struct {
	instantaneousMeasurements  *ms.MathematicalSeries[float64]
	movingAverages             *ms.MathematicalSeries[float64]
	stabilityStandardDeviation float64
	m                          sync.Mutex
	dbgLevel                   debug.DebugLevel
	dbgConfig                  *debug.DebugWithPrefix
}

type ProbeStabilizer DataPointStabilizer
type ThroughputStabilizer DataPointStabilizer

// Stabilizer parameters:
// 1. I: The number of previous instantaneous measurements to consider when generating
//       the so-called instantaneous moving averages.
// 2. K: The number of instantaneous moving averages to consider when determining stability.
// 3: S: The standard deviation cutoff used to determine stability among the K preceding
//       moving averages of a measurement.

func NewProbeStabilizer(i int, k int, s float64, debugLevel debug.DebugLevel, debug *debug.DebugWithPrefix) ProbeStabilizer {
	return ProbeStabilizer{instantaneousMeasurements: ms.NewMathematicalSeries[float64](i),
		movingAverages:             ms.NewMathematicalSeries[float64](k),
		stabilityStandardDeviation: s,
		dbgConfig:                  debug,
		dbgLevel:                   debugLevel}
}

func (r3 *ProbeStabilizer) AddMeasurement(measurement rpm.ProbeDataPoint) {
	r3.m.Lock()
	defer r3.m.Unlock()
	// Add this instantaneous measurement to the mix of the I previous instantaneous measurements.
	r3.instantaneousMeasurements.AddElement(measurement.Duration.Seconds())
	// Calculate the moving average of the I previous instantaneous measurements and add it to
	// the mix of K previous moving averages.
	r3.movingAverages.AddElement(r3.instantaneousMeasurements.CalculateAverage())

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: MA: %f ns (previous %d intervals).\n",
			r3.dbgConfig.String(),
			r3.movingAverages.CalculateAverage(),
			r3.movingAverages.Size(),
		)
	}
}

func (r3 *ProbeStabilizer) IsStable() bool {
	// calculate whether the standard deviation of the K previous moving averages falls below S.
	islt, stddev := r3.movingAverages.StandardDeviationLessThan(r3.stabilityStandardDeviation)

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: Standard Deviation: %f s; Is Normally Distributed? %v).\n",
			r3.dbgConfig.String(),
			stddev,
			r3.movingAverages.IsNormallyDistributed(),
		)
		fmt.Printf("%s: Values: ", r3.dbgConfig.String())
		for _, v := range r3.movingAverages.Values() {
			fmt.Printf("%v, ", v)
		}
		fmt.Printf("\n")
	}
	return islt
}

func NewThroughputStabilizer(i int, k int, s float64, debugLevel debug.DebugLevel, debug *debug.DebugWithPrefix) ThroughputStabilizer {
	return ThroughputStabilizer{instantaneousMeasurements: ms.NewMathematicalSeries[float64](i),
		movingAverages:             ms.NewMathematicalSeries[float64](k),
		stabilityStandardDeviation: s,
		dbgConfig:                  debug,
		dbgLevel:                   debugLevel}
}

func (r3 *ThroughputStabilizer) AddMeasurement(measurement rpm.ThroughputDataPoint) {
	r3.m.Lock()
	defer r3.m.Unlock()
	// Add this instantaneous measurement to the mix of the I previous instantaneous measurements.
	r3.instantaneousMeasurements.AddElement(utilities.ToMbps(measurement.Throughput))
	// Calculate the moving average of the I previous instantaneous measurements and add it to
	// the mix of K previous moving averages.
	r3.movingAverages.AddElement(r3.instantaneousMeasurements.CalculateAverage())

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: MA: %f Mbps (previous %d intervals).\n",
			r3.dbgConfig.String(),
			r3.movingAverages.CalculateAverage(),
			r3.movingAverages.Size(),
		)
	}
}

func (r3 *ThroughputStabilizer) IsStable() bool {
	// calculate whether the standard deviation of the K previous moving averages falls below S.
	islt, stddev := r3.movingAverages.StandardDeviationLessThan(r3.stabilityStandardDeviation)

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: Standard Deviation: %f Mbps; Is Normally Distributed? %v).\n",
			r3.dbgConfig.String(),
			stddev,
			r3.movingAverages.IsNormallyDistributed(),
		)
		fmt.Printf("%s: Values: ", r3.dbgConfig.String())
		for _, v := range r3.movingAverages.Values() {
			fmt.Printf("%v, ", v)
		}
		fmt.Printf("\n")
	}
	return islt
}
