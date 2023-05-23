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

package probe

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"os"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
)

type ProbeTracer struct {
	client    *http.Client
	stats     *stats.TraceStats
	trace     *httptrace.ClientTrace
	debug     debug.DebugLevel
	probeid   uint
	probeType ProbeType
}

func (p *ProbeTracer) String() string {
	return fmt.Sprintf("(Probe %v): stats: %v\n", p.probeid, p.stats)
}

func (p *ProbeTracer) ProbeId() uint {
	return p.probeid
}

func (p *ProbeTracer) GetTrace() *httptrace.ClientTrace {
	return p.trace
}

func (p *ProbeTracer) GetDnsDelta() time.Duration {
	if p.stats.ConnectionReused {
		return time.Duration(0)
	}
	delta := p.stats.DnsDoneTime.Sub(p.stats.DnsStartTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): DNS Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *ProbeTracer) GetTCPDelta() time.Duration {
	if p.stats.ConnectionReused {
		return time.Duration(0)
	}
	delta := p.stats.ConnectDoneTime.Sub(p.stats.ConnectStartTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): TCP Connection Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *ProbeTracer) GetTLSDelta() time.Duration {
	if utilities.IsSome(p.stats.TLSDoneTime) {
		panic("There should not be TLS information, but there is.")
	}
	delta := time.Duration(0)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): TLS Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *ProbeTracer) GetTLSAndHttpHeaderDelta() time.Duration {
	// Because the TLS handshake occurs *after* the TCP connection (ConnectDoneTime)
	// and *before* the HTTP transaction, we know that the delta between the time
	// that the first HTTP response byte is available and the time that the TCP
	// connection was established includes both the time for the HTTP header RTT
	// *and* the TLS handshake RTT, whether we can specifically measure the latter
	// or not. Eventually when TLS handshake tracing is fixed, we can break these
	// into separate buckets, but for now this workaround is reasonable.
	before := p.stats.ConnectDoneTime
	if p.stats.ConnectionReused {
		// When we reuse a connection there will be no time logged for when the
		// TCP connection was established (obviously). So, fall back to the time
		// when we were notified about reusing a connection (as a close approximation!).
		before = p.stats.GetConnectionDoneTime
	}
	delta := p.stats.HttpResponseReadyTime.Sub(before)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http TLS and Header Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *ProbeTracer) GetHttpHeaderDelta() time.Duration {
	panic(
		"Unusable until TLS tracing support is enabled! Use GetTLSAndHttpHeaderDelta() instead.\n",
	)
	delta := p.stats.HttpResponseReadyTime.Sub(utilities.GetSome(p.stats.TLSDoneTime))
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http Header Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *ProbeTracer) GetHttpDownloadDelta(httpDoneTime time.Time) time.Duration {
	delta := httpDoneTime.Sub(p.stats.HttpResponseReadyTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http Download Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (probe *ProbeTracer) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {
	probe.stats.DnsStartTime = now
	probe.stats.DnsStart = dnsStartInfo
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) DNS Start for Probe %v: %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			dnsStartInfo,
		)
	}
}

func (probe *ProbeTracer) SetDnsDoneTimeInfo(
	now time.Time,
	dnsDoneInfo httptrace.DNSDoneInfo,
) {
	probe.stats.DnsDoneTime = now
	probe.stats.DnsDone = dnsDoneInfo
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) DNS Done for Probe %v: %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.DnsDone,
		)
	}
}

func (probe *ProbeTracer) SetConnectStartTime(
	now time.Time,
) {
	probe.stats.ConnectStartTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) TCP Start for Probe %v at %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.ConnectStartTime,
		)
	}
}

func (probe *ProbeTracer) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {
	probe.stats.ConnectDoneTime = now
	probe.stats.ConnectDoneError = err
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) TCP Done for Probe %v (with error %v) @ %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.ConnectDoneError,
			probe.stats.ConnectDoneTime,
		)
	}
}

func (probe *ProbeTracer) SetGetConnTime(now time.Time) {
	probe.stats.GetConnectionStartTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) Started getting connection for Probe %v @ %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.GetConnectionStartTime,
		)
	}
}

func (probe *ProbeTracer) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	probe.stats.GetConnectionDoneTime = now
	probe.stats.ConnInfo = gotConnInfo
	probe.stats.ConnectionReused = gotConnInfo.Reused
	if (probe.probeType == SelfUp || probe.probeType == SelfDown) && !gotConnInfo.Reused {
		fmt.Fprintf(
			os.Stderr,
			"(%s Probe) Probe %v at %v with info %v did not properly reuse a connection.\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.GetConnectionDoneTime,
			probe.stats.ConnInfo,
		)
	}
	if gotConnInfo.Reused {
		if debug.IsDebug(probe.debug) {
			fmt.Printf(
				"(%s Probe) Got a reused connection for Probe %v at %v with info %v.\n",
				probe.probeType.Value(),
				probe.ProbeId(),
				probe.stats.GetConnectionDoneTime,
				probe.stats.ConnInfo,
			)
		}
	}
}

func (probe *ProbeTracer) SetTLSHandshakeStartTime(
	now time.Time,
) {
	probe.stats.TLSStartTime = utilities.Some(now)
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) Started TLS Handshake for Probe %v @ %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.TLSStartTime,
		)
	}
}

func (probe *ProbeTracer) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
	probe.stats.TLSDoneTime = utilities.Some(now)
	probe.stats.TLSConnInfo = connectionState
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) Completed TLS handshake for Probe %v at %v with info %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.TLSDoneTime,
			probe.stats.TLSConnInfo,
		)
	}
}

func (probe *ProbeTracer) SetHttpWroteRequestTimeInfo(
	now time.Time,
	info httptrace.WroteRequestInfo,
) {
	probe.stats.HttpWroteRequestTime = now
	probe.stats.HttpInfo = info
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) Http finished writing request for Probe %v at %v with info %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.HttpWroteRequestTime,
			probe.stats.HttpInfo,
		)
	}
}

func (probe *ProbeTracer) SetHttpResponseReadyTime(
	now time.Time,
) {
	probe.stats.HttpResponseReadyTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(%s Probe) Http response is ready for Probe %v at %v\n",
			probe.probeType.Value(),
			probe.ProbeId(),
			probe.stats.HttpResponseReadyTime,
		)
	}
}

func NewProbeTracer(
	client *http.Client,
	probeType ProbeType,
	probeId uint,
	debugging *debug.DebugWithPrefix,
) *ProbeTracer {
	probe := &ProbeTracer{
		client:    client,
		stats:     &stats.TraceStats{},
		trace:     nil,
		debug:     debugging.Level,
		probeid:   probeId,
		probeType: probeType,
	}
	trace := traceable.GenerateHttpTimingTracer(probe, debugging.Level)

	probe.trace = trace
	return probe
}
