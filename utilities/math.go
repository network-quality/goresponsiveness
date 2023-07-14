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

package utilities

import (
	"math"
	"sort"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
)

type Number interface {
	constraints.Float | constraints.Integer
}

func CalculateAverage[T Number](elements []T) float64 {
	total := T(0)
	for i := 0; i < len(elements); i++ {
		total += elements[i]
	}
	return float64(total) / float64(len(elements))
}

func CalculatePercentile[T Number](
	elements []T,
	p uint,
) (result T) {
	result = T(0)
	if p < 1 || p > 100 {
		return
	}

	sort.Slice(elements, func(l int, r int) bool { return elements[l] < elements[r] })
	pindex := int((float64(p) / float64(100)) * float64(len(elements)))
	if pindex >= len(elements) {
		return
	}
	result = elements[pindex]
	return
}

func Max(x, y uint64) uint64 {
	if x > y {
		return x
	}
	return y
}

func SignedPercentDifference[T constraints.Float | constraints.Integer](
	current T,
	previous T,
) (difference float64) {
	fCurrent := float64(current)
	fPrevious := float64(previous)
	return ((fCurrent - fPrevious) / fPrevious) * 100.0
}

func AbsPercentDifference(
	current float64,
	previous float64,
) (difference float64) {
	return (math.Abs(current-previous) / (float64(current+previous) / 2.0)) * float64(
		100,
	)
}

func CalculateStandardDeviation[T constraints.Float | constraints.Integer](elements []T) float64 {
	// From https://www.mathsisfun.com/data/standard-deviation-calculator.html
	// Yes, for real!

	// Calculate the average of the numbers ...
	average := CalculateAverage(elements)

	// Calculate the square of each of the elements' differences from the mean.
	differences_squared := make([]float64, len(elements))
	for index, value := range elements {
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
	variance := sds / float64(len(elements))

	// Finally, the standard deviation is the square root
	// of the variance.
	sd := float64(math.Sqrt(variance))
	// sd := T(variance)

	return sd
}

func AllSequentialIncreasesLessThan[T Number](elements []T, limit float64) (bool, float64) {
	if len(elements) < 2 {
		return false, 0.0
	}

	maximumSequentialIncrease := float64(0)
	for i := 1; i < len(elements); i++ {
		current := elements[i]
		previous := elements[i-1]
		percentChange := SignedPercentDifference(current, previous)
		if percentChange > limit {
			return false, percentChange
		}
		if percentChange > float64(maximumSequentialIncrease) {
			maximumSequentialIncrease = percentChange
		}
	}
	return true, maximumSequentialIncrease
}

// elements must already be sorted!
func TrimBy[T Number](elements []T, trim int) []T {
	numberToKeep := int(float32(len(elements)) * (float32(trim) / 100.0))

	return elements[:numberToKeep]
}

func TrimmedMean[T Number](elements []T, trim int) (float64, []T) {
	sortedElements := make([]T, len(elements))
	copy(sortedElements, elements)
	slices.Sort(sortedElements)

	trimmedElements := TrimBy(sortedElements, trim)
	return CalculateAverage(trimmedElements), trimmedElements
}
