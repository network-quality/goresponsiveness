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

package stabilizer

import (
	"fmt"
	"sync"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/ms"
	"golang.org/x/exp/constraints"
)

type MeasurementStablizer[T constraints.Float | constraints.Integer] struct {
	instantaneousses           ms.MathematicalSeries[T]
	aggregates                 ms.MathematicalSeries[float64]
	stabilityStandardDeviation float64
	trimmingLevel              uint
	m                          sync.Mutex
	dbgLevel                   debug.DebugLevel
	dbgConfig                  *debug.DebugWithPrefix
	units                      string
}

// Stabilizer parameters:
// 1. MAD: An all-purpose value that determines the hysteresis of various calculations
//         that will affect saturation (of either throughput or responsiveness).
// 2: SDT: The standard deviation cutoff used to determine stability among the K preceding
//         moving averages of a measurement.
// 3: TMP: The percentage by which to trim the values before calculating the standard deviation
//         to determine whether the value is within acceptable range for stability (SDT).

// Stabilizer Algorithm:
// Throughput stabilization is achieved when the standard deviation of the MAD number of the most
// recent moving averages of instantaneous measurements is within an upper bound.
//
// Yes, that *is* a little confusing:
// The user will deliver us a steady diet of measurements of the number of bytes transmitted during the immediately
// previous interval. We will keep the MAD most recent of those measurements. Every time that we get a new
// measurement, we will recalculate the moving average of the MAD most instantaneous measurements. We will call that
// the moving average aggregate throughput at interval p. We keep the MAD most recent of those values.
// If the calculated standard deviation of *those* values is less than SDT, we declare
// stability.

func NewStabilizer[T constraints.Float | constraints.Integer](
	mad uint,
	sdt float64,
	trimmingLevel uint,
	units string,
	debugLevel debug.DebugLevel,
	debug *debug.DebugWithPrefix,
) MeasurementStablizer[T] {
	return MeasurementStablizer[T]{
		instantaneousses:           ms.NewCappedMathematicalSeries[T](mad),
		aggregates:                 ms.NewCappedMathematicalSeries[float64](mad),
		stabilityStandardDeviation: sdt,
		trimmingLevel:              trimmingLevel,
		units:                      units,
		dbgConfig:                  debug,
		dbgLevel:                   debugLevel,
	}
}

func (r3 *MeasurementStablizer[T]) AddMeasurement(measurement T) {
	r3.m.Lock()
	defer r3.m.Unlock()
	// Add this instantaneous measurement to the mix of the MAD previous instantaneous measurements.
	r3.instantaneousses.AddElement(measurement)
	// Calculate the moving average of the MAD previous instantaneous measurements (what the
	// algorithm calls moving average aggregate throughput at interval p) and add it to
	// the mix of MAD previous moving averages.
	r3.aggregates.AddElement(r3.instantaneousses.CalculateAverage())

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: MA: %f Mbps (previous %d intervals).\n",
			r3.dbgConfig.String(),
			r3.aggregates.CalculateAverage(),
			r3.aggregates.Len(),
		)
	}
}

func (r3 *MeasurementStablizer[T]) IsStable() bool {
	// There are MAD number of measurements of the _moving average aggregate throughput
	// at interval p_ in movingAverages.
	isvalid, stddev := r3.aggregates.StandardDeviation()

	if !isvalid {
		// If there are not enough values in the series to be able to calculate a
		// standard deviation, then we know that we are not yet stable. Vamoose.
		return false
	}

	// Stability is determined by whether or not the standard deviation of the values
	// is within some percentage of the average.
	stabilityCutoff := r3.aggregates.CalculateAverage() * (r3.stabilityStandardDeviation / 100.0)
	isStable := stddev <= stabilityCutoff

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: Is Stable? %v; Standard Deviation: %f %s; Is Normally Distributed? %v; Standard Deviation Cutoff: %v %s).\n",
			r3.dbgConfig.String(),
			isStable,
			stddev,
			r3.units,
			r3.aggregates.IsNormallyDistributed(),
			stabilityCutoff,
			r3.units,
		)
		fmt.Printf("%s: Values: ", r3.dbgConfig.String())
		for _, v := range r3.aggregates.Values() {
			fmt.Printf("%v, ", v)
		}
		fmt.Printf("\n")
	}
	return isStable
}
