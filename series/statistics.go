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

package series

import (
	"github.com/network-quality/goresponsiveness/utilities"
)

func SeriesStandardDeviation[Data utilities.Number, Bucket utilities.Number](s WindowSeries[Data, Bucket]) (bool, float64) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculateStandardDeviation[Data](values)
}

func Percentile[Data utilities.Number, Bucket utilities.Number](s WindowSeries[Data, Bucket], p uint) (bool, Data) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculatePercentile(values, p)
}

func AllSequentialIncreasesLessThan[Data utilities.Number, Bucket utilities.Number](s WindowSeries[Data, Bucket], limit float64,
) (bool, bool, float64) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(utilities.Reverse(inputValues), func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	result, actualLimit := utilities.AllSequentialIncreasesLessThan(values, limit)
	return complete, result, actualLimit
}

func CalculateAverage[Data utilities.Number, Bucket utilities.Number](s WindowSeries[Data, Bucket]) (bool, float64) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculateAverage(values)
}

func TrimmedMean[Data utilities.Number, Bucket utilities.Number](s WindowSeries[Data, Bucket], trim int) (bool, float64, []Data) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	trimmedMean, trimmedElements := utilities.TrimmedMean(values, trim)
	return complete, trimmedMean, trimmedElements
}
