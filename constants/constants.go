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
	// The initial number of load-generating connections when attempting to saturate the network.
	StartingNumberOfLoadGeneratingConnections uint64 = 1
	// The number of load-generating connections to add at each interval while attempting to
	// saturate the network.
	AdditiveNumberOfLoadGeneratingConnections uint64 = 1

	// The amount of time that the client will cooldown if it is in debug mode.
	CooldownPeriod time.Duration = 4 * time.Second

	// The default amount of time that a test will take to calculate the RPM.
	DefaultTestTime int = 20
	// The default port number to which to connect on the config host.
	DefaultPortNumber int = 4043
	// The default determination of whether to run in debug mode.
	DefaultDebug bool = false
	// The default URL for the config host.
	DefaultConfigHost string = "networkquality.example.com"
	// The default determination of whether to verify server certificates
	DefaultInsecureSkipVerify bool = true
)

type SpecParametersCliOptions struct {
	Mad int
	Id  int
	Tmp uint
	Sdt float64
	Mnp int
	Mps int
	Ptc float64
	P   int
}

var SpecParameterCliOptionsDefaults = SpecParametersCliOptions{Mad: 4, Id: 1, Tmp: 5, Sdt: 5.0, Mnp: 16, Mps: 100, Ptc: 0.05, P: 90}
