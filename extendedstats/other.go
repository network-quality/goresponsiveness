//go:build !dragonfly && !freebsd && !linux && !netbsd && !openbsd
// +build !dragonfly,!freebsd,!linux,!netbsd,!openbsd

package extendedstats

import (
	"net"
)

type ExtendedStats struct{}

func (es *ExtendedStats) IncorporateConnectionStats(conn net.Conn) {}

func (es *ExtendedStats) Repr() string {
	return ""
}

func ExtendedStatsAvailable() bool {
	return false
}
