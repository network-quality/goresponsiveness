//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd
// +build darwin dragonfly freebsd linux netbsd openbsd

package utilities

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func GetTCPInfo(connection net.Conn) (*unix.TCPInfo, error) {
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

func PrintTCPInfo(info *unix.TCPInfo) {
	fmt.Printf("TCPInfo: \n")
	fmt.Printf("	State: %v\n", info.State)
	fmt.Printf("	Ca_state: %v\n", info.Ca_state)
	fmt.Printf("	Retransmits: %v\n", info.Retransmits)
	fmt.Printf("	Probes: %v\n", info.Probes)
	fmt.Printf("	Backoff: %v\n", info.Backoff)
	fmt.Printf("	Options: %v\n", info.Options)
	fmt.Printf("	Rto: %v\n", info.Rto)
	fmt.Printf("	Ato: %v\n", info.Ato)
	fmt.Printf("	Snd_mss: %v\n", info.Snd_mss)
	fmt.Printf("	Rcv_mss: %v\n", info.Rcv_mss)
	fmt.Printf("	Unacked: %v\n", info.Unacked)
	fmt.Printf("	Sacked: %v\n", info.Sacked)
	fmt.Printf("	Lost: %v\n", info.Lost)
	fmt.Printf("	Retrans: %v\n", info.Retrans)
	fmt.Printf("	Fackets: %v\n", info.Fackets)
	fmt.Printf("	Last_data_sent: %v\n", info.Last_data_sent)
	fmt.Printf("	Last_ack_sent: %v\n", info.Last_ack_sent)
	fmt.Printf("	Last_data_recv: %v\n", info.Last_data_recv)
	fmt.Printf("	Last_ack_recv: %v\n", info.Last_ack_recv)
	fmt.Printf("	Pmtu: %v\n", info.Pmtu)
	fmt.Printf("	Rcv_ssthresh: %v\n", info.Rcv_ssthresh)
	fmt.Printf("	Rtt: %v\n", info.Rtt)
	fmt.Printf("	Rttvar: %v\n", info.Rttvar)
	fmt.Printf("	Snd_ssthresh: %v\n", info.Snd_ssthresh)
	fmt.Printf("	Snd_cwnd: %v\n", info.Snd_cwnd)
	fmt.Printf("	Advmss: %v\n", info.Advmss)
	fmt.Printf("	Reordering: %v\n", info.Reordering)
	fmt.Printf("	Rcv_rtt: %v\n", info.Rcv_rtt)
	fmt.Printf("	Rcv_space: %v\n", info.Rcv_space)
	fmt.Printf("	Total_retrans: %v\n", info.Total_retrans)
}
