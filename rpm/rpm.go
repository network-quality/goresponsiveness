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
package rpm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"

	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

func addFlows(
	ctx context.Context,
	toAdd uint64,
	lgcc *lgc.LoadGeneratingConnectionCollection,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	debug debug.DebugLevel,
) uint64 {
	lgcc.Lock.Lock()
	defer lgcc.Lock.Unlock()
	for i := uint64(0); i < toAdd; i++ {
		// First, generate the connection.
		*lgcc.LGCs = append(*lgcc.LGCs, lgcGenerator())
		// Second, try to start the connection.
		if !(*lgcc.LGCs)[len(*lgcc.LGCs)-1].Start(ctx, debug) {
			// If there was an error, we'll make sure that the caller knows it.
			fmt.Printf(
				"Error starting lgc with id %d!\n",
				(*lgcc.LGCs)[len(*lgcc.LGCs)-1].ClientId(),
			)
			return i
		}
	}
	return toAdd
}

type ProbeConfiguration struct {
	URL  string
	Host string
}

type ProbeDataPoint struct {
	Time           time.Time     `Description:"Time of the generation of the data point."                    Formatter:"Format"  FormatterArgument:"01-02-2006-15-04-05.000"`
	RoundTripCount uint64        `Description:"The number of round trips measured by this data point."`
	Duration       time.Duration `Description:"The duration for this measurement."                           Formatter:"Seconds"`
	TCPRtt         time.Duration `Description:"The underlying connection's RTT at probe time."               Formatter:"Seconds"`
	TCPCwnd        uint32        `Description:"The underlying connection's congestion window at probe time."`
	Type           ProbeType     `Description:"The type of the probe."                                       Formatter:"Value"`
}

type GranularThroughputDataPoint struct {
	Time       time.Time     `Description:"Time of the generation of the data point." Formatter:"Format" FormatterArgument:"01-02-2006-15-04-05.000"`
	Throughput float64       `Description:"Instantaneous throughput (B/s)."`
	ConnID     uint32        `Description:"Position of connection (ID)."`
	TCPRtt     time.Duration `Description:"The underlying connection's RTT at probe time."               Formatter:"Seconds"`
	TCPCwnd    uint32        `Description:"The underlying connection's congestion window at probe time."`
	Direction  string        `Description:"Direction of Throughput."`
}

type ThroughputDataPoint struct {
	Time                         time.Time                     `Description:"Time of the generation of the data point." Formatter:"Format" FormatterArgument:"01-02-2006-15-04-05.000"`
	Throughput                   float64                       `Description:"Instantaneous throughput (B/s)."`
	Connections                  int                           `Description:"Number of parallel connections."`
	GranularThroughputDataPoints []GranularThroughputDataPoint `Description:"[OMIT]"`
}

type SelfDataCollectionResult struct {
	RateBps             float64
	LGCs                []lgc.LoadGeneratingConnection
	ProbeDataPoints     []ProbeDataPoint
	LoggingContinuation func()
}

type ProbeType int64

const (
	SelfUp ProbeType = iota
	SelfDown
	Foreign
)

type ProbeRoundTripCountType uint16

const (
	DefaultDownRoundTripCount ProbeRoundTripCountType = 1
	SelfUpRoundTripCount      ProbeRoundTripCountType = 1
	SelfDownRoundTripCount    ProbeRoundTripCountType = 1
	ForeignRoundTripCount     ProbeRoundTripCountType = 3
)

func (pt ProbeType) Value() string {
	if pt == SelfUp {
		return "SelfUp"
	} else if pt == SelfDown {
		return "SelfDown"
	}
	return "Foreign"
}

