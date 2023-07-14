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
	"time"

	"github.com/network-quality/goresponsiveness/utilities"
)

type SpecParameters struct {
	TestTimeout      time.Duration // Total test time.
	MovingAvgDist    int
	EvalInterval     time.Duration // How often to reevaluate network conditions.
	TrimmedMeanPct   uint
	StdDevTolerance  float64
	MaxParallelConns int
	ProbeInterval    time.Duration
	ProbeCapacityPct float64
	Percentile       uint
}

func SpecParametersFromArguments(timeout int, mad int, id int, tmp uint, sdt float64, mnp int, mps int, ptc float64, p int) (*SpecParameters, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative timeout for the test")
	}
	if mad <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative moving-average distance for the test")
	}
	if id <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative reevaluation interval for the test")
	}
	if tmp < 0 {
		return nil, fmt.Errorf("cannot specify a negative trimming percentage for the test")
	}
	if sdt < 0 {
		return nil, fmt.Errorf("cannot specify a negative standard-deviation tolerance for the test")
	}
	if mnp <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative maximum number of parallel connections for the test")
	}
	if mps <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative probing interval for the test")
	}
	if ptc <= 0 {
		return nil, fmt.Errorf("cannot specify a 0 or negative probe capacity for the test")
	}
	if p < 1 || p >= 100 {
		return nil, fmt.Errorf("percentile for statistical calculations (%v) is invalid", p)
	}
	testTimeout := time.Second * time.Duration(timeout)
	evalInterval := time.Second * time.Duration(id)
	probeInterval := utilities.PerSecondToInterval(int64(mps))

	params := SpecParameters{
		TestTimeout: testTimeout, MovingAvgDist: mad,
		EvalInterval: evalInterval, TrimmedMeanPct: tmp, StdDevTolerance: sdt,
		MaxParallelConns: mnp, ProbeInterval: probeInterval, ProbeCapacityPct: ptc, Percentile: uint(p),
	}
	return &params, nil
}

func (parameters *SpecParameters) ToString() string {
	return fmt.Sprintf(
		`Timeout:                                     %v,
Moving-Average Distance:                     %v,
Interval Duration:                           %v,
Trimmed-Mean Percentage:                     %v,
Standard-Deviation Tolerance:                %v,
Maximum number of parallel connections:      %v,
Probe Interval:                              %v (derived from given maximum-probes-per-second parameter),
Maximum Percentage Of Throughput For Probes: %v`,
		parameters.TestTimeout, parameters.MovingAvgDist, parameters.EvalInterval, parameters.TrimmedMeanPct,
		parameters.StdDevTolerance, parameters.MaxParallelConns, parameters.ProbeInterval, parameters.ProbeCapacityPct,
	)
}
