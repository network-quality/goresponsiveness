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
	if p.stats.ConnectionReused {
		return time.Duration(0)
	}
	delta := p.stats.DnsDoneTime.Sub(p.stats.DnsStartTime)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): DNS Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetTCPDelta() time.Duration {
	if p.stats.ConnectionReused {
		return time.Duration(0)
	}
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
	before := p.stats.ConnectDoneTime
	if p.stats.ConnectionReused {
		// When we reuse a connection there will be no time logged for when the
		// TCP connection was established (obviously). So, fall back to the time
		// when were notified about reusing a connection (as a close approximation!).
		before = p.stats.GetConnectionDoneTime
	}
	delta := p.stats.HttpResponseReadyTime.Sub(before)
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
	probe.stats.ConnectionReused = gotConnInfo.Reused
	if debug.IsDebug(probe.debug) {
		reusedString := "(new)"
		if probe.stats.ConnectionReused {
			reusedString = "(reused)"
		}
		fmt.Printf(
			"(Probe) Got %v connection for %v at %v with info %v\n",
			reusedString,
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

func getLatency(ctx context.Context, probe *Probe, url string, debugLevel debug.DebugLevel) utilities.MeasurementResult {
	time_before_probe := time.Now()
	probe_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, probe.GetTrace()),
		"GET",
		url,
		nil,
	)
	if err != nil {
		return utilities.MeasurementResult{Delay: 0, MeasurementCount: 0, Err: err}
	}

	probe_resp, err := probe.client.Do(probe_req)
	if err != nil {
		return utilities.MeasurementResult{Delay: 0, MeasurementCount: 0, Err: err}
	}

	// TODO: Make this interruptable somehow by using _ctx_.
	_, err = io.ReadAll(probe_resp.Body)
	if err != nil {
		return utilities.MeasurementResult{Delay: 0, Err: err}
	}
	time_after_probe := time.Now()

	// Depending on whether we think that Close() requires another RTT (via TCP), we
	// may need to move this before/after capturing the after time.
	probe_resp.Body.Close()

	sanity := time_after_probe.Sub(time_before_probe)

	tlsAndHttpHeaderDelta := probe.GetTLSAndHttpHeaderDelta()
	httpDownloadDelta := probe.GetHttpDownloadDelta(time_after_probe) // Combined with above, constitutes 2 time measurements, per the Spec.
	dnsDelta := probe.GetDnsDelta()                                   // Constitutes 1 time measurement, per the Spec.
	tcpDelta := probe.GetTCPDelta()                                   // Constitutes 1 time measurement, per the Spec.
	totalDelay := tlsAndHttpHeaderDelta + httpDownloadDelta + dnsDelta + tcpDelta

	// By default, assume that there was a reused connection which
	// means that we only made 1 time measurement.
	var measurementCount uint16 = 1
	if !probe.stats.ConnectionReused {
		// If we did not reuse the connection, then we made three additional time measurements.
		// See above for details on that calculation.
		measurementCount = 4
	}

	if debug.IsDebug(debugLevel) {
		fmt.Printf(
			"(Probe %v) sanity vs total: %v vs %v\n",
			probe.ProbeId(),
			sanity,
			totalDelay,
		)
	}
	return utilities.MeasurementResult{Delay: totalDelay, MeasurementCount: measurementCount, Err: nil}
}

func CalculateProbeMeasurements(
	ctx context.Context,
	strict bool,
	saturated_measurement_probe *Probe,
	new_measurement_probe *Probe,
	url string,
	debugLevel debug.DebugLevel,
) chan utilities.MeasurementResult {
	responseChannel := make(chan utilities.MeasurementResult)
	go func() {
		/*
			  TODO: We *are* measuring round-trip times on the load-generating connection
				right now. However, it is not clear if Apple is doing the same in their native
				client. We will have to adjust based on that.
		*/
		var saturated_latency utilities.MeasurementResult
		if strict {

			if debug.IsDebug(debugLevel) {
				fmt.Printf("Beginning saturated RTT probe.\n")
			}
			saturated_latency := getLatency(ctx, saturated_measurement_probe, url, debugLevel)

			if saturated_latency.Err != nil {
				fmt.Printf("Error occurred getting the saturated RTT.\n")
				responseChannel <- saturated_latency
				return
			}
		}

		if debug.IsDebug(debugLevel) {
			fmt.Printf("Beginning unsaturated RTT probe.\n")
		}
		new_rtt_latency := getLatency(ctx, new_measurement_probe, url, debugLevel)

		if new_rtt_latency.Err != nil {
			fmt.Printf("Error occurred getting the unsaturated RTT.\n")
			responseChannel <- new_rtt_latency
			return
		}

		total_delay := new_rtt_latency.Delay
		total_measurement_count := new_rtt_latency.MeasurementCount

		if strict {
			total_delay += saturated_latency.Delay
			total_measurement_count += saturated_latency.MeasurementCount
		}
		responseChannel <- utilities.MeasurementResult{Delay: total_delay, MeasurementCount: total_measurement_count, Err: nil}
		return
	}()
	return responseChannel
}
