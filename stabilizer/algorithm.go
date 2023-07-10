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
	"github.com/network-quality/goresponsiveness/series"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type MeasurementStablizer[Data constraints.Float | constraints.Integer, Bucket constraints.Ordered] struct {
	// The number of instantaneous measurements in the current interval could be infinite (Forever).
	instantaneousses series.WindowSeries[Data, Bucket]
	// There are a fixed, finite number of aggregates (WindowOnly).
	aggregates                 series.WindowSeries[series.WindowSeries[Data, Bucket], int]
	stabilityStandardDeviation float64
	trimmingLevel              uint
	m                          sync.Mutex
	dbgLevel                   debug.DebugLevel
	dbgConfig                  *debug.DebugWithPrefix
	units                      string
	currentInterval            int
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

func NewStabilizer[Data constraints.Float | constraints.Integer, Bucket constraints.Ordered](
	mad int,
	sdt float64,
	trimmingLevel uint,
	units string,
	debugLevel debug.DebugLevel,
	debug *debug.DebugWithPrefix,
) MeasurementStablizer[Data, Bucket] {
	return MeasurementStablizer[Data, Bucket]{
		instantaneousses: series.NewWindowSeries[Data, Bucket](series.Forever, 0),
		aggregates: series.NewWindowSeries[
			series.WindowSeries[Data, Bucket], int](series.WindowOnly, mad),
		stabilityStandardDeviation: sdt,
		trimmingLevel:              trimmingLevel,
		units:                      units,
		currentInterval:            0,
		dbgConfig:                  debug,
		dbgLevel:                   debugLevel,
	}
}

func (r3 *MeasurementStablizer[Data, Bucket]) Reserve(bucket Bucket) {
	r3.m.Lock()
	defer r3.m.Unlock()
	r3.instantaneousses.Reserve(bucket)
}

func (r3 *MeasurementStablizer[Data, Bucket]) AddMeasurement(bucket Bucket, measurement Data) {
	r3.m.Lock()
	defer r3.m.Unlock()

	// Fill in the bucket in the current interval.
	if err := r3.instantaneousses.Fill(bucket, measurement); err != nil {
		if debug.IsDebug(r3.dbgLevel) {
			fmt.Printf("%s: A bucket (with id %v) does not exist in the isntantaneousses.\n",
				r3.dbgConfig.String(),
				bucket)
		}
	}

	// The result may have been retired from the current interval. Look in the older series
	// to fill it in there if it is.
	r3.aggregates.ForEach(func(b int, md *utilities.Optional[series.WindowSeries[Data, Bucket]]) {
		if utilities.IsSome[series.WindowSeries[Data, Bucket]](*md) {
			md := utilities.GetSome[series.WindowSeries[Data, Bucket]](*md)
			if err := md.Fill(bucket, measurement); err != nil {
				if debug.IsDebug(r3.dbgLevel) {
					fmt.Printf("%s: A bucket (with id %v) does not exist in a historical window.\n",
						r3.dbgConfig.String(),
						bucket)
				}
			}
		}
	})

	/*
		// Add this instantaneous measurement to the mix of the MAD previous instantaneous measurements.
		r3.instantaneousses.Fill(bucket, measurement)
		// Calculate the moving average of the MAD previous instantaneous measurements (what the
		// algorithm calls moving average aggregate throughput at interval p) and add it to
		// the mix of MAD previous moving averages.

		r3.aggregates.AutoFill(r3.instantaneousses.CalculateAverage())

		if debug.IsDebug(r3.dbgLevel) {
			fmt.Printf(
				"%s: MA: %f Mbps (previous %d intervals).\n",
				r3.dbgConfig.String(),
				r3.aggregates.CalculateAverage(),
				r3.aggregates.Len(),
			)
		}
	*/
}

func (r3 *MeasurementStablizer[Data, Bucket]) Interval() {
	r3.m.Lock()
	defer r3.m.Unlock()

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: stability interval marked (transitioning from %d to %d).\n",
			r3.dbgConfig.String(),
			r3.currentInterval,
			r3.currentInterval+1,
		)
	}

	// At the interval boundary, move the instantaneous series to
	// the aggregates and start a new instantaneous series.
	r3.aggregates.Reserve(r3.currentInterval)
	r3.aggregates.Fill(r3.currentInterval, r3.instantaneousses)

	r3.instantaneousses = series.NewWindowSeries[Data, Bucket](series.Forever, 0)
	r3.currentInterval++
}

func (r3 *MeasurementStablizer[Data, Bucket]) IsStable() bool {
	r3.m.Lock()
	defer r3.m.Unlock()

	if debug.IsDebug(r3.dbgLevel) {
		fmt.Printf(
			"%s: Determining stability in the %d th interval.\n",
			r3.dbgConfig.String(),
			r3.currentInterval,
		)
	}
	// Determine if
	// a) All the aggregates have values,
	// b) All the aggregates are complete.
	allComplete := true
	r3.aggregates.ForEach(func(b int, md *utilities.Optional[series.WindowSeries[Data, Bucket]]) {
		if utilities.IsSome[series.WindowSeries[Data, Bucket]](*md) {
			md := utilities.GetSome[series.WindowSeries[Data, Bucket]](*md)
			allComplete = md.Complete()
			if debug.IsDebug(r3.dbgLevel) {
				fmt.Printf("%s\n", md.String())
			}
		} else {
			allComplete = false
		}
		if debug.IsDebug(r3.dbgLevel) {
			fmt.Printf(
				"%s: The aggregate for the %d th interval was %s.\n",
				r3.dbgConfig.String(),
				b,
				utilities.Conditional(allComplete, "complete", "incomplete"),
			)
		}
	})

	if !allComplete {
		return false
	}

	// Calculate the averages of each of the aggregates.
	averages := make([]float64, 0)
	r3.aggregates.ForEach(func(b int, md *utilities.Optional[series.WindowSeries[Data, Bucket]]) {
		if utilities.IsSome[series.WindowSeries[Data, Bucket]](*md) {
			md := utilities.GetSome[series.WindowSeries[Data, Bucket]](*md)
			_, average := series.CalculateAverage(md)
			averages = append(averages, average)
		}
	})

	// Calculate the standard deviation of the averages of the aggregates.
	sd := utilities.CalculateStandardDeviation(averages)

	// Take a percentage of the average of the averages of the aggregates ...
	stabilityCutoff := utilities.CalculateAverage(averages) * (r3.stabilityStandardDeviation / 100.0)
	// and compare that to the standard deviation to determine stability.
	isStable := sd <= stabilityCutoff

	return isStable
}
