//go:build !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !darwin && !windows
// +build !dragonfly,!freebsd,!linux,!netbsd,!openbsd,!darwin,!windows

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

package extendedstats

import (
	"fmt"
	"net"
)

type ExtendedStats struct{}

func (es *ExtendedStats) IncorporateConnectionStats(conn net.Conn) error {
	return fmt.Errorf("IncorporateConnectionStats is not supported on this platform")
}

func (es *ExtendedStats) Repr() string {
	return ""
}

func ExtendedStatsAvailable() bool {
	return false
}

func GetTCPInfo(basicConn net.Conn) (interface, error) {
	return nil, fmt.Errorf("GetTCPInfo is not supported on this platform")
}