func Probe(
	managingCtx context.Context,
	waitGroup *sync.WaitGroup,
	client *http.Client,
	probeUrl string,
	probeHost string, // optional: for use with a test_endpoint
	probeType ProbeType,
	result *chan ProbeDataPoint,
	captureExtendedStats bool,
	debugging *debug.DebugWithPrefix,
) error {

	if waitGroup != nil {
		waitGroup.Add(1)
		defer waitGroup.Done()
	}

	if client == nil {
		return fmt.Errorf("cannot start a probe with a nil client")
	}

	probeId := utilities.GenerateUniqueId()
	probeTracer := NewProbeTracer(client, probeType, probeId, debugging)
	time_before_probe := time.Now()
	probe_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(managingCtx, probeTracer.trace),
		"GET",
		probeUrl,
		nil,
	)
	if err != nil {
		return err
	}

	// To support test_endpoint
	if len(probeHost) != 0 {
		probe_req.Host = probeHost
	}
	// Used to disable compression
	probe_req.Header.Set("Accept-Encoding", "identity")

	probe_resp, err := client.Do(probe_req)
	if err != nil {
		return err
	}

	// Header.Get returns "" when not set
	if probe_resp.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("Content-Encoding header was set (compression not allowed)")
	}

	// TODO: Make this interruptable somehow by using _ctx_.
	_, err = io.ReadAll(probe_resp.Body)
	if err != nil {
		return err
	}
	time_after_probe := time.Now()

	// Depending on whether we think that Close() requires another RTT (via TCP), we
	// may need to move this before/after capturing the after time.
	probe_resp.Body.Close()

	sanity := time_after_probe.Sub(time_before_probe)

	// When the probe is run on a load-generating connection (a self probe) there should
	// only be a single round trip that is measured. We will take the accumulation of all these
	// values just to be sure, though. Because of how this traced connection was launched, most
	// of the values will be 0 (or very small where the time that go takes for delivering callbacks
	// and doing context switches pokes through). When it is !isSelfProbe then the values will
	// be significant and we want to add them regardless!
	totalDelay := probeTracer.GetTLSAndHttpHeaderDelta() + probeTracer.GetHttpDownloadDelta(
		time_after_probe,
	) + probeTracer.GetTCPDelta()

	// We must have reused the connection if we are a self probe!
	if (probeType == SelfUp || probeType == SelfDown) && !probeTracer.stats.ConnectionReused {
		panic(!probeTracer.stats.ConnectionReused)
	}

	if debug.IsDebug(debugging.Level) {
		fmt.Printf(
			"(%s) (%s Probe %v) sanity vs total: %v vs %v\n",
			debugging.Prefix,
			probeType.Value(),
			probeId,
			sanity,
			totalDelay,
		)
	}
	roundTripCount := DefaultDownRoundTripCount
	if probeType == Foreign {
		roundTripCount = ForeignRoundTripCount
	}
	// Careful!!! It's possible that this channel has been closed because the Prober that
	// started it has been stopped. Writing to a closed channel will cause a panic. It might not
	// matter because a panic just stops the go thread containing the paniced code and we are in
	// a go thread that executes only this function.
	defer func() {
		isThreadPanicing := recover()
		if isThreadPanicing != nil && debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) (%s Probe %v) Probe attempted to write to the result channel after its invoker ended (official reason: %v).\n",
				debugging.Prefix,
				probeType.Value(),
				probeId,
				isThreadPanicing,
			)
		}
	}()
	tcpRtt := time.Duration(0 * time.Second)
	tcpCwnd := uint32(0)
	// TODO: Only get the extended stats for a connection if the user has requested them overall.
	if captureExtendedStats && extendedstats.ExtendedStatsAvailable() {
		tcpInfo, err := extendedstats.GetTCPInfo(probeTracer.stats.ConnInfo.Conn)
		if err == nil {
			tcpRtt = time.Duration(tcpInfo.Rtt) * time.Microsecond
			tcpCwnd = tcpInfo.Snd_cwnd
		} else {
			fmt.Printf("Warning: Could not fetch the extended stats for a probe: %v\n", err)
		}
	}
	dataPoint := ProbeDataPoint{
		Time:           time_before_probe,
		RoundTripCount: uint64(roundTripCount),
		Duration:       totalDelay,
		TCPRtt:         tcpRtt,
		TCPCwnd:        tcpCwnd,
		Type:           probeType,
	}
	*result <- dataPoint
	return nil
}

