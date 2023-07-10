// Implements a data structure for quality attenuation.

package qualityattenuation

import (
	"fmt"
	"math"

	"github.com/influxdata/tdigest"
)

type cablelabsHist struct {
	hist [256]float64
}

func (h *cablelabsHist) GetHist() [256]float64 {
	return h.hist
}

func (h *cablelabsHist) AddSample(sample float64) error {
	bin := 0
	if sample < 0.050 {
		// Round down
		bin = int(sample / 0.0005)
		h.hist[bin]++
	} else if sample < 0.150 {
		bin = int((sample - 0.050) / 0.001)
		h.hist[100+bin]++
	} else if sample < 1.150 {
		bin = int((sample - 0.150) / 0.020)
		h.hist[200+bin]++
	} else if sample < 1.400 {
		bin = 250
		h.hist[bin]++
	} else if sample < 3.000 {
		bin = int((sample - 1.400) / 0.400)
		h.hist[251+bin]++
	} else {
		bin = 255
		h.hist[bin]++
	}
	return nil
}

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
	hist                   cablelabsHist
}

type PercentileLatencyPair struct {
	percentile     float64
	perfectLatency float64
	uselessLatency float64
}

type QualityRequirement struct {
	latencyRequirements []PercentileLatencyPair
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
	qa.hist.AddSample(sample)
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

func (qa *SimpleQualityAttenuation) PrintCablelabsStatisticsSummary() string {
	// Prints a digest based on Cablelabs Latency Measurements Metrics and Architeture, CL-TR-LM-Arch-V01-221123, https://www.cablelabs.com/specifications/CL-TR-LM-Arch
	// The recommendation is to report the following percentiles: 0, 10, 25, 50, 75, 90, 95, 99, 99.9 and 100
	return fmt.Sprintf("Cablelabs Statistics Summary:\n"+
		"0th Percentile: %f\n"+
		"10th Percentile: %f\n"+
		"25th Percentile: %f\n"+
		"50th Percentile: %f\n"+
		"75th Percentile: %f\n"+
		"90th Percentile: %f\n"+
		"95th Percentile: %f\n"+
		"99th Percentile: %f\n"+
		"99.9th Percentile: %f\n"+
		"100th Percentile: %f\n",
		qa.GetPercentile(0.0),
		qa.GetPercentile(10.0),
		qa.GetPercentile(25.0),
		qa.GetPercentile(50.0),
		qa.GetPercentile(75.0),
		qa.GetPercentile(90.0),
		qa.GetPercentile(95.0),
		qa.GetPercentile(99.0),
		qa.GetPercentile(99.9),
		qa.GetPercentile(100.0))
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

func (qa *SimpleQualityAttenuation) GetHist() [256]float64 {
	return qa.hist.GetHist()
}

func (qa *SimpleQualityAttenuation) EmpiricalDistributionHistogram() []float64 {
	// Convert the tdigest to a histogram on the format defined by CableLabs, with the following bucket edges:
	// 100 bins from 0 to 50 ms, each 0.5 ms wide
	// 100 bins from 50 to 100 ms, each 1 ms wide
	// 50 bins from 150 to 1150 ms, each 20 ms wide
	// 1 bin from 1150 to 1400 ms, 250 ms wide
	// 4 bins from 1400 to 3000 ms, each 400 ms wide
	hist := make([]float64, 256)
	for i := 0; i < 100; i++ {
		hist[i] = float64(qa.numberOfSamples) * (qa.empiricalDistribution.CDF(float64(i+1)*0.0005) - qa.empiricalDistribution.CDF(float64(i)*0.0005))
	}
	for i := 100; i < 200; i++ {
		hist[i] = float64(qa.numberOfSamples) * (qa.empiricalDistribution.CDF(0.050+float64(i-99)*0.001) - qa.empiricalDistribution.CDF(0.050+float64(i-100)*0.001))
	}
	for i := 200; i < 250; i++ {
		hist[i] = float64(qa.numberOfSamples) * (qa.empiricalDistribution.CDF(0.150+float64(i-199)*0.020) - qa.empiricalDistribution.CDF(0.150+float64(i-200)*0.020))
	}
	for i := 250; i < 251; i++ {
		hist[i] = float64(qa.numberOfSamples) * (qa.empiricalDistribution.CDF(1.150+0.250) - qa.empiricalDistribution.CDF(1.150))
	}
	for i := 251; i < 255; i++ {
		hist[i] = float64(qa.numberOfSamples) * (qa.empiricalDistribution.CDF(1.400+float64(i-250)*0.400) - qa.empiricalDistribution.CDF(1.400+float64(i-251)*0.400))
	}
	hist[255] = float64(qa.numberOfSamples) * (1 - qa.empiricalDistribution.CDF(3.000))
	return hist
}

// Compute the Quality of Outcome (QoO) for a given quality requirement.
// The details and motivation for the QoO metric are described in the following internet draft:
// https://datatracker.ietf.org/doc/draft-olden-ippm-qoo/
func (qa *SimpleQualityAttenuation) QoO(requirement QualityRequirement) float64 {
	QoO := 100.0
	for _, percentileLatencyPair := range requirement.latencyRequirements {
		score := 0.0
		percentile := percentileLatencyPair.percentile
		perfectLatency := percentileLatencyPair.perfectLatency
		uselessLatency := percentileLatencyPair.uselessLatency
		latency := qa.GetPercentile(percentile)
		if latency >= uselessLatency {
			score = 0.0
		} else if latency <= perfectLatency {
			score = 100.0
		} else {
			score = 100 * ((uselessLatency - latency) / (uselessLatency - perfectLatency))
		}
		if score < QoO {
			QoO = score
		}
	}
	return QoO
}

func (qa *SimpleQualityAttenuation) GetGamingQoO() float64 {
	qualReq := QualityRequirement{}
	qualReq.latencyRequirements = []PercentileLatencyPair{}
	qualReq.latencyRequirements = append(qualReq.latencyRequirements, PercentileLatencyPair{percentile: 50.0, perfectLatency: 0.030, uselessLatency: 0.150})
	qualReq.latencyRequirements = append(qualReq.latencyRequirements, PercentileLatencyPair{percentile: 95.0, perfectLatency: 0.065, uselessLatency: 0.200})
	qualReq.latencyRequirements = append(qualReq.latencyRequirements, PercentileLatencyPair{percentile: 99.0, perfectLatency: 0.100, uselessLatency: 0.250})
	return qa.QoO(qualReq)
}
