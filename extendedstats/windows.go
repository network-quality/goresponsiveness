//go:build windows
// +build windows

package extendedstats

import (
	"crypto/tls"
	"fmt"
	"net"
	"unsafe"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/sys/windows"
)

type ExtendedStats struct {
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

func (es *ExtendedStats) IncorporateConnectionStats(rawConn net.Conn) error {
	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection (not a TLS connection)")
	}
	tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
	if !ok {
		return fmt.Errorf("OOPS: Could not get the TCP info for the connection (not a TCP connection)")
	}
	if info, err := getTCPInfo(tcpConn); err != nil {
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

func (es *ExtendedStats) Repr() string {
	return fmt.Sprintf(`Extended Statistics:
	Maximum Segment Size: %v
	Total Bytes Retransmitted: %v
	Retransmission Ratio: %.2f%%
	Total Bytes Reordered: %v
	Average RTT: %v
`, es.MaxMss, es.TotalBytesRetransmitted, es.RetransmitRatio, es.TotalBytesReordered, es.AverageRtt)
}

func ExtendedStatsAvailable() bool {
	return true
}

func getTCPInfo(connection net.Conn) (*TCPINFO_V1, error) {
	tcpConn, ok := connection.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("connection is not a net.TCPConn")
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
		err = windows.WSAIoctl(windows.Handle(fd), iocc, (*byte)(unsafe.Pointer(&inbuf)), cbif, (*byte)(unsafe.Pointer(&outbuf)), cbob, &cbbr, &overlapped, completionRoutine)
	})
	return &outbuf, err
}
