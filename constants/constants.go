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

package constants

import "time"

var (
	// The initial number of connections on a LBC.
	StartingNumberOfLoadGeneratingConnections uint64 = 4
	// The number of intervals for which to account in a moving-average
	// calculation.
	MovingAverageIntervalCount int = 4
	// The number of intervals across which to consider a moving average stable.
	MovingAverageStabilitySpan uint64 = 4
	// The number of connections to add to a LBC when unsaturated.
	AdditiveNumberOfLoadGeneratingConnections uint64 = 4
	// The cutoff of the percent difference that defines instability.
	InstabilityDelta float64 = 5

	// The amount of time that the client will cooldown if it is in debug mode.
	CooldownPeriod time.Duration = 4 * time.Second
	// The number of probes to send when calculating RTT.
	MeasurementProbeCount int = 5
	// The amount of time that we give ourselves to calculate the RPM.
	RPMCalculationTime int = 10

	// The default amount of time that a test will take to calculate the RPM.
	DefaultTestTime int = 20
	// The default port number to which to connect on the config host.
	DefaultPortNumber int = 4043
	// The default determination of whether to run in debug mode.
	DefaultDebug bool = false
	// The default URL for the config host.
	DefaultConfigHost string = "networkquality.example.com"
)
