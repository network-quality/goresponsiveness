// Implements a data structure for quality attenuation.

package qualityattenuation

import (
	"fmt"
	"math"

	"github.com/influxdata/tdigest"
)

type SimpleQualityAttenuation struct {
	empiricalDistribution  *tdigest.TDigest
	offset                 float64
	offsetSum              float64
	offsetSumOfSquares     float64
	numberOfSamples        int64
	numberOfLosses         int64
	latencyEqLossThreshold float64
	minimumLatency         float64
	maximumLatency         float64
}

func NewSimpleQualityAttenuation() *SimpleQualityAttenuation {
	return &SimpleQualityAttenuation{
		empiricalDistribution:  tdigest.NewWithCompression(50),
		offset:                 0.1,
		offsetSum:              0.0,
		offsetSumOfSquares:     0.0,
		numberOfSamples:        0,
		numberOfLosses:         0,
		latencyEqLossThreshold: 15.0, // Count latency greater than this value as a loss.
		minimumLatency:         0.0,
		maximumLatency:         0.0,
	}
}

func (qa *SimpleQualityAttenuation) AddSample(sample float64) error {
	if sample <= 0.0 {
		// Ignore zero or negative samples because they cannot be valid.
		// TODO: This should raise a warning and/or trigger error handling.
		return fmt.Errorf("sample is zero or negative")
	}
	qa.numberOfSamples++
	if sample > qa.latencyEqLossThreshold {
		qa.numberOfLosses++
		return nil
	} else {
		if qa.minimumLatency == 0.0 || sample < qa.minimumLatency {
			qa.minimumLatency = sample
		}
		if qa.maximumLatency == 0.0 || sample > qa.maximumLatency {
			qa.maximumLatency = sample
		}
		qa.empiricalDistribution.Add(sample, 1)
		qa.offsetSum += sample - qa.offset
		qa.offsetSumOfSquares += (sample - qa.offset) * (sample - qa.offset)
	}
	return nil
}

func (qa *SimpleQualityAttenuation) GetNumberOfLosses() int64 {
	return qa.numberOfLosses
}

func (qa *SimpleQualityAttenuation) GetNumberOfSamples() int64 {
	return qa.numberOfSamples
}

func (qa *SimpleQualityAttenuation) GetPercentile(percentile float64) float64 {
	return qa.empiricalDistribution.Quantile(percentile / 100)
}

func (qa *SimpleQualityAttenuation) GetAverage() float64 {
	return qa.offsetSum/float64(qa.numberOfSamples-qa.numberOfLosses) + qa.offset
}

func (qa *SimpleQualityAttenuation) GetVariance() float64 {
	number_of_latency_samples := float64(qa.numberOfSamples) - float64(qa.numberOfLosses)
	return (qa.offsetSumOfSquares - (qa.offsetSum * qa.offsetSum / number_of_latency_samples)) / (number_of_latency_samples - 1)
}

func (qa *SimpleQualityAttenuation) GetStandardDeviation() float64 {
	return math.Sqrt(qa.GetVariance())
}

func (qa *SimpleQualityAttenuation) GetMinimum() float64 {
	return qa.minimumLatency
}

func (qa *SimpleQualityAttenuation) GetMaximum() float64 {
	return qa.maximumLatency
}

func (qa *SimpleQualityAttenuation) GetMedian() float64 {
	return qa.GetPercentile(50.0)
}

func (qa *SimpleQualityAttenuation) GetLossPercentage() float64 {
	return 100 * float64(qa.numberOfLosses) / float64(qa.numberOfSamples)
}

func (qa *SimpleQualityAttenuation) GetRPM() float64 {
	return 60.0 / qa.GetAverage()
}

func (qa *SimpleQualityAttenuation) GetPDV(percentile float64) float64 {
	return qa.GetPercentile(percentile) - qa.GetMinimum()
}

// Merge two quality attenuation values. This operation assumes the two samples have the same offset and latency_eq_loss_threshold, and
// will return an error if they do not.
// It also assumes that the two quality attenuation values are measurements of the same thing (path, outcome, etc.).
func (qa *SimpleQualityAttenuation) Merge(other *SimpleQualityAttenuation) error {
	// Check that offsets are the same
	if qa.offset != other.offset ||
		qa.latencyEqLossThreshold != other.latencyEqLossThreshold {
		return fmt.Errorf("merge quality attenuation values with different offset or latency_eq_loss_threshold")
	}
	for _, centroid := range other.empiricalDistribution.Centroids() {
		mean := centroid.Mean
		weight := centroid.Weight
		qa.empiricalDistribution.Add(mean, weight)
	}
	qa.offsetSum += other.offsetSum
	qa.offsetSumOfSquares += other.offsetSumOfSquares
	qa.numberOfSamples += other.numberOfSamples
	qa.numberOfLosses += other.numberOfLosses
	if other.minimumLatency < qa.minimumLatency {
		qa.minimumLatency = other.minimumLatency
	}
	if other.maximumLatency > qa.maximumLatency {
		qa.maximumLatency = other.maximumLatency
	}
	return nil
}
