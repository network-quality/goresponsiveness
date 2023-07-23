//go:build linux
// +build linux

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

package l4s

import (
	"crypto/tls"
	"fmt"
	"net"
	"syscall"
)

func SetL4S(conn net.Conn, algorithm *string) error {
	if algorithm == nil {
		panic("Attempting to set L4S congestion control without specifying a congestion control algorithm.")
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return fmt.Errorf("when setting L4S congestion control algorithm, outermost connection is not a TLS connection")
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		return fmt.Errorf("when setting L4S congestion control algorithm, could not get the underlying IP connection")
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("when setting L4S congestion control algorithm, could not get the underlying raw connection")
	}
	var sockoptError error = nil
	rawConn.Control(func(fd uintptr) {
		sockoptError = syscall.SetsockoptString(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CONGESTION, *algorithm)
	})
	if sockoptError != nil {
		return fmt.Errorf("when setting L4S congestion control algorithm, could not set the algorithm to %v: %v", *algorithm, sockoptError.Error())
	}
	return nil
}
