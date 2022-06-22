//go:build darwin
// +build darwin

package extendedstats

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/sys/unix"
)

type ExtendedStats struct {
	Maxseg               uint64
	MaxSendMss           uint64
	MaxRecvMss           uint64
	TotalRetransmissions uint64
	totalSent            uint64
	TotalReorderings     uint64
	AverageRtt           float64
	rtt_measurements     uint64
	total_rtt            float64
	RetransmitRatio      float64
}

func ExtendedStatsAvailable() bool {
	return true
}

func (es *ExtendedStats) IncorporateConnectionStats(rawConn net.Conn) error {
	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		return fmt.Errorf(
			"OOPS: Could not get the TCP info for the connection (not a TLS connection)",
		)
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		return fmt.Errorf(
			"OOPS: Could not get the TCP info for the connection (not a TCP connection)",
		)
	}
	if info, err := getTCPConnectionInfo(tcpConn); err != nil {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection: %v", err)
	} else {
		es.Maxseg = utilities.Max(es.Maxseg, uint64(info.Maxseg))
		es.TotalReorderings += info.Rxoutoforderbytes
		es.TotalRetransmissions += info.Txretransmitbytes
		es.totalSent += info.Txbytes
		es.total_rtt += float64(info.Srtt)
		es.rtt_measurements += 1
		es.AverageRtt = es.total_rtt / float64(es.rtt_measurements)
		es.RetransmitRatio = (float64(es.TotalRetransmissions) / float64(es.totalSent)) * 100.0
	}
	return nil
}

func (es *ExtendedStats) Repr() string {
	return fmt.Sprintf(`Extended Statistics:
	Maximum Segment Size: %v
	Total Bytes Retransmitted: %v
	Retransmission Ratio: %.2f%%
	Total Bytes Reordered: %v
	Average RTT: %v
`, es.Maxseg, es.TotalRetransmissions, es.RetransmitRatio, es.TotalReorderings, es.AverageRtt)
}

func getTCPConnectionInfo(connection net.Conn) (*unix.TCPConnectionInfo, error) {
	tcpConn, ok := connection.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("connection is not a net.TCPConn")
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var info *unix.TCPConnectionInfo = nil
	rawConn.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPConnectionInfo(
			int(fd),
			unix.IPPROTO_TCP,
			unix.TCP_CONNECTION_INFO,
		)
	})
	return info, err
}
