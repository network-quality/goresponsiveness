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

type SaturationResult struct {
	RateBps float64
	LGCs    []lgc.LoadGeneratingConnection
}

func Saturate(
	saturationCtx context.Context,
	operatingCtx context.Context,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	debugging *debug.DebugWithPrefix,
) (saturated chan SaturationResult) {
	saturated = make(chan SaturationResult)
	go func() {

		lgcs := make([]lgc.LoadGeneratingConnection, 0)

		addFlows(
			saturationCtx,
			constants.StartingNumberOfLoadGeneratingConnections,
			&lgcs,
			lgcGenerator,
			debugging.Level,
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
			if saturationCtx.Err() != nil {
				return
			}

			// We may be asked to stop trying to saturate the
			// network and return our current status.
			if saturationCtx.Err() != nil {
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
						saturationCtx,
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
					addFlows(saturationCtx, constants.AdditiveNumberOfLoadGeneratingConnections, &lgcs, lgcGenerator, debugging.Level)
					previousFlowIncreaseInterval = currentInterval
				}
			}

		}
		saturated <- SaturationResult{RateBps: movingAverage.CalculateAverage(), LGCs: lgcs}
	}()
	return
}

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
		// when we were notified about reusing a connection (as a close approximation!).
		before = p.stats.GetConnectionDoneTime
	}
	delta := p.stats.HttpResponseReadyTime.Sub(before)
	if debug.IsDebug(p.debug) {
		fmt.Printf("(Probe %v): Http TLS and Header Time: %v\n", p.probeid, delta)
	}
	return delta
}

func (p *Probe) GetHttpHeaderDelta() time.Duration {
	panic(
		"Unusable until TLS tracing support is enabled! Use GetTLSAndHttpHeaderDelta() instead.\n",
	)
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

func getLatency(
	ctx context.Context,
	probe *Probe,
	url string,
	debugLevel debug.DebugLevel,
) utilities.MeasurementResult {
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
	httpDownloadDelta := probe.GetHttpDownloadDelta(
		time_after_probe,
	) // Combined with above, constitutes 2 time measurements, per the Spec.
	tcpDelta := probe.GetTCPDelta() // Constitutes 1 time measurement, per the Spec.
	totalDelay := tlsAndHttpHeaderDelta + httpDownloadDelta + tcpDelta

	// By default, assume that there was a reused connection which
	// means that we only made 1 time measurement.
	var measurementCount uint16 = 1
	if !probe.stats.ConnectionReused {
		// If we did not reuse the connection, then we made three additional time measurements.
		// See above for details on that calculation.
		measurementCount = 3
	}

	if debug.IsDebug(debugLevel) {
		fmt.Printf(
			"(Probe %v) sanity vs total: %v vs %v\n",
			probe.ProbeId(),
			sanity,
			totalDelay,
		)
	}
	return utilities.MeasurementResult{
		Delay:            totalDelay,
		MeasurementCount: measurementCount,
		Err:              nil,
	}
}

func CalculateProbeMeasurements(
	ctx context.Context,
	strict bool,
	saturated_measurement_probe *Probe,
	unsaturated_measurement_probe *Probe,
	url string,
	debugLevel debug.DebugLevel,
) chan utilities.MeasurementResult {
	responseChannel := make(chan utilities.MeasurementResult)
	go func() {
		/*
		 * Depending on whether the user wants their measurements to be strict, we will
		 * measure on the LGC.
		 */
		var saturated_probe_latency utilities.MeasurementResult
		if strict {

			if debug.IsDebug(debugLevel) {
				fmt.Printf("Beginning saturated measurement probe.\n")
			}
			saturated_latency := getLatency(ctx, saturated_measurement_probe, url, debugLevel)

			if saturated_latency.Err != nil {
				fmt.Printf("Error occurred getting the saturated measurement.\n")
				responseChannel <- saturated_latency
				return
			}
		}

		if debug.IsDebug(debugLevel) {
			fmt.Printf("Beginning unsaturated measurement probe.\n")
		}
		unsaturated_probe_latency := getLatency(ctx, unsaturated_measurement_probe, url, debugLevel)

		if unsaturated_probe_latency.Err != nil {
			fmt.Printf("Error occurred getting the unsaturated measurement.\n")
			responseChannel <- unsaturated_probe_latency
			return
		}

		total_latency := unsaturated_probe_latency.Delay
		total_measurement_count := unsaturated_probe_latency.MeasurementCount

		if strict {
			total_latency += saturated_probe_latency.Delay
			total_measurement_count += saturated_probe_latency.MeasurementCount
		}
		responseChannel <- utilities.MeasurementResult{Delay: total_latency, MeasurementCount: total_measurement_count, Err: nil}
		return
	}()
	return responseChannel
}
