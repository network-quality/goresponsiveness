//go:build dragonfly || freebsd || linux || netbsd || openbsd
// +build dragonfly freebsd linux netbsd openbsd

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
	"crypto/tls"
	"fmt"
	"net"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/sys/unix"
)

type AggregateExtendedStats struct {
	MaxPathMtu           uint64
	MaxSendMss           uint64
	MaxRecvMss           uint64
	TotalRetransmissions uint64
	TotalReorderings     uint64
	AverageRtt           float64
	rtt_measurements     uint64
	total_rtt            float64
}

func ExtendedStatsAvailable() bool {
	return true
}

func (es *AggregateExtendedStats) IncorporateConnectionStats(basicConn net.Conn) error {
	if info, err := GetTCPInfo(basicConn); err != nil {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection: %v", err)
	} else {
		es.MaxPathMtu = utilities.Max(es.MaxPathMtu, uint64(info.Pmtu))
		es.MaxRecvMss = utilities.Max(es.MaxRecvMss, uint64(info.Rcv_mss))
		es.MaxSendMss = utilities.Max(es.MaxSendMss, uint64(info.Snd_mss))
		// https://lkml.iu.edu/hypermail/linux/kernel/1705.0/01790.html
		es.TotalRetransmissions += uint64(info.Total_retrans)
		es.TotalReorderings += uint64(info.Reordering)
		es.total_rtt += float64(info.Rtt)
		es.rtt_measurements += 1
		es.AverageRtt = es.total_rtt / float64(es.rtt_measurements)
	}
	return nil
}

func (es *AggregateExtendedStats) Repr() string {
	return fmt.Sprintf(`Extended Statistics:
	Maximum Path MTU: %v
	Maximum Send MSS: %v
	Maximum Recv MSS: %v
	Total Retransmissions: %v
	Total Reorderings: %v
	Average RTT: %v
`, es.MaxPathMtu, es.MaxSendMss, es.MaxRecvMss, es.TotalRetransmissions, es.TotalReorderings, es.AverageRtt)
}

func GetTCPInfo(basicConn net.Conn) (*unix.TCPInfo, error) {
	tlsConn, ok := basicConn.(*tls.Conn)
	if !ok {
		return nil, fmt.Errorf("OOPS: Outermost connection is not a TLS connection")
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf(
			"OOPS: Could not get the TCP info for the connection (not a TCP connection)",
		)
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var info *unix.TCPInfo = nil
	rawConn.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPInfo(int(fd), unix.SOL_TCP, unix.TCP_INFO)
	})
	return info, err
}
