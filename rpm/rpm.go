package rpm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
)

type Probe struct {
	client  *http.Client
	stats   *stats.TraceStats
	trace   *httptrace.ClientTrace
	debug   debug.DebugLevel
	probeid uint64
}

func (p *Probe) String() string {
	return fmt.Sprintf("(Probe %v): stats: %v\n", p.probeid, p.stats)
}

func (p *Probe) ProbeId() uint64 {
	return p.probeid
}

func (p *Probe) GetTrace() *httptrace.ClientTrace {
	return p.trace
}

func (p *Probe) GetDnsDelta() time.Duration {
	delta := p.stats.DnsDoneTime.Sub(p.stats.DnsStartTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): DNS Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetTCPDelta() time.Duration {
	delta := p.stats.ConnectDoneTime.Sub(p.stats.ConnectStartTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): TCP Connection Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetTLSDelta() time.Duration {
	if utilities.IsSome(p.stats.TLSDoneTime) {
		panic("There should not be TLS information, but there is.")
	}
	delta := time.Duration(0)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): TLS Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetTLSAndHttpHeaderDelta() time.Duration {
	// Because the TLS handshake occurs *after* the TCP connection (ConnectDoneTime)
	// and *before* the HTTP transaction, we know that the delta between the time
	// that the first HTTP response byte is available and the time that the TCP
	// connection was established includes both the time for the HTTP header RTT
	// *and* the TLS handshake RTT, whether we can specifically measure the latter
	// or not. Eventually when TLS handshake tracing is fixed, we can break these
	// into separate buckets, but for now this workaround is reasonable.
	delta := p.stats.HttpResponseReadyTime.Sub(p.stats.ConnectDoneTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http TLS and Header Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetHttpHeaderDelta() time.Duration {
	panic("Unusable until TLS tracing support is enabled! Use GetTLSAndHttpHeaderDelta() instead.\n")
	delta := p.stats.HttpResponseReadyTime.Sub(utilities.GetSome(p.stats.TLSDoneTime))
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http Header Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetHttpDownloadDelta(httpDoneTime time.Time) time.Duration {
	delta := httpDoneTime.Sub(p.stats.HttpResponseReadyTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http Download Time: %v\n", p.probeid, delta)
	}
	return delta
}

func NewProbe(client *http.Client, debugLevel debug.DebugLevel) *Probe {
	probe := &Probe{
		client:  client,
		stats:   &stats.TraceStats{},
		trace:   nil,
		debug:   debugLevel,
		probeid: utilities.GenerateConnectionId(),
	}
	trace := traceable.GenerateHttpTimingTracer(probe, debugLevel)

	probe.trace = trace
	return probe
}

func (probe *Probe) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {
	probe.stats.DnsStartTime = now
	probe.stats.DnsStart = dnsStartInfo
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) DNS Start for %v: %v\n",
			probe.ProbeId(),
			dnsStartInfo,
		)
	}
}

func (probe *Probe) SetDnsDoneTimeInfo(
	now time.Time,
	dnsDoneInfo httptrace.DNSDoneInfo,
) {
	probe.stats.DnsDoneTime = now
	probe.stats.DnsDone = dnsDoneInfo
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) DNS Done for %v: %v\n",
			probe.ProbeId(),
			probe.stats.DnsDone,
		)
	}
}

func (probe *Probe) SetConnectStartTime(
	now time.Time,
) {
	probe.stats.ConnectStartTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) TCP Start for %v at %v\n",
			probe.ProbeId(),
			probe.stats.ConnectStartTime,
		)
	}
}

func (probe *Probe) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {
	probe.stats.ConnectDoneTime = now
	probe.stats.ConnectDoneError = err
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) TCP Done for %v (with error %v) @ %v\n",
			probe.ProbeId(),
			probe.stats.ConnectDoneError,
			probe.stats.ConnectDoneTime,
		)
	}
}

func (probe *Probe) SetGetConnTime(now time.Time) {
	probe.stats.GetConnectionStartTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Started getting connection for %v @ %v\n",
			probe.ProbeId(),
			probe.stats.GetConnectionStartTime,
		)
	}
}

func (probe *Probe) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	probe.stats.GetConnectionDoneTime = now
	probe.stats.ConnInfo = gotConnInfo
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Got connection for %v at %v with info %v\n",
			probe.ProbeId(),
			probe.stats.GetConnectionDoneTime,
			probe.stats.ConnInfo,
		)
	}
}

func (probe *Probe) SetTLSHandshakeStartTime(
	now time.Time,
) {
	probe.stats.TLSStartTime = utilities.Some(now)
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Started TLS Handshake for %v @ %v\n",
			probe.ProbeId(),
			probe.stats.TLSStartTime,
		)
	}
}

func (probe *Probe) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
	probe.stats.TLSDoneTime = utilities.Some(now)
	probe.stats.TLSConnInfo = connectionState
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Completed TLS handshake for %v at %v with info %v\n",
			probe.ProbeId(),
			probe.stats.TLSDoneTime,
			probe.stats.TLSConnInfo,
		)
	}
}

