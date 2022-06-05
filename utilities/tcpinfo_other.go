//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd
// +build !darwin,!dragonfly,!freebsd,!linux,!netbsd,!openbsd

package utilities

import (
	"net"

	"golang.org/x/sys/unix"
)

func GetTCPInfo(connection net.Conn) (*unix.TCPInfo, error) {
	return nil, NotImplemented{Functionality: "GetTCPInfo"}
}

func PrintTCPInfo(info *unix.TCPInfo) {
	return
}
