//go:build windows
// +build windows

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
	"unsafe"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/sys/windows"
)

type AggregateExtendedStats struct {
	MaxMss                  uint64
	TotalBytesSent          uint64
	TotalBytesReceived      uint64
	TotalBytesReordered     uint64
	TotalBytesRetransmitted uint64

	RetransmitRatio  float64
	AverageRtt       float64
	rtt_measurements uint64
	total_rtt        float64
}

type TCPINFO_BASE struct {
	State             uint32
	Mss               uint32
	ConnectionTimeMs  uint64
	TimestampsEnabled bool
	RttUs             uint32
	MinRttUs          uint32
	BytesInFlight     uint32
	Cwnd              uint32
	SndWnd            uint32
	RcvWnd            uint32
	RcvBuf            uint32
	BytesOut          uint64
	BytesIn           uint64
	BytesReordered    uint32
	BytesRetrans      uint32
	FastRetrans       uint32
	DupAcksIn         uint32
	TimeoutEpisodes   uint32
	SynRetrans        byte // UCHAR
}

// https://github.com/tpn/winsdk-10/blob/9b69fd26ac0c7d0b83d378dba01080e93349c2ed/Include/10.0.16299.0/shared/mstcpip.h#L289
type TCPINFO_V0 struct {
	TCPINFO_BASE
}

// https://docs.microsoft.com/en-us/windows/win32/api/mstcpip/ns-mstcpip-tcp_info_v1
type TCPINFO_V1 struct {
	TCPINFO_BASE
	SndLimTransRwin uint32
	SndLimTimeRwin  uint32
	SndLimBytesRwin uint64
	SndLimTransCwnd uint32
	SndLimTimeCwnd  uint32
	SndLimBytesCwnd uint64
	SndLimTransSnd  uint32
	SndLimTimeSnd   uint32
	SndLimBytesSnd  uint64
}

// https://pkg.go.dev/golang.org/x/sys/unix#TCPInfo
// Used to allow access to TCPInfo in like manner to unix.
type TCPInfo struct {
	State          uint8
	Ca_state       uint8
	Retransmits    uint8
	Probes         uint8
	Backoff        uint8
	Options        uint8
	Rto            uint32
	Ato            uint32
	Snd_mss        uint32
	Rcv_mss        uint32
	Unacked        uint32
	Sacked         uint32
	Lost           uint32
	Retrans        uint32
	Fackets        uint32
	Last_data_sent uint32
	Last_ack_sent  uint32
	Last_data_recv uint32
	Last_ack_recv  uint32
	Pmtu           uint32
	Rcv_ssthresh   uint32
	Rtt            uint32
	Rttvar         uint32
	Snd_ssthresh   uint32
	Snd_cwnd       uint32
	Advmss         uint32
	Reordering     uint32
	Rcv_rtt        uint32
	Rcv_space      uint32
	Total_retrans  uint32
}

func ExtendedStatsAvailable() bool {
	return true
}

func (es *AggregateExtendedStats) IncorporateConnectionStats(basicConn net.Conn) error {
	if info, err := getTCPInfoRaw(basicConn); err != nil {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection: %v", err)
	} else {
		es.MaxMss = utilities.Max(es.MaxMss, uint64(info.Mss))
		es.TotalBytesReordered += uint64(info.BytesReordered)
		es.TotalBytesRetransmitted += uint64(info.BytesRetrans)
		es.TotalBytesSent += info.BytesOut
		es.TotalBytesReceived += info.BytesIn

		es.total_rtt += float64(info.RttUs)
		es.rtt_measurements += 1
		es.AverageRtt = es.total_rtt / float64(es.rtt_measurements)
		es.RetransmitRatio = (float64(es.TotalBytesRetransmitted) / float64(es.TotalBytesSent)) * 100.0
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
`, es.MaxMss, es.TotalBytesRetransmitted, es.RetransmitRatio, es.TotalBytesReordered, es.AverageRtt)
}

func getTCPInfoRaw(basicConn net.Conn) (*TCPINFO_V1, error) {
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

	// SIO_TCP_INFO
	// https://docs.microsoft.com/en-us/windows/win32/winsock/sio-tcp-info
	// https://github.com/tpn/winsdk-10/blob/master/Include/10.0.16299.0/shared/mstcpip.h
	iocc := uint32(windows.IOC_INOUT | windows.IOC_VENDOR | 39)

	// Should be a DWORD, 0 for version 0, 1 for version 1 tcp_info:
	// 0: https://docs.microsoft.com/en-us/windows/win32/api/mstcpip/ns-mstcpip-tcp_info_v0
	// 1: https://docs.microsoft.com/en-us/windows/win32/api/mstcpip/ns-mstcpip-tcp_info_v1
	inbuf := uint32(1)

	// Size of the inbuf variable
	cbif := uint32(4)

	outbuf := TCPINFO_V1{}

	cbob := uint32(unsafe.Sizeof(outbuf)) // Size = 136 for V1 and 88 for V0

	// Size pointer of return object
	cbbr := uint32(0)

	overlapped := windows.Overlapped{}

	completionRoutine := uintptr(0)

	rawConn.Control(func(fd uintptr) {
		err = windows.WSAIoctl(
			windows.Handle(fd),
			iocc,
			(*byte)(unsafe.Pointer(&inbuf)),
			cbif,
			(*byte)(unsafe.Pointer(&outbuf)),
			cbob,
			&cbbr,
			&overlapped,
			completionRoutine,
		)
	})
	return &outbuf, err
}

func GetTCPInfo(connection net.Conn) (*TCPInfo, error) {
	info, err := getTCPInfoRaw(connection)
	if err != nil {
		return nil, err
	}
	// Uncertain on all the statistic correlation so only transferring the needed
	return &TCPInfo{
		Rtt:      info.RttUs,
		Snd_cwnd: info.Cwnd,
	}, err
}
