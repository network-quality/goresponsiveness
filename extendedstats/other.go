//go:build !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !darwin && !windows
// +build !dragonfly,!freebsd,!linux,!netbsd,!openbsd,!darwin,!windows

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
