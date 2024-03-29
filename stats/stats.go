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

package stats

import (
	"crypto/tls"
	"fmt"
	"net/http/httptrace"
	"time"

	"github.com/network-quality/goresponsiveness/utilities"
)

type TraceStats struct {
	DnsStart               httptrace.DNSStartInfo
	DnsDone                httptrace.DNSDoneInfo
	ConnInfo               httptrace.GotConnInfo
	HttpInfo               httptrace.WroteRequestInfo
	TLSConnInfo            tls.ConnectionState
	ConnectDoneError       error
	DnsStartTime           time.Time
	DnsDoneTime            time.Time
	TLSStartTime           utilities.Optional[time.Time]
	TLSDoneTime            utilities.Optional[time.Time]
	ConnectStartTime       time.Time
	ConnectDoneTime        time.Time
	ConnectionReused       bool
	GetConnectionStartTime time.Time
	GetConnectionDoneTime  time.Time
	HttpWroteRequestTime   time.Time
	HttpResponseReadyTime  time.Time
}

func NewStats() *TraceStats {
	return &TraceStats{}
}

func (s *TraceStats) String() string {
	return fmt.Sprintf("DnsStart: %v\n", s.DnsStart) +
		fmt.Sprintf("DnsDone: %v\n", s.DnsDone) +
		fmt.Sprintf("ConnInfo: %v\n", s.ConnInfo) +
		fmt.Sprintf("HttpInfo: %v\n", s.HttpInfo) +
		fmt.Sprintf("TLSConnInfo: %v\n", s.TLSConnInfo) +
		fmt.Sprintf("ConnectDoneError: %v\n", s.ConnectDoneError) +
		fmt.Sprintf("DnsStartTime: %v\n", s.DnsStartTime) +
		fmt.Sprintf("DnsDoneTime: %v\n", s.DnsDoneTime) +
		fmt.Sprintf("TLSDoneTime: %v\n", s.TLSDoneTime) +
		fmt.Sprintf("ConnectStartTime: %v\n", s.ConnectStartTime) +
		fmt.Sprintf("ConnectDoneTime: %v\n", s.ConnectDoneTime) +
		fmt.Sprintf("ConnectionReused: %v\n", s.ConnectionReused) +
		fmt.Sprintf("GetConnectionStartTime: %v\n", s.GetConnectionStartTime) +
		fmt.Sprintf("GetConnectionDoneTime: %v\n", s.GetConnectionDoneTime) +
		fmt.Sprintf("HttpResponseReadyTime: %v\n", s.HttpResponseReadyTime)
}
