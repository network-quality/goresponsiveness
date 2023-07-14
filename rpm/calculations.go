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

package rpm

import (
	"fmt"

	"github.com/network-quality/goresponsiveness/series"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type Rpm[Data utilities.Number] struct {
	SelfRttsTotal       int
	ForeignRttsTotal    int
	SelfRttsTrimmed     int
	ForeignRttsTrimmed  int
	SelfProbeRttPN      Data
	ForeignProbeRttPN   Data
	SelfProbeRttMean    float64
	ForeignProbeRttMean float64
	PNRpm               float64
	MeanRpm             float64
}

func CalculateRpm[Data utilities.Number, Bucket constraints.Ordered](
	selfRtts series.WindowSeries[Data, Bucket], aggregatedForeignRtts series.WindowSeries[Data, Bucket], trimming uint, percentile uint,
) *Rpm[Data] {
	// There may be more than one round trip accumulated together. If that is the case,
	// we will blow them apart in to three separate measurements and each one will just
	// be 1 / 3.
	foreignRtts := series.NewWindowSeries[Data, int](series.Forever, 0)
	foreignBuckets := 0
	for _, v := range aggregatedForeignRtts.GetValues() {
		if utilities.IsSome(v) {
			v := utilities.GetSome(v)
			foreignRtts.Reserve(foreignBuckets)
			foreignRtts.Reserve(foreignBuckets + 1)
			foreignRtts.Reserve(foreignBuckets + 2)
			foreignRtts.Fill(foreignBuckets, v/3)
			foreignRtts.Fill(foreignBuckets+1, v/3)
			foreignRtts.Fill(foreignBuckets+2, v/3)
			foreignBuckets += 3
		}
	}

	// First, let's do a double-sided trim of the top/bottom 10% of our measurements.
	selfRttsTotalCount, _ := selfRtts.Count()
	foreignRttsTotalCount, _ := foreignRtts.Count()

	_, selfProbeRoundTripTimeMean, selfRttsTrimmed :=
		series.TrimmedMean(selfRtts, int(trimming))
	_, foreignProbeRoundTripTimeMean, foreignRttsTrimmed :=
		series.TrimmedMean(foreignRtts, int(trimming))

	selfRttsTrimmedCount := len(selfRttsTrimmed)
	foreignRttsTrimmedCount := len(foreignRttsTrimmed)

	// Second, let's do the P90 calculations.
	_, selfProbeRoundTripTimePN := series.Percentile(selfRtts, percentile)
	_, foreignProbeRoundTripTimePN := series.Percentile(foreignRtts, percentile)

	// Note: The specification indicates that we want to calculate the foreign probes as such:
	// 1/3*tcp_foreign + 1/3*tls_foreign + 1/3*http_foreign
	// where tcp_foreign, tls_foreign, http_foreign are the P90 RTTs for the connection
	// of the tcp, tls and http connections, respectively. However, we cannot break out
	// the individual RTTs so we assume that they are roughly equal.

	// This is 60 because we measure in seconds not ms
	pnRpm := 60.0 / (float64(selfProbeRoundTripTimePN+foreignProbeRoundTripTimePN) / 2.0)
	meanRpm := 60.0 / (float64(selfProbeRoundTripTimeMean+foreignProbeRoundTripTimeMean) / 2.0)

	return &Rpm[Data]{
		SelfRttsTotal: selfRttsTotalCount, ForeignRttsTotal: foreignRttsTotalCount,
		SelfRttsTrimmed: selfRttsTrimmedCount, ForeignRttsTrimmed: foreignRttsTrimmedCount,
		SelfProbeRttPN: selfProbeRoundTripTimePN, ForeignProbeRttPN: foreignProbeRoundTripTimePN,
		SelfProbeRttMean: selfProbeRoundTripTimeMean, ForeignProbeRttMean: foreignProbeRoundTripTimeMean,
		PNRpm: pnRpm, MeanRpm: meanRpm,
	}
}

func (rpm *Rpm[Data]) ToString() string {
	return fmt.Sprintf(
		`Total Self Probes:            %d
Total Foreign Probes:         %d
Trimmed Self Probes Count:    %d
Trimmed Foreign Probes Count: %d
P90 Self RTT:                 %v
P90 Foreign RTT:              %v
Trimmed Mean Self RTT:        %f
Trimmed Mean Foreign RTT:     %f
`,
		rpm.SelfRttsTotal,
		rpm.ForeignRttsTotal,
		rpm.SelfRttsTrimmed,
		rpm.ForeignRttsTrimmed,
		rpm.SelfProbeRttPN,
		rpm.ForeignProbeRttPN,
		rpm.SelfProbeRttMean,
		rpm.ForeignProbeRttMean,
	)
}
