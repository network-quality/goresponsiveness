//go:build dragonfly || freebsd || linux || netbsd || openbsd
// +build  dragonfly freebsd linux netbsd openbsd

package extendedstats

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/sys/unix"
)

type ExtendedStats struct {
	MaxPathMtu           uint64
	TotalRetransmissions uint64
	AverageRtt           float64
	rtt_measurements     uint64
	total_rtt            float64
}

func ExtendedStatsAvailable() bool {
	return true
}

func (es *ExtendedStats) IncorporateConnectionStats(rawConn net.Conn) {
	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		fmt.Printf("OOPS: Could not get the TCP info for the connection (not a TLS connection)!\n")
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		fmt.Printf("OOPS: Could not get the TCP info for the connection (not a TCP connection)!\n")
	}
	if info, err := getTCPInfo(tcpConn); err != nil {
		fmt.Printf("OOPS: Could not get the TCP info for the connection: %v!\n", err)
	} else {
		es.MaxPathMtu = utilities.Max(es.MaxPathMtu, uint64(info.Pmtu))
		// https://lkml.iu.edu/hypermail/linux/kernel/1705.0/01790.html
		es.TotalRetransmissions += uint64(info.Total_retrans)
		es.total_rtt += float64(info.Rtt)
		es.rtt_measurements += 1
		es.AverageRtt = es.total_rtt / float64(es.rtt_measurements)

	}
}

func (es *ExtendedStats) Repr() string {
	return fmt.Sprintf(`Extended Statistics:
	Maximum Path MTU: %v
	Total Retransmissions: %v
	Average RTT: %v
`, es.MaxPathMtu, es.TotalRetransmissions, es.AverageRtt)
}

func getTCPInfo(connection net.Conn) (*unix.TCPInfo, error) {
	tcpConn, ok := connection.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("connection is not a net.TCPConn")
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
