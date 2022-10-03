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
	"github.com/network-quality/goresponsiveness/datalogger"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/ma"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

func addFlows(
	ctx context.Context,
	toAdd uint64,
	lgcs *[]lgc.LoadGeneratingConnection,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	debug debug.DebugLevel,
) {
	for i := uint64(0); i < toAdd; i++ {
		*lgcs = append(*lgcs, lgcGenerator())
		if !(*lgcs)[len(*lgcs)-1].Start(ctx, debug) {
			fmt.Printf(
				"Error starting lgc with id %d!\n",
				(*lgcs)[len(*lgcs)-1].ClientId(),
			)
			return
		}
	}
}

type ProbeConfiguration struct {
	URL        string
	DataLogger datalogger.DataLogger[ProbeDataPoint]
	Interval   time.Duration
}

type ProbeDataPoint struct {
	Time           time.Time     `Description:"Time of the generation of the data point."                    Formatter:"Format"  FormatterArgument:"01-02-2006-15-04-05.000"`
	RoundTripCount uint64        `Description:"The number of round trips measured by this data point."`
	Duration       time.Duration `Description:"The duration for this measurement."                           Formatter:"Seconds"`
	TCPRtt         time.Duration `Description:"The underlying connection's RTT at probe time."               Formatter:"Seconds"`
	TCPCwnd        uint32        `Description:"The underlying connection's congestion window at probe time."`
}

type ThroughputDataPoint struct {
	Time       time.Time `Description:"Time of the generation of the data point." Formatter:"Format" FormatterArgument:"01-02-2006-15-04-05.000"`
	Throughput float64   `Description:"Instantaneous throughput (b/s)."`
}

type SelfDataCollectionResult struct {
	RateBps             float64
	LGCs                []lgc.LoadGeneratingConnection
	ProbeDataPoints     []ProbeDataPoint
	LoggingContinuation func()
}

type ProbeType int64

const (
	Self ProbeType = iota
	Foreign
)

func (pt ProbeType) Value() string {
	if pt == Self {
		return "Self"
	}
	return "Foreign"
}

