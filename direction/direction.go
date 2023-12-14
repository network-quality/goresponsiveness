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

package direction

import (
	"context"

	"github.com/network-quality/goresponsiveness/datalogger"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/probe"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/series"
)

type Direction struct {
	DirectionLabel                    string
	SelfProbeDataLogger               datalogger.DataLogger[probe.ProbeDataPoint]
	ForeignProbeDataLogger            datalogger.DataLogger[probe.ProbeDataPoint]
	ThroughputDataLogger              datalogger.DataLogger[rpm.ThroughputDataPoint]
	GranularThroughputDataLogger      datalogger.DataLogger[rpm.GranularThroughputDataPoint]
	CreateLgdc                        func() lgc.LoadGeneratingConnection
	Lgcc                              lgc.LoadGeneratingConnectionCollection
	DirectionDebugging                *debug.DebugWithPrefix
	ProbeDebugging                    *debug.DebugWithPrefix
	ThroughputStabilizerDebugging     *debug.DebugWithPrefix
	ResponsivenessStabilizerDebugging *debug.DebugWithPrefix
	ExtendedStatsEligible             bool
	StableThroughput                  bool
	StableResponsiveness              bool
	SelfRtts                          series.WindowSeries[float64, uint64]
	ForeignRtts                       series.WindowSeries[float64, uint64]
	ThroughputActivityCtx             *context.Context
	ThroughputActivityCtxCancel       *context.CancelFunc
	FormattedResults                  string
}
