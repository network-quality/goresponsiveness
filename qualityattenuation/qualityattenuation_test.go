package qualityattenuation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicSimpleQualityAttenuation(t *testing.T) {
	qa := NewSimpleQualityAttenuation()
	qa.AddSample(1.0)
	qa.AddSample(2.0)
	qa.AddSample(3.0)
	assert.Equal(t, qa.Number_of_samples, int64(3))
	assert.Equal(t, qa.Number_of_losses, int64(0))
	assert.InEpsilon(t, 1.0, qa.Minimum_latency, 0.000001)
	assert.InEpsilon(t, 3.0, qa.Maximum_latency, 0.000001)
	assert.InEpsilon(t, 5.7, qa.Offset_sum, 0.000001)
	assert.InEpsilon(t, 12.83, qa.Offset_sum_of_squares, 0.000001)
	assert.InEpsilon(t, 1.0, qa.EmpiricalDistribution.Quantile(0.1), 0.000001)
	assert.InEpsilon(t, 2.0, qa.EmpiricalDistribution.Quantile(0.5), 0.000001)
	assert.InEpsilon(t, 3.0, qa.EmpiricalDistribution.Quantile(0.9), 0.000001)
	//Test the get functions
	assert.Equal(t, qa.GetNumberOfSamples(), int64(3))
	assert.Equal(t, qa.GetNumberOfLosses(), int64(0))
	assert.InEpsilon(t, 1.0, qa.GetMinimum(), 0.000001)
	assert.InEpsilon(t, 3.0, qa.GetMaximum(), 0.000001)
	assert.InEpsilon(t, 2.0, qa.GetAverage(), 0.000001)
	assert.InEpsilon(t, 1.0, qa.GetVariance(), 0.000001)
	assert.InEpsilon(t, 1.0, qa.GetStandardDeviation(), 0.000001)
	assert.InEpsilon(t, 2.0, qa.GetMedian(), 0.000001)
	assert.InEpsilon(t, 1.0, qa.GetLossPercentage()+1.000000000, 0.000001)
	assert.InEpsilon(t, 30, qa.GetRPM(), 0.000001)
	assert.InEpsilon(t, 1.0, qa.GetPercentile(10.0), 0.000001)
	assert.InEpsilon(t, 2.0, qa.GetPercentile(50.0), 0.000001)
	assert.InEpsilon(t, 3.0, qa.GetPercentile(90.0), 0.000001)
	assert.InEpsilon(t, 2.0, qa.GetPDV(90), 0.000001)
}

func TestManySamples(t *testing.T) {
	qa := NewSimpleQualityAttenuation()
	for i := 1; i < 160000; i++ {
		qa.AddSample(float64(i) / 10000.0) //Linear ramp from 0.0001 to 16.0
	}
	assert.Equal(t, qa.Number_of_samples, int64(160000-1))
	assert.Equal(t, qa.Number_of_losses, int64(10000-1)) //Samples from 15.0001 to 16.0 are lost
	assert.InEpsilon(t, 0.0001, qa.Minimum_latency, 0.000001)
	assert.InEpsilon(t, 15.0000, qa.Maximum_latency, 0.000001)
	assert.InEpsilon(t, 1110007.5, qa.Offset_sum, 0.000001)
	assert.InEpsilon(t, 11026611.00024998, qa.Offset_sum_of_squares, 0.000001)
	assert.InEpsilon(t, 1.50005, qa.EmpiricalDistribution.Quantile(0.1), 0.000001)
	assert.InEpsilon(t, 7.500049, qa.EmpiricalDistribution.Quantile(0.5), 0.000001)
	assert.InEpsilon(t, 13.50005, qa.EmpiricalDistribution.Quantile(0.9), 0.000001)
	//Test the get functions
	assert.Equal(t, qa.GetNumberOfSamples(), int64(160000-1))
	assert.Equal(t, qa.GetNumberOfLosses(), int64(10000-1))
	assert.InEpsilon(t, 0.0001, qa.GetMinimum(), 0.000001)
	assert.InEpsilon(t, 15.0000, qa.GetMaximum(), 0.000001)
	assert.InEpsilon(t, 7.500049, qa.GetAverage(), 0.000001)
	assert.InEpsilon(t, 18.749750, qa.GetVariance(), 0.001)
	assert.InEpsilon(t, 4.330098, qa.GetStandardDeviation(), 0.001)
	assert.InEpsilon(t, 7.500049, qa.GetMedian(), 0.000001)
	assert.InEpsilon(t, 6.249414, qa.GetLossPercentage(), 0.000001)
	assert.InEpsilon(t, 7.999947, qa.GetRPM(), 0.000001)
}
