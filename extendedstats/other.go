//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd
// +build !darwin,!dragonfly,!freebsd,!linux,!netbsd,!openbsd

package extendedstats

import (
	"net"

	"golang.org/x/sys/unix"
)

type ExtendedStats struct{}

func (es *ExtendedStats) IncorporateConnectionStats(conn net.Conn) {}

func (es *ExtendedStats) Repr() string {
	return ""
}

func ExtendedStatsAvailable() bool {
	return false
}