func CombinedProber(
	proberCtx context.Context,
	networkActivityCtx context.Context,
	foreignProbeConfigurationGenerator func() ProbeConfiguration,
	selfProbeConfigurationGenerator func() ProbeConfiguration,
	selfDownProbeConnection lgc.LoadGeneratingConnection,
	selfUpProbeConnection lgc.LoadGeneratingConnection,
	probeInterval time.Duration,
	keyLogger io.Writer,
	captureExtendedStats bool,
	debugging *debug.DebugWithPrefix,
) (dataPoints chan ProbeDataPoint) {

	// Make a channel to send back all the generated data points
	// when we are probing.
	dataPoints = make(chan ProbeDataPoint)

	go func() {
		wg := sync.WaitGroup{}
		probeCount := 0

		// As long as our context says that we can continue to probe!
		for proberCtx.Err() == nil {

			time.Sleep(probeInterval)

			foreignProbeConfiguration := foreignProbeConfigurationGenerator()
			selfProbeConfiguration := selfProbeConfigurationGenerator()

			if debug.IsDebug(debugging.Level) {
				fmt.Printf(
					"(%s) About to send round %d of probes!\n",
					debugging.Prefix,
					probeCount+1,
				)
			}
			transport := http2.Transport{}
			transport.TLSClientConfig = &tls.Config{}

			if !utilities.IsInterfaceNil(keyLogger) {
				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"Using an SSL Key Logger for this foreign probe.\n",
					)
				}

				// The presence of a custom TLSClientConfig in a *generic* `transport`
				// means that go will default to HTTP/1.1 and cowardly avoid HTTP/2:
				// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L278
				// Also, it would appear that the API's choice of HTTP vs HTTP2 can
				// depend on whether the url contains
				// https:// or http://:
				// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L74
				transport.TLSClientConfig.KeyLogWriter = keyLogger
			}
			transport.TLSClientConfig.InsecureSkipVerify = true

			foreignProbeClient := &http.Client{Transport: &transport}

			// Start Foreign Connection Prober
			probeCount++
			go Probe(
				networkActivityCtx,
				&wg,
				foreignProbeClient,
				foreignProbeConfiguration.URL,
				foreignProbeConfiguration.Host,
				Foreign,
				&dataPoints,
				captureExtendedStats,
				debugging,
			)

			// Start Self Download Connection Prober
			go Probe(
				networkActivityCtx,
				&wg,
				selfDownProbeConnection.Client(),
				selfProbeConfiguration.URL,
				selfProbeConfiguration.Host,
				SelfDown,
				&dataPoints,
				captureExtendedStats,
				debugging,
			)

			// Start Self Upload Connection Prober
			go Probe(
				proberCtx,
				&wg,
				selfUpProbeConnection.Client(),
				selfProbeConfiguration.URL,
				selfProbeConfiguration.Host,
				SelfUp,
				&dataPoints,
				captureExtendedStats,
				debugging,
			)
		}
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Combined probe driver is going to start waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		utilities.OrTimeout(func() { wg.Wait() }, 2*time.Second)
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Combined probe driver is done waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		close(dataPoints)
	}()
	return
}