func (probe *Probe) SetHttpWroteRequestTimeInfo(
	now time.Time,
	info httptrace.WroteRequestInfo,
) {
	probe.stats.HttpWroteRequestTime = now
	probe.stats.HttpInfo = info
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Http finished writing request for %v at %v with info %v\n",
			probe.ProbeId(),
			probe.stats.HttpWroteRequestTime,
			probe.stats.HttpInfo,
		)
	}
}

func (probe *Probe) SetHttpResponseReadyTime(
	now time.Time,
) {
	probe.stats.HttpResponseReadyTime = now
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Http response is ready for %v at %v\n",
			probe.ProbeId(),
			probe.stats.HttpResponseReadyTime,
		)
	}
}

func getLatency(ctx context.Context, probe *Probe, url string, debugLevel debug.DebugLevel) utilities.GetLatency {
	before := time.Now()
	c_b_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, probe.GetTrace()),
		"GET",
		url,
		nil,
	)
	if err != nil {
		return utilities.GetLatency{Delay: 0, RoundTripCount: 0, Err: err}
	}

	c_b, err := probe.client.Do(c_b_req)
	if err != nil {
		return utilities.GetLatency{Delay: 0, RoundTripCount: 0, Err: err}
	}

	// TODO: Make this interruptable somehow by using _ctx_.
	_, err = io.ReadAll(c_b.Body)
	if err != nil {
		return utilities.GetLatency{Delay: 0, Err: err}
	}
	after := time.Now()

	// Depending on whether we think that Close() requires another RTT (via TCP), we
	// may need to move this before/after capturing the after time.
	c_b.Body.Close()

	sanity := after.Sub(before)

	tlsAndHttpHeaderDelta := probe.GetTLSAndHttpHeaderDelta() // Constitutes 2 RTT, per the Spec.
	httpDownloadDelta := probe.GetHttpDownloadDelta(after)    // Constitutes 1 RTT, per the Spec.
	dnsDelta := probe.GetDnsDelta()                           // Constitutes 1 RTT, per the Spec.
	tcpDelta := probe.GetTCPDelta()                           // Constitutes 1 RTT, per the Spec.
	totalDelay := tlsAndHttpHeaderDelta + httpDownloadDelta + dnsDelta + tcpDelta

	if debug.IsDebug(debugLevel) {
		fmt.Printf(
			"(Probe %v) sanity vs total: %v vs %v\n",
			probe.ProbeId(),
			sanity,
			totalDelay,
		)
	}

	roundTripCount := uint16(5) // According to addition, there are 5 RTTs that we measured.
	return utilities.GetLatency{Delay: totalDelay, RoundTripCount: roundTripCount, Err: nil}
}

func CalculateSequentialRTTsTime(
	ctx context.Context,
	saturated_rtt_probe *Probe,
	new_rtt_probe *Probe,
	url string,
	debugLevel debug.DebugLevel,
) chan utilities.GetLatency {
	responseChannel := make(chan utilities.GetLatency)
	go func() {

		if debug.IsDebug(debugLevel) {
			fmt.Printf("Beginning saturated RTT probe.\n")
		}
		saturated_latency := getLatency(ctx, saturated_rtt_probe, url, debugLevel)

		if saturated_latency.Err != nil {
			fmt.Printf("Error occurred getting the saturated RTT.\n")
			responseChannel <- saturated_latency
			return
		}
		/*
			  TODO: We are not going to measure round-trip times on the load-generating connection
				right now because we are dealing with a massive amount of buffer bloat on the
				Apple CDN.

				TODO: When this functionality is enabled, we may need to change the assertion in
				the GotConn callback in the Traceable interface in traceable.go because a connection
				will be reused in that case. If such a situation does come to pass, we will want to
				move that assertion in to the various Traceable interface implementations that continue
				to rely on this assertion.

				c_a, err := saturated_client.Get(url)
				if err != nil {
					responseChannel <- GetLatency{Delay: 0, RTTs: 0, Err: err}
					return
				}
				// TODO: Make this interruptable somehow
				// by using _ctx_.
				_, err = io.ReadAll(c_a.Body)
				if err != nil {
					responseChannel <- GetLatency{Delay: 0, RTTs: 0, Err: err}
					return
				}
				roundTripCount += 5
				c_a.Body.Close()
		*/
		if debug.IsDebug(debugLevel) {
			fmt.Printf("Beginning unsaturated RTT probe.\n")
		}
		new_rtt_latency := getLatency(ctx, new_rtt_probe, url, debugLevel)

		if new_rtt_latency.Err != nil {
			fmt.Printf("Error occurred getting the unsaturated RTT.\n")
			responseChannel <- new_rtt_latency
			return
		}

		responseChannel <- utilities.GetLatency{Delay: saturated_latency.Delay + new_rtt_latency.Delay, RoundTripCount: uint16(10), Err: nil}
		return
	}()
	return responseChannel
}
