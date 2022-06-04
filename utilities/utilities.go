/*
 * This file is part of Go Responsiveness.
 *
 * Go Responsiveness is free software: you can redistribute it and/or modify it under
 * the terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 * Go Responsiveness is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with Foobar. If not, see <https://www.gnu.org/licenses/>.
 */

package utilities

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"reflect"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

func IsInterfaceNil(ifc interface{}) bool {
	return ifc == nil ||
		(reflect.ValueOf(ifc).Kind() == reflect.Ptr && reflect.ValueOf(ifc).IsNil())
}

func SignedPercentDifference(
	current float64,
	previous float64,
) (difference float64) {
	//return ((current - previous) / (float64(current+previous) / 2.0)) * float64(
	//100,
	//	)
	return ((current - previous) / previous) * float64(
		100,
	)
}

func AbsPercentDifference(
	current float64,
	previous float64,
) (difference float64) {
	return (math.Abs(current-previous) / (float64(current+previous) / 2.0)) * float64(
		100,
	)
}

func Conditional(condition bool, t string, f string) string {
	if condition {
		return t
	}
	return f
}

func ToMbps(bytes float64) float64 {
	return ToMBps(bytes) * float64(8)
}

func ToMBps(bytes float64) float64 {
	return float64(bytes) / float64(1024*1024)
}

type MeasurementResult struct {
	Delay            time.Duration
	MeasurementCount uint16
	Err              error
}

func SeekForAppend(file *os.File) (err error) {
	_, err = file.Seek(0, 2)
	return
}

var GenerateConnectionId func() uint64 = func() func() uint64 {
	var nextConnectionId uint64 = 0
	return func() uint64 {
		return atomic.AddUint64(&nextConnectionId, 1)
	}
}()

type Optional[S any] struct {
	value S
	some  bool
}

func Some[S any](value S) Optional[S] {
	return Optional[S]{value: value, some: true}
}

func None[S any]() Optional[S] {
	return Optional[S]{some: false}
}

func IsNone[S any](optional Optional[S]) bool {
	return !optional.some
}

func IsSome[S any](optional Optional[S]) bool {
	return optional.some
}

func GetSome[S any](optional Optional[S]) S {
	if !optional.some {
		panic("Attempting to access Some of a None.")
	}
	return optional.value
}

func (optional Optional[S]) String() string {
	if IsSome(optional) {
		return fmt.Sprintf("Some: %v", optional.some)
	} else {
		return "None"
	}
}

func RandBetween(max int) int {
	return rand.New(rand.NewSource(int64(time.Now().Nanosecond()))).Int() % max
}

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
