package series

import (
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

func SeriesStandardDeviation[Data utilities.Number, Bucket constraints.Ordered](s WindowSeries[Data, Bucket]) (bool, float64) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculateStandardDeviation[Data](values)
}

func Percentile[Data utilities.Number, Bucket constraints.Ordered](s WindowSeries[Data, Bucket], p int) (bool, Data) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculatePercentile(values, p)
}

func AllSequentialIncreasesLessThan[Data utilities.Number, Bucket constraints.Ordered](s WindowSeries[Data, Bucket], limit float64,
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

func CalculateAverage[Data utilities.Number, Bucket constraints.Ordered](s WindowSeries[Data, Bucket]) (bool, float64) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	return complete, utilities.CalculateAverage(values)
}

func TrimmedMean[Data utilities.Number, Bucket constraints.Ordered](s WindowSeries[Data, Bucket], trim int) (bool, float64, []Data) {
	complete := s.Complete()

	inputValues := s.GetValues()

	actualValues := utilities.Filter(inputValues, func(d utilities.Optional[Data]) bool {
		return utilities.IsSome[Data](d)
	})
	values := utilities.Fmap(actualValues, func(d utilities.Optional[Data]) Data { return utilities.GetSome[Data](d) })

	trimmedMean, trimmedElements := utilities.TrimmedMean(values, trim)
	return complete, trimmedMean, trimmedElements
}
