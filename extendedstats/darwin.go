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

type AggregateExtendedStats struct {
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

type TCPInfo struct {
	Rxoutoforderbytes uint64
	Txretransmitbytes uint64
	Txbytes           uint64
	Rtt               uint32
	Maxseg            uint32
	Snd_cwnd          uint32
}

func (es *AggregateExtendedStats) IncorporateConnectionStats(basicConn net.Conn) error {
	if info, err := GetTCPInfo(basicConn); err != nil {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection: %v", err)
	} else {
		es.Maxseg = utilities.Max(es.Maxseg, uint64(info.Maxseg))
		es.TotalReorderings += info.Rxoutoforderbytes
		es.TotalRetransmissions += info.Txretransmitbytes
		es.totalSent += info.Txbytes
		es.total_rtt += float64(info.Rtt)
		es.rtt_measurements += 1
		es.AverageRtt = es.total_rtt / float64(es.rtt_measurements)
		es.RetransmitRatio = (float64(es.TotalRetransmissions) / float64(es.totalSent)) * 100.0
	}
	return nil
}

func (es *AggregateExtendedStats) Repr() string {
	return fmt.Sprintf(`Extended Statistics:
	Maximum Segment Size: %v
	Total Bytes Retransmitted: %v
	Retransmission Ratio: %.2f%%
	Total Bytes Reordered: %v
	Average RTT: %v
`, es.Maxseg, es.TotalRetransmissions, es.RetransmitRatio, es.TotalReorderings, es.AverageRtt)
}

func GetTCPInfo(basicConn net.Conn) (*TCPInfo, error) {
	tlsConn, ok := basicConn.(*tls.Conn)
	if !ok {
		return nil, fmt.Errorf(
			"OOPS: Could not get the TCP info for the connection (not a TLS connection)",
		)
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

	var rawInfo *unix.TCPConnectionInfo = nil
	var tcpInfo *TCPInfo = nil
	rawConn.Control(func(fd uintptr) {
		rawInfo, err = unix.GetsockoptTCPConnectionInfo(
			int(fd),
			unix.IPPROTO_TCP,
			unix.TCP_CONNECTION_INFO,
		)
	})
	if rawInfo != nil && err == nil {
		tcpInfo = &TCPInfo{}
		tcpInfo.Rxoutoforderbytes = rawInfo.Rxoutoforderbytes
		tcpInfo.Txretransmitbytes = rawInfo.Txretransmitbytes
		tcpInfo.Rtt = rawInfo.Srtt
		tcpInfo.Snd_cwnd = rawInfo.Snd_cwnd
		tcpInfo.Maxseg = rawInfo.Maxseg
	}
	return tcpInfo, err
}
