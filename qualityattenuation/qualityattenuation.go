// Implements a data structure for quality attenuation.

package qualityattenuation

import (
	"math"

	"github.com/influxdata/tdigest"
)

type SimpleQualityAttenuation struct {
	EmpiricalDistribution     *tdigest.TDigest
	Offset                    float64
	Offset_sum                float64
	Offset_sum_of_squares     float64
	Number_of_samples         int64
	Number_of_losses          int64
	Latency_eq_loss_threshold float64
	Minimum_latency           float64
	Maximum_latency           float64
}

func NewSimpleQualityAttenuation() *SimpleQualityAttenuation {
	return &SimpleQualityAttenuation{
		EmpiricalDistribution:     tdigest.NewWithCompression(50),
		Offset:                    0.1,
		Offset_sum:                0.0,
		Offset_sum_of_squares:     0.0,
		Number_of_samples:         0,
		Number_of_losses:          0,
		Latency_eq_loss_threshold: 15.0, // Count latency greater than this value as a loss.
		Minimum_latency:           0.0,
		Maximum_latency:           0.0,
	}
}

func (qa *SimpleQualityAttenuation) AddSample(sample float64) {
	if sample <= 0.0 {
		// Ignore zero or negative samples because they cannot be valid.
		// TODO: This should raise a warning and/or trigger error handling.
		return
	}
	qa.Number_of_samples++
	if sample > qa.Latency_eq_loss_threshold {
		qa.Number_of_losses++
		return
	} else {
		if qa.Minimum_latency == 0.0 || sample < qa.Minimum_latency {
			qa.Minimum_latency = sample
		}
		if qa.Maximum_latency == 0.0 || sample > qa.Maximum_latency {
			qa.Maximum_latency = sample
		}
		qa.EmpiricalDistribution.Add(sample, 1)
		qa.Offset_sum += sample - qa.Offset
		qa.Offset_sum_of_squares += (sample - qa.Offset) * (sample - qa.Offset)
	}
}

func (qa *SimpleQualityAttenuation) GetNumberOfLosses() int64 {
	return qa.Number_of_losses
}

func (qa *SimpleQualityAttenuation) GetNumberOfSamples() int64 {
	return qa.Number_of_samples
}

func (qa *SimpleQualityAttenuation) GetPercentile(percentile float64) float64 {
	return qa.EmpiricalDistribution.Quantile(percentile / 100)
}

func (qa *SimpleQualityAttenuation) GetAverage() float64 {
	return qa.Offset_sum/float64(qa.Number_of_samples-qa.Number_of_losses) + qa.Offset
}

func (qa *SimpleQualityAttenuation) GetVariance() float64 {
	number_of_latency_samples := float64(qa.Number_of_samples) - float64(qa.Number_of_losses)
	return (qa.Offset_sum_of_squares - (qa.Offset_sum * qa.Offset_sum / number_of_latency_samples)) / (number_of_latency_samples)
}

func (qa *SimpleQualityAttenuation) GetStandardDeviation() float64 {
	return math.Sqrt(qa.GetVariance())
}

func (qa *SimpleQualityAttenuation) GetMinimum() float64 {
	return qa.Minimum_latency
}

func (qa *SimpleQualityAttenuation) GetMaximum() float64 {
	return qa.Maximum_latency
}

func (qa *SimpleQualityAttenuation) GetMedian() float64 {
	return qa.GetPercentile(50.0)
}

func (qa *SimpleQualityAttenuation) GetLossPercentage() float64 {
	return 100 * float64(qa.Number_of_losses) / float64(qa.Number_of_samples)
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
func (qa *SimpleQualityAttenuation) Merge(other *SimpleQualityAttenuation) {
	// Check that offsets are the same
	if qa.Offset != other.Offset ||
		qa.Latency_eq_loss_threshold != other.Latency_eq_loss_threshold {
		//"Cannot merge quality attenuation values with different offset or latency_eq_loss_threshold"

	}
	for _, centroid := range other.EmpiricalDistribution.Centroids() {
		mean := centroid.Mean
		weight := centroid.Weight
		qa.EmpiricalDistribution.Add(mean, weight)
	}
	qa.Offset_sum += other.Offset_sum
	qa.Offset_sum_of_squares += other.Offset_sum_of_squares
	qa.Number_of_samples += other.Number_of_samples
	qa.Number_of_losses += other.Number_of_losses
	if other.Minimum_latency < qa.Minimum_latency {
		qa.Minimum_latency = other.Minimum_latency
	}
	if other.Maximum_latency > qa.Maximum_latency {
		qa.Maximum_latency = other.Maximum_latency
	}
}
