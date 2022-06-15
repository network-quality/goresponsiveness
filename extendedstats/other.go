//go:build !dragonfly && !freebsd && !linux && !netbsd && !openbsd
// +build !dragonfly,!freebsd,!linux,!netbsd,!openbsd

package extendedstats

import (
	"fmt"
	"net"
)

type ExtendedStats struct{}

func (es *ExtendedStats) IncorporateConnectionStats(conn net.Conn) error {
	return fmt.Errorf("OOPS: IncorporateConnectionStats is not supported on this platform.")
}

func (es *ExtendedStats) Repr() string {
	return ""
}

func ExtendedStatsAvailable() bool {
	return false
}
