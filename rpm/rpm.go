package rpm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"time"

	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/ma"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
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
	URL string
}

type DataPoint struct {
	RoundTripCount uint64
	Duration       time.Duration
}

type LGDataCollectionResult struct {
	RateBps    float64
	LGCs       []lgc.LoadGeneratingConnection
	DataPoints []DataPoint
}

func LGProbe(
	parentProbeCtx context.Context,
	connection lgc.LoadGeneratingConnection,
	lgProbeUrl string,
	result *chan DataPoint,
	debugging *debug.DebugWithPrefix,
) error {
	probeTracer := NewProbeTracer(connection.Client(), true, debugging)
	time_before_probe := time.Now()
	probe_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(parentProbeCtx, probeTracer.trace),
		"GET",
		lgProbeUrl,
		nil,
	)
	if err != nil {
		return err
	}

	probe_resp, err := connection.Client().Do(probe_req)
	if err != nil {
		return err
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

	tlsAndHttpHeaderDelta := probeTracer.GetTLSAndHttpHeaderDelta()
	httpDownloadDelta := probeTracer.GetHttpDownloadDelta(
		time_after_probe,
	) // Combined with above, constitutes 2 time measurements, per the Spec.
	tcpDelta := probeTracer.GetTCPDelta() // Constitutes 1 time measurement, per the Spec.
	totalDelay := tlsAndHttpHeaderDelta + httpDownloadDelta + tcpDelta

	// We must have reused the connection!
	if !probeTracer.stats.ConnectionReused {
		panic(!probeTracer.stats.ConnectionReused)
	}

	if debug.IsDebug(debugging.Level) {
		fmt.Printf(
			"(%s) (Probe %v) sanity vs total: %v vs %v\n",
			debugging.Prefix,
			probeTracer.ProbeId(),
			sanity,
			totalDelay,
		)
	}
	*result <- DataPoint{RoundTripCount: 1, Duration: totalDelay}

	return nil
}

func LGProber(
	proberCtx context.Context,
	defaultConnection lgc.LoadGeneratingConnection,
	altConnections *[]lgc.LoadGeneratingConnection,
	url string,
	interval time.Duration,
	debugging *debug.DebugWithPrefix,
) (points chan DataPoint) {
	points = make(chan DataPoint)

	go func() {
		for proberCtx.Err() == nil {
			time.Sleep(interval)
			if debug.IsDebug(debugging.Level) {
				fmt.Printf("(%s) About to probe!\n", debugging.Prefix)
			}
			go LGProbe(proberCtx, defaultConnection, url, &points, debugging)
		}
	}()

	return
}

func LGCollectData(
	lgDataCollectionCtx context.Context,
	operatingCtx context.Context,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	lgProbeConfigurationGenerator func() ProbeConfiguration,
	debugging *debug.DebugWithPrefix,
) (resulted chan LGDataCollectionResult) {
	resulted = make(chan LGDataCollectionResult)
	go func() {

		lgcs := make([]lgc.LoadGeneratingConnection, 0)

		addFlows(
			lgDataCollectionCtx,
			constants.StartingNumberOfLoadGeneratingConnections,
			&lgcs,
			lgcGenerator,
			debugging.Level,
		)

		lgProbeConfiguration := lgProbeConfigurationGenerator()

		LGProber(
			lgDataCollectionCtx,
			lgcs[0],
			&lgcs,
			lgProbeConfiguration.URL,
			time.Duration(100*time.Millisecond),
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

			// When the program stops operating, then stop.
			if lgDataCollectionCtx.Err() != nil {
				return
			}

			// We may be asked to stop trying to saturate the
			// network and return our current status.
			if lgDataCollectionCtx.Err() != nil {
				//break
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
			if currentInterval == 0 {
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
						lgDataCollectionCtx,
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
					break
				} else {
					// Else, add four more flows
					if debug.IsDebug(debugging.Level) {
						fmt.Printf("%v: New flows to add to try to increase our saturation!\n", debugging)
					}
					addFlows(lgDataCollectionCtx, constants.AdditiveNumberOfLoadGeneratingConnections, &lgcs, lgcGenerator, debugging.Level)
					previousFlowIncreaseInterval = currentInterval
				}
			}

		}
		resulted <- LGDataCollectionResult{RateBps: movingAverage.CalculateAverage(), LGCs: lgcs}
	}()
	return
}

type ProbeTracer struct {
	client  *http.Client
	stats   *stats.TraceStats
	trace   *httptrace.ClientTrace
	debug   debug.DebugLevel
	probeid uint64
	isLG    bool
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

func NewProbeTracer(client *http.Client, isLG bool, debugging *debug.DebugWithPrefix) *ProbeTracer {
	probe := &ProbeTracer{
		client:  client,
		stats:   &stats.TraceStats{},
		trace:   nil,
		debug:   debugging.Level,
		probeid: utilities.GenerateConnectionId(),
		isLG:    isLG,
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
			"(Probe) DNS Start for %v: %v\n",
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
			"(Probe) DNS Done for %v: %v\n",
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
			"(Probe) TCP Start for %v at %v\n",
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
			"(Probe) TCP Done for %v (with error %v) @ %v\n",
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
			"(Probe) Started getting connection for %v @ %v\n",
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
	if probe.isLG && !gotConnInfo.Reused {
		fmt.Fprintf(os.Stderr, "A probe sent on an LG Connection used a new connection!\n")
	} else if debug.IsDebug(probe.debug) {
		fmt.Printf("Properly reused a connection when probing on an LG Connection!\n")
	}
	if debug.IsDebug(probe.debug) {
		fmt.Printf(
			"(Probe) Got a reused connection for %v at %v with info %v\n",
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
			"(Probe) Started TLS Handshake for %v @ %v\n",
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
			"(Probe) Completed TLS handshake for %v at %v with info %v\n",
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
			"(Probe) Http finished writing request for %v at %v with info %v\n",
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
			"(Probe) Http response is ready for %v at %v\n",
			probe.ProbeId(),
			probe.stats.HttpResponseReadyTime,
		)
	}
}
