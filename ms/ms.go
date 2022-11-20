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
	"sort"

	"github.com/network-quality/goresponsiveness/saturating"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type MathematicalSeries[T constraints.Float | constraints.Integer] interface {
	AddElement(T)
	CalculateAverage() float64
	AllSequentialIncreasesLessThan(float64) (bool, float64)
	StandardDeviation() (bool, T)
	IsNormallyDistributed() bool
	Size() int
	Values() []T
	Percentile(int) T
}

func calculateAverage[T constraints.Integer | constraints.Float](elements []T) float64 {
	total := T(0)
	for i := 0; i < len(elements); i++ {
		total += elements[i]
	}
	return float64(total) / float64(len(elements))
}

func calculatePercentile[T constraints.Integer | constraints.Float](elements []T, p int) (result T) {
	result = T(0)
	if p < 0 || p > 100 {
		return
	}

	sort.Slice(elements, func(l int, r int) bool { return elements[l] < elements[r] })
	pindex := int64((float64(p) / float64(100)) * float64(len(elements)))
	result = elements[pindex]
	return
}
func NewInfiniteMathematicalSeries[T constraints.Float | constraints.Integer]() MathematicalSeries[T] {
	return &InfiniteMathematicalSeries[T]{}
}

type InfiniteMathematicalSeries[T constraints.Float | constraints.Integer] struct {
	elements []T
}

func (ims *InfiniteMathematicalSeries[T]) AddElement(element T) {
	ims.elements = append(ims.elements, element)
}

func (ims *InfiniteMathematicalSeries[T]) CalculateAverage() float64 {
	return calculateAverage(ims.elements)
}

func (ims *InfiniteMathematicalSeries[T]) AllSequentialIncreasesLessThan(limit float64) (bool, float64) {
	if len(ims.elements) < 2 {
		return false, 0.0
	}

	maximumSequentialIncrease := float64(0)
	for i := 1; i < len(ims.elements); i++ {
		current := ims.elements[i]
		previous := ims.elements[i-1]
		percentChange := utilities.SignedPercentDifference(current, previous)
		if percentChange > limit {
			return false, percentChange
		}
		if percentChange > float64(maximumSequentialIncrease) {
			maximumSequentialIncrease = percentChange
		}
	}
	return true, maximumSequentialIncrease
}

/*
 * N.B.: Overflow is possible -- use at your discretion!
 */
func (ims *InfiniteMathematicalSeries[T]) StandardDeviation() (bool, T) {

	// From https://www.mathsisfun.com/data/standard-deviation-calculator.html
	// Yes, for real!

	// Calculate the average of the numbers ...
	average := ims.CalculateAverage()

	// Calculate the square of each of the elements' differences from the mean.
	differences_squared := make([]float64, len(ims.elements))
	for index, value := range ims.elements {
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
	variance := sds / float64(len(ims.elements))

	// Finally, the standard deviation is the square root
	// of the variance.
	sd := T(math.Sqrt(variance))
	//sd := T(variance)

	return true, sd
}

func (ims *InfiniteMathematicalSeries[T]) IsNormallyDistributed() bool {
	return false
}

func (ims *InfiniteMathematicalSeries[T]) Size() int {
	return len(ims.elements)
}

func (ims *InfiniteMathematicalSeries[T]) Values() []T {
	return ims.elements
}

func (ims *InfiniteMathematicalSeries[T]) Percentile(p int) T {
	return calculatePercentile(ims.elements, p)
}

type CappedMathematicalSeries[T constraints.Float | constraints.Integer] struct {
	elements_count int
	elements       []T
	index          int
	divisor        *saturating.SaturatingInt
}

func NewCappedMathematicalSeries[T constraints.Float | constraints.Integer](instants_count int) MathematicalSeries[T] {
	return &CappedMathematicalSeries[T]{
		elements:       make([]T, instants_count),
		elements_count: instants_count,
		divisor:        saturating.NewSaturatingInt(instants_count),
		index:          0,
	}
}

func (ma *CappedMathematicalSeries[T]) AddElement(measurement T) {
	ma.elements[ma.index] = measurement
	ma.divisor.Add(1)
	// Invariant: ma.index always points to the oldest measurement
	ma.index = (ma.index + 1) % ma.elements_count
}

func (ma *CappedMathematicalSeries[T]) CalculateAverage() float64 {
	// If we do not yet have all the values, then we know that the values
	// exist between 0 and ma.divisor.Value(). If we do have all the values,
	// we know that they, too, exist between 0 and ma.divisor.Value().
	return calculateAverage(ma.elements[0:ma.divisor.Value()])
}

func (ma *CappedMathematicalSeries[T]) AllSequentialIncreasesLessThan(limit float64) (_ bool, maximumSequentialIncrease float64) {

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
func (ma *CappedMathematicalSeries[T]) StandardDeviation() (bool, T) {

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

	return true, sd
}

func (ma *CappedMathematicalSeries[T]) IsNormallyDistributed() bool {
	valid, stddev := ma.StandardDeviation()
	// If there are not enough values in our series to generate a standard
	// deviation, then we cannot do this calculation either.
	if !valid {
		return false
	}
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

func (ma *CappedMathematicalSeries[T]) Values() []T {
	return ma.elements
}

func (ma *CappedMathematicalSeries[T]) Size() int {
	return len(ma.elements)
}

func (ma *CappedMathematicalSeries[T]) Percentile(p int) T {
	if p < 0 || p > 100 {
		return 0
	}

	// Because we need to sort the list to perform the percentile calculation,
	// we have to make a copy of the list so that we don't disturb
	// the time-relative ordering of the elements.

	kopy := make([]T, len(ma.elements))
	copy(kopy, ma.elements)
	return calculatePercentile(kopy, p)
}