func LoadGenerator(
	networkActivityCtx context.Context, // Create all network connections in this context.
	loadGeneratorCtx context.Context, // Stop our activity when we no longer need to generate load.
	rampupInterval time.Duration,
	lgcGenerator func() lgc.LoadGeneratingConnection, // Use this to generate a new load-generating connection.
	loadGeneratingConnections *lgc.LoadGeneratingConnectionCollection,
	captureExtendedStats bool, // do we want to attempt to gather TCP information on these connections?
	debugging *debug.DebugWithPrefix, // How can we forget debugging?
) (probeConnectionCommunicationChannel chan lgc.LoadGeneratingConnection, // Send back a channel to communicate the connection to be used for self probes.
	throughputCalculations chan ThroughputDataPoint, // Send back all the instantaneous throughputs that we generate.
) {

	throughputCalculations = make(chan ThroughputDataPoint)
	// The channel that we are going to use to send back the connection to use for probing may not immediately
	// be read by the caller. We don't want to wait around until they are ready before we start doing our work.
	// So, we'll make it buffered.
	probeConnectionCommunicationChannel = make(chan lgc.LoadGeneratingConnection, 1)

	go func() {

		flowsCreated := uint64(0)

		flowsCreated += addFlows(
			networkActivityCtx,
			constants.StartingNumberOfLoadGeneratingConnections,
			loadGeneratingConnections,
			lgcGenerator,
			debugging.Level,
		)

		// We have at least a single load-generating channel. This channel will be the one that
		// the self probes use. Let's send it back to the caller so that they can pass it on if they need to.
		probeConnectionCommunicationChannel <- (*loadGeneratingConnections.LGCs)[0]

		nextSampleStartTime := time.Now().Add(rampupInterval)

		for currentInterval := uint64(0); true; currentInterval++ {

			// If the loadGeneratorCtx is canceled, then that means our work here is done ...
			if loadGeneratorCtx.Err() != nil {
				break
			}

			now := time.Now()
			// At each 1-second interval
			if nextSampleStartTime.Sub(now) > 0 {
				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"%v: Sleeping until %v\n",
						debugging,
						nextSampleStartTime,
					)
				}
				time.Sleep(nextSampleStartTime.Sub(now))
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Missed a one-second deadline.\n")
			}
			nextSampleStartTime = time.Now().Add(time.Second)

			// Compute "instantaneous aggregate" goodput which is the number of
			// bytes transferred within the last second.
			var instantaneousTotalThroughput float64 = 0
			granularThroughputDatapoints := make([]GranularThroughputDataPoint, 0)
			now = time.Now() // Used to align granular throughput data
			allInvalid := true
			for i := range *loadGeneratingConnections.LGCs {
				if !(*loadGeneratingConnections.LGCs)[i].IsValid() {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf(
							"%v: Load-generating connection with id %d is invalid ... skipping.\n",
							debugging,
							(*loadGeneratingConnections.LGCs)[i].ClientId(),
						)
					}
					// TODO: Do we add null connection to throughput? and how do we define it? Throughput -1 or 0?
					granularThroughputDatapoints = append(
						granularThroughputDatapoints,
						GranularThroughputDataPoint{now, 0, uint32(i), 0, 0, ""},
					)
					continue
				}
				allInvalid = false
				currentTransferred, currentInterval := (*loadGeneratingConnections.LGCs)[i].TransferredInInterval()
				// normalize to a second-long interval!
				instantaneousConnectionThroughput := float64(
					currentTransferred,
				) / float64(
					currentInterval.Seconds(),
				)
				instantaneousTotalThroughput += instantaneousConnectionThroughput

				tcpRtt := time.Duration(0 * time.Second)
				tcpCwnd := uint32(0)
				if captureExtendedStats && extendedstats.ExtendedStatsAvailable() {
					if stats := (*loadGeneratingConnections.LGCs)[i].Stats(); stats != nil {
						tcpInfo, err := extendedstats.GetTCPInfo(stats.ConnInfo.Conn)
						if err == nil {
							tcpRtt = time.Duration(tcpInfo.Rtt) * time.Microsecond
							tcpCwnd = tcpInfo.Snd_cwnd
						} else {
							fmt.Printf("Warning: Could not fetch the extended stats for a probe: %v\n", err)
						}
					}
				}
				granularThroughputDatapoints = append(
					granularThroughputDatapoints,
					GranularThroughputDataPoint{
						now,
						instantaneousConnectionThroughput,
						uint32(i),
						tcpRtt,
						tcpCwnd,
						"",
					},
				)
			}

			// For some reason, all the lgcs are invalid. This likely means that
			// the network/server went away.
			if allInvalid {
				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"%v: All lgcs were invalid. Assuming that network/server went away.\n",
						debugging,
					)
				}
				break
			}

			// We have generated a throughput calculation -- let's send it back to the coordinator
			throughputDataPoint := ThroughputDataPoint{
				time.Now(),
				instantaneousTotalThroughput,
				len(*loadGeneratingConnections.LGCs),
				granularThroughputDatapoints,
			}
			throughputCalculations <- throughputDataPoint

			// Just add another constants.AdditiveNumberOfLoadGeneratingConnections flows -- that's our only job now!
			flowsCreated += addFlows(
				networkActivityCtx,
				constants.AdditiveNumberOfLoadGeneratingConnections,
				loadGeneratingConnections,
				lgcGenerator,
				debugging.Level,
			)
		}

		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Stopping a load generator after creating %d flows.\n",
				debugging.Prefix, flowsCreated)
		}
	}()
	return
}

type ProbeTracer struct {
	client    *http.Client
	stats     *stats.TraceStats
	trace     *httptrace.ClientTrace
	debug     debug.DebugLevel
	probeid   uint64
	probeType ProbeType
}

func (p *ProbeTracer) String() string {
	return fmt.Sprintf("(Probe %v): stats: %v\n", p.probeid, p.stats)
}

func (p *ProbeTracer) ProbeId() uint64 {
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

func NewProbeTracer(
	client *http.Client,
	probeType ProbeType,
	probeId uint64,
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
			"A self probe sent used a new connection!\n",
		)
	}
	if gotConnInfo.Reused {
		if debug.IsDebug(probe.debug) {
			fmt.Printf(
				"(%s Probe) Got a reused connection for Probe %v at %v with info %v\n",
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