func Probe(
	parentProbeCtx context.Context,
	waitGroup *sync.WaitGroup,
	logger datalogger.DataLogger[ProbeDataPoint],
	client *http.Client,
	probeUrl string,
	probeType ProbeType,
	result *chan ProbeDataPoint,
	debugging *debug.DebugWithPrefix,
) error {

	if waitGroup != nil {
		waitGroup.Add(1)
		defer waitGroup.Done()
	}

	if client == nil {
		return fmt.Errorf("Cannot start a probe with a nil client")
	}

	probeId := utilities.GenerateUniqueId()
	probeTracer := NewProbeTracer(client, probeType, probeId, debugging)
	time_before_probe := time.Now()
	probe_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(parentProbeCtx, probeTracer.trace),
		"GET",
		probeUrl,
		nil,
	)
	if err != nil {
		return err
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
	if probeType == Self && !probeTracer.stats.ConnectionReused {
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
	roundTripCount := uint64(1)
	if probeType == Foreign {
		roundTripCount = 3
	}
	// TODO: Careful!!! It's possible that this channel has been closed because the Prober that
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
	if extendedstats.ExtendedStatsAvailable() {
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
		RoundTripCount: roundTripCount,
		Duration:       totalDelay,
		TCPRtt:         tcpRtt,
		TCPCwnd:        tcpCwnd,
	}
	if !utilities.IsInterfaceNil(logger) {
		logger.LogRecord(dataPoint)
	}
	*result <- dataPoint
	return nil
}

func ForeignProber(
	proberCtx context.Context,
	foreignProbeConfigurationGenerator func() ProbeConfiguration,
	keyLogger io.Writer,
	debugging *debug.DebugWithPrefix,
) (points chan ProbeDataPoint) {
	points = make(chan ProbeDataPoint)

	foreignProbeConfiguration := foreignProbeConfigurationGenerator()

	go func() {
		wg := sync.WaitGroup{}
		probeCount := 0

		for proberCtx.Err() == nil {
			time.Sleep(foreignProbeConfiguration.Interval)

			if debug.IsDebug(debugging.Level) {
				fmt.Printf(
					"(%s) About to start foreign probe number %d!\n",
					debugging.Prefix,
					probeCount,
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

			client := &http.Client{Transport: &transport}

			probeCount++
			go Probe(
				proberCtx,
				&wg,
				foreignProbeConfiguration.DataLogger,
				client,
				foreignProbeConfiguration.URL,
				Foreign,
				&points,
				debugging,
			)
		}
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Foreign probe driver is going to start waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		utilities.OrTimeout(func() { wg.Wait() }, 2*time.Second)
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Foreign probe driver is done waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		close(points)
	}()
	return
}

func SelfProber(
	proberCtx context.Context,
	defaultConnection lgc.LoadGeneratingConnection,
	altConnections *[]lgc.LoadGeneratingConnection,
	selfProbeConfiguration ProbeConfiguration,
	debugging *debug.DebugWithPrefix,
) (points chan ProbeDataPoint) {
	points = make(chan ProbeDataPoint)

	debugging = debug.NewDebugWithPrefix(debugging.Level, debugging.Prefix+" self probe")

	go func() {
		wg := sync.WaitGroup{}
		probeCount := 0
		for proberCtx.Err() == nil {
			time.Sleep(selfProbeConfiguration.Interval)
			if debug.IsDebug(debugging.Level) {
				fmt.Printf(
					"(%s) About to start self probe number %d!\n",
					debugging.Prefix,
					probeCount,
				)
			}
			probeCount++
			// TODO: We do not yet take in to account that the load-generating connection that we were given
			// on which to perform measurements might go away during testing. We have access to all the open
			// load-generating connections (altConnections) to handle this case, but we just aren't using them
			// yet.
			go Probe(
				proberCtx,
				&wg,
				selfProbeConfiguration.DataLogger,
				defaultConnection.Client(),
				selfProbeConfiguration.URL,
				Self,
				&points,
				debugging,
			)
		}
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Self probe driver is going to start waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		utilities.OrTimeout(func() { wg.Wait() }, 2*time.Second)
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Self probe driver is stopping after sending %d probes.\n",
				debugging.Prefix,
				probeCount,
			)
		}
		close(points)
	}()
	return
}

func LGCollectData(
	saturationCtx context.Context,
	networkActivityCtx context.Context,
	controlCtx context.Context,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	selfProbeConfigurationGenerator func() ProbeConfiguration,
	throughputDataLogger datalogger.DataLogger[ThroughputDataPoint],
	debugging *debug.DebugWithPrefix,
) (saturated chan bool, resulted chan SelfDataCollectionResult) {
	resulted = make(chan SelfDataCollectionResult)
	saturated = make(chan bool)
	go func() {

		isSaturated := false

		lgcs := make([]lgc.LoadGeneratingConnection, 0)

		addFlows(
			networkActivityCtx,
			constants.StartingNumberOfLoadGeneratingConnections,
			&lgcs,
			lgcGenerator,
			debugging.Level,
		)

		selfProbeCtx, selfProbeCtxCancel := context.WithCancel(saturationCtx)
		probeDataPointsChannel := SelfProber(selfProbeCtx,
			lgcs[0],
			&lgcs,
			selfProbeConfigurationGenerator(),
			debugging,
		)

		previousFlowIncreaseInterval := uint64(0)
		previousMovingAverage := float64(0)

		// The moving average will contain the average for the last
		// constants.MovingAverageIntervalCount throughputs.
		// ie, ma[i] = (throughput[i-3] + throughput[i-2] + throughput[i-1] + throughput[i])/4
		movingAverage := ma.NewMovingAverage(
			constants.MovingAverageIntervalCount,
		)

		// The moving average average will be the average of the last
		// constants.MovingAverageIntervalCount moving averages.
		// ie, maa[i] = (ma[i-3] + ma[i-2] + ma[i-1] + ma[i])/4
		movingAverageAverage := ma.NewMovingAverage(
			constants.MovingAverageIntervalCount,
		)

		nextSampleStartTime := time.Now().Add(time.Second)

		for currentInterval := uint64(0); true; currentInterval++ {

			// Stop if the client has reached saturation on both sides (up and down)
			if saturationCtx.Err() != nil {
				if debug.IsDebug(debugging.Level) {
					fmt.Printf("%v: Stopping data-collection/saturation loop at %v because both sides are saturated.", debugging, time.Now())
				}
				break
			}

			// Stop if we timed out! Send back false to indicate that we are returning under duress.
			if controlCtx.Err() != nil {
				if debug.IsDebug(debugging.Level) {
					fmt.Printf("%v: Stopping data-collection/saturation loop at %v because our controller told us to do so.", debugging, time.Now())
				}
				saturated <- false
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
			var totalTransfer float64 = 0
			allInvalid := true
			for i := range lgcs {
				if !lgcs[i].IsValid() {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf(
							"%v: Load-generating connection with id %d is invalid ... skipping.\n",
							debugging,
							lgcs[i].ClientId(),
						)
					}
					continue
				}
				allInvalid = false
				currentTransferred, currentInterval := lgcs[i].TransferredInInterval()
				// normalize to a second-long interval!
				instantaneousTransferred := float64(
					currentTransferred,
				) / float64(
					currentInterval.Seconds(),
				)
				totalTransfer += instantaneousTransferred
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

			// Compute a moving average of the last
			// constants.MovingAverageIntervalCount "instantaneous aggregate
			// goodput" measurements
			movingAverage.AddMeasurement(float64(totalTransfer))
			currentMovingAverage := movingAverage.CalculateAverage()
			movingAverageAverage.AddMeasurement(currentMovingAverage)
			movingAverageDelta := utilities.SignedPercentDifference(
				currentMovingAverage,
				previousMovingAverage,
			)

			if !utilities.IsInterfaceNil(throughputDataLogger) {
				throughputDataLogger.LogRecord(
					ThroughputDataPoint{time.Now(), currentMovingAverage},
				)
			}

			if debug.IsDebug(debugging.Level) {
				fmt.Printf(
					"%v: Instantaneous goodput: %f MB.\n",
					debugging,
					utilities.ToMBps(float64(totalTransfer)),
				)
				fmt.Printf(
					"%v: Previous moving average: %f MB.\n",
					debugging,
					utilities.ToMBps(previousMovingAverage),
				)
				fmt.Printf(
					"%v: Current moving average: %f MB.\n",
					debugging,
					utilities.ToMBps(currentMovingAverage),
				)
				fmt.Printf(
					"%v: Moving average delta: %f.\n",
					debugging,
					movingAverageDelta,
				)
			}

			previousMovingAverage = currentMovingAverage

			intervalsSinceLastFlowIncrease := currentInterval - previousFlowIncreaseInterval

			// Special case: We won't make any adjustments on the first
			// iteration.
			// Special case: If we are already saturated, let's move on.
			//               We would already be saturated and want to continue
			//               to do this loop because we are still generating good
			//               data!
			if currentInterval == 0 || isSaturated {
				continue
			}

			// If moving average > "previous" moving average + InstabilityDelta:
			if movingAverageDelta > constants.InstabilityDelta {
				// Network did not yet reach saturation. If no flows added
				// within the last 4 seconds, add 4 more flows
				if intervalsSinceLastFlowIncrease > constants.MovingAverageStabilitySpan {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf(
							"%v: Adding flows because we are unsaturated and waited a while.\n",
							debugging,
						)
					}
					addFlows(
						networkActivityCtx,
						constants.AdditiveNumberOfLoadGeneratingConnections,
						&lgcs,
						lgcGenerator,
						debugging.Level,
					)
					previousFlowIncreaseInterval = currentInterval
				} else {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf("%v: We are unsaturated, but it still too early to add anything.\n", debugging)
					}
				}
			} else { // Else, network reached saturation for the current flow count.
				if debug.IsDebug(debugging.Level) {
					fmt.Printf("%v: Network reached saturation with current flow count.\n", debugging)
				}
				// If new flows added and for 4 seconds the moving average
				// throughput did not change: network reached stable saturation
				if intervalsSinceLastFlowIncrease < constants.MovingAverageStabilitySpan && movingAverageAverage.AllSequentialIncreasesLessThan(constants.InstabilityDelta) {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf("%v: New flows added within the last four seconds and the moving-average average is consistent!\n", debugging)
					}
					// Do not break -- we want to continue looping so that we can continue to log.
					// See comment at the beginning of the loop for its terminating condition.
					isSaturated = true

					// But, we do send back a flare that says we are saturated (and happily so)!
					saturated <- true
				} else {
					// Else, add four more flows
					if debug.IsDebug(debugging.Level) {
						fmt.Printf("%v: New flows to add to try to increase our saturation!\n", debugging)
					}
					addFlows(networkActivityCtx, constants.AdditiveNumberOfLoadGeneratingConnections, &lgcs, lgcGenerator, debugging.Level)
					previousFlowIncreaseInterval = currentInterval
				}
			}
		}
		// For whatever reason, we are done. Let's report our results.

		// In the case that we ended happily, there should be no reason to do this (because
		// the self-probe context is a descendant of the saturation context). However, if we
		// were cancelled because of a timeout, we will need to explicitly cancel it. Multiple
		// calls to a cancel function are a-okay.
		selfProbeCtxCancel()

		selfProbeDataPoints := make([]ProbeDataPoint, 0)
		for dataPoint := range probeDataPointsChannel {
			selfProbeDataPoints = append(selfProbeDataPoints, dataPoint)
		}
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Collected %d self data points\n",
				debugging.Prefix,
				len(selfProbeDataPoints),
			)
		}
		resulted <- SelfDataCollectionResult{RateBps: movingAverage.CalculateAverage(), LGCs: lgcs, ProbeDataPoints: selfProbeDataPoints}
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
	if probe.probeType == Self && !gotConnInfo.Reused {
		fmt.Fprintf(
			os.Stderr,
			"A self probe sent used a new connection!\n",
		)
	} else if debug.IsDebug(probe.debug) {
		fmt.Printf("Properly reused a connection when doing a self probe!\n")
	}
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
