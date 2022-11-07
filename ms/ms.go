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

package ms

import (
	"math"

	"github.com/network-quality/goresponsiveness/saturating"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type MathematicalSeries[T constraints.Float | constraints.Integer] struct {
	elements_count int
	elements       []T
	index          int
	divisor        *saturating.SaturatingInt
}

func NewMathematicalSeries[T constraints.Float | constraints.Integer](instants_count int) *MathematicalSeries[T] {
	return &MathematicalSeries[T]{
		elements:       make([]T, instants_count),
		elements_count: instants_count,
		divisor:        saturating.NewSaturatingInt(instants_count),
	}
}

func (ma *MathematicalSeries[T]) AddElement(measurement T) {
	ma.elements[ma.index] = measurement
	ma.divisor.Add(1)
	// Invariant: ma.index always points to the oldest measurement
	ma.index = (ma.index + 1) % ma.elements_count
}

func (ma *MathematicalSeries[T]) CalculateAverage() float64 {
	total := T(0)
	for i := 0; i < ma.elements_count; i++ {
		total += ma.elements[i]
	}
	return float64(total) / float64(ma.divisor.Value())
}

func (ma *MathematicalSeries[T]) AllSequentialIncreasesLessThan(limit float64) (_ bool, maximumSequentialIncrease float64) {

	// If we have not yet accumulated a complete set of intervals,
	// this is false.
	if ma.divisor.Value() != ma.elements_count {
		return false, 0
	}

	// Invariant: ma.index always points to the oldest (see AddMeasurement
	// above)
	oldestIndex := ma.index
	previous := ma.elements[oldestIndex]
	maximumSequentialIncrease = 0
	for i := 1; i < ma.elements_count; i++ {
		currentIndex := (oldestIndex + i) % ma.elements_count
		current := ma.elements[currentIndex]
		percentChange := utilities.SignedPercentDifference(current, previous)
		previous = current
		if percentChange > limit {
			return false, percentChange
		}
	}
	return true, maximumSequentialIncrease
}

/*
 * N.B.: Overflow is possible -- use at your discretion!
 */
func (ma *MathematicalSeries[T]) StandardDeviationLessThan(limit T) (bool, T) {

	// If we have not yet accumulated a complete set of intervals,
	// we are always false.
	if ma.divisor.Value() != ma.elements_count {
		return false, T(0)
	}

	// From https://www.mathsisfun.com/data/standard-deviation-calculator.html
	// Yes, for real!

	// Calculate the average of the numbers ...
	average := ma.CalculateAverage()

	// Calculate the square of each of the elements' differences from the mean.
	differences_squared := make([]float64, ma.elements_count)
	for index, value := range ma.elements {
		differences_squared[index] = math.Pow(float64(value-T(average)), 2)
	}

	// The variance is the average of the squared differences.
	// So, we need to ...

	// Accumulate all those squared differences.
	sds := float64(0)
	for _, dss := range differences_squared {
		sds += dss
	}

	// And then divide that total by the number of elements
	variance := sds / float64(ma.divisor.Value())

	// Finally, the standard deviation is the square root
	// of the variance.
	sd := T(math.Sqrt(variance))
	//sd := T(variance)

	return T(sd) < limit, sd
}

func (ma *MathematicalSeries[T]) IsNormallyDistributed() bool {
	_, stddev := ma.StandardDeviationLessThan(0.0)
	avg := float64(ma.CalculateAverage())

	fstddev := float64(stddev)
	within := float64(0)
	for _, v := range ma.Values() {
		if (avg-fstddev) <= float64(v) && float64(v) <= (avg+fstddev) {
			within++
		}
	}
	return within/float64(ma.divisor.Value()) >= 0.68
}

func (ma *MathematicalSeries[T]) Values() []T {
	return ma.elements
}
