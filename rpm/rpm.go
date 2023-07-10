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
	"os"
	"sync"
	"time"

	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/probe"
	"github.com/network-quality/goresponsiveness/series"
	"github.com/network-quality/goresponsiveness/utilities"
)

func addFlows(
	ctx context.Context,
	toAdd uint64,
	lgcc *lgc.LoadGeneratingConnectionCollection,
	lgcGenerator func() lgc.LoadGeneratingConnection,
	debugging debug.DebugLevel,
) uint64 {
	lgcc.Lock.Lock()
	defer lgcc.Lock.Unlock()
	for i := uint64(0); i < toAdd; i++ {
		// First, generate the connection.
		newConnection := lgcGenerator()
		lgcc.Append(newConnection)
		if debug.IsDebug(debugging) {
			fmt.Printf("Added a new %s load-generating connection.\n", newConnection.Direction())
		}
		// Second, try to start the connection.
		if !newConnection.Start(ctx, debugging) {
			// If there was an error, we'll make sure that the caller knows it.
			fmt.Printf(
				"Error starting lgc with id %d!\n", newConnection.ClientId(),
			)
			return i
		}
	}
	return toAdd
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
	ActiveConnections            int                           `Description:"Number of active parallel connections."`
	Connections                  int                           `Description:"Number of parallel connections."`
	GranularThroughputDataPoints []GranularThroughputDataPoint `Description:"[OMIT]"`
}

type SelfDataCollectionResult struct {
	RateBps             float64
	LGCs                []lgc.LoadGeneratingConnection
	ProbeDataPoints     []probe.ProbeDataPoint
	LoggingContinuation func()
}

type ResponsivenessProbeResult struct {
	Foreign *probe.ProbeDataPoint
	Self    *probe.ProbeDataPoint
}

func ResponsivenessProber(
	proberCtx context.Context,
	networkActivityCtx context.Context,
	foreignProbeConfigurationGenerator func() probe.ProbeConfiguration,
	selfProbeConfigurationGenerator func() probe.ProbeConfiguration,
	selfProbeConnectionCollection *lgc.LoadGeneratingConnectionCollection,
	probeDirection lgc.LgcDirection,
	probeInterval time.Duration,
	keyLogger io.Writer,
	captureExtendedStats bool,
	debugging *debug.DebugWithPrefix,
) (dataPoints chan series.SeriesMessage[ResponsivenessProbeResult, uint]) {
	if debug.IsDebug(debugging.Level) {
		fmt.Printf(
			"(%s) Starting to collect responsiveness information at an interval of %v!\n",
			debugging.Prefix,
			probeInterval,
		)
	}

	// Make a channel to send back all the generated data points
	// when we are probing.
	dataPoints = make(chan series.SeriesMessage[ResponsivenessProbeResult, uint])

	go func() {
		wg := sync.WaitGroup{}
		probeCount := uint(0)

		dataPointsLock := sync.Mutex{}

		// As long as our context says that we can continue to probe!
		for proberCtx.Err() == nil {
			time.Sleep(probeInterval)

			// We may have slept for a very long time. So, let's check to see if we are
			// still active, just for fun!
			if proberCtx.Err() != nil {
				break
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				probeCount++
				probeCount := probeCount

				foreignProbeConfiguration := foreignProbeConfigurationGenerator()
				selfProbeConfiguration := selfProbeConfigurationGenerator()

				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"(%s) About to send round %d of probes!\n",
						debugging.Prefix,
						probeCount,
					)
				}

				dataPoints <- series.SeriesMessage[ResponsivenessProbeResult, uint]{
					Type: series.SeriesMessageReserve, Bucket: probeCount,
					Measure: utilities.None[ResponsivenessProbeResult](),
				}

				// The presence of a custom TLSClientConfig in a *generic* `transport`
				// means that go will default to HTTP/1.1 and cowardly avoid HTTP/2:
				// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L278
				// Also, it would appear that the API's choice of HTTP vs HTTP2 can
				// depend on whether the url contains
				// https:// or http://:
				// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L74
				transport := &http.Transport{}
				transport.TLSClientConfig = &tls.Config{}
				transport.Proxy = http.ProxyFromEnvironment

				if !utilities.IsInterfaceNil(keyLogger) {
					if debug.IsDebug(debugging.Level) {
						fmt.Printf(
							"Using an SSL Key Logger for a foreign probe.\n",
						)
					}

					transport.TLSClientConfig.KeyLogWriter = keyLogger
				}

				transport.TLSClientConfig.InsecureSkipVerify =
					foreignProbeConfiguration.InsecureSkipVerify

				utilities.OverrideHostTransport(transport,
					foreignProbeConfiguration.ConnectToAddr)

				foreignProbeClient := &http.Client{Transport: transport}

				// Start Foreign Connection Prober
				foreignProbeDataPoint, err := probe.Probe(
					networkActivityCtx,
					foreignProbeClient,
					foreignProbeConfiguration.URL,
					foreignProbeConfiguration.Host,
					probeDirection,
					probe.Foreign,
					probeCount,
					captureExtendedStats,
					debugging,
				)
				if err != nil {
					return
				}

				var selfProbeConnection *lgc.LoadGeneratingConnection = nil
				func() {
					selfProbeConnectionCollection.Lock.Lock()
					defer selfProbeConnectionCollection.Lock.Unlock()
					selfProbeConnection, err = selfProbeConnectionCollection.GetRandom()
					if err != nil {
						if debug.IsWarn(debugging.Level) {
							fmt.Printf(
								"(%s) Failed to get a random %s load-generating connection on which to send a probe: %v.\n",
								debugging.Prefix,
								utilities.Conditional(probeDirection == lgc.LGC_DOWN, "download", "upload"),
								err,
							)
						}
						return
					}
				}()
				if selfProbeConnection == nil {
					return
				}

				// TODO: Make the following sanity check more than just a check.
				// We only want to start a SelfUp probe on a connection that is
				// in the RUNNING state.
				if (*selfProbeConnection).Status() != lgc.LGC_STATUS_RUNNING {
					if debug.IsWarn(debugging.Level) {
						fmt.Printf(
							"(%s) The selected random %s load-generating connection on which to send a probe was not running.\n",
							debugging.Prefix,
							utilities.Conditional(probeDirection == lgc.LGC_DOWN, "download", "upload"),
						)
					}
					return
				}

				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"(%s) Selected %s load-generating connection with ID %d to send a self probe with Id %d.\n",
						debugging.Prefix,
						utilities.Conditional(probeDirection == lgc.LGC_DOWN, "download", "upload"),
						(*selfProbeConnection).ClientId(),
						probeCount,
					)
				}
				selfProbeDataPoint, err := probe.Probe(
					proberCtx,
					(*selfProbeConnection).Client(),
					selfProbeConfiguration.URL,
					selfProbeConfiguration.Host,
					probeDirection,
					utilities.Conditional(probeDirection == lgc.LGC_DOWN, probe.SelfDown, probe.SelfUp),
					probeCount,
					captureExtendedStats,
					debugging,
				)
				if err != nil {
					fmt.Printf(
						"(%s) There was an error sending a self probe with Id %d: %v\n",
						debugging.Prefix,
						probeCount,
						err,
					)
					return
				}

				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"(%s) About to report results for round %d of probes!\n",
						debugging.Prefix,
						probeCount,
					)
				}

				dataPointsLock.Lock()
				// Now we have our four data points (three in the foreign probe data point and one in the self probe data point)
				if dataPoints != nil {
					measurement := ResponsivenessProbeResult{
						Foreign: foreignProbeDataPoint, Self: selfProbeDataPoint,
					}

					dataPoints <- series.SeriesMessage[ResponsivenessProbeResult, uint]{
						Type: series.SeriesMessageMeasure, Bucket: probeCount,
						Measure: utilities.Some[ResponsivenessProbeResult](measurement),
					}
				}
				dataPointsLock.Unlock()
			}()
		}
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Probe driver is going to start waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		utilities.OrTimeout(func() { wg.Wait() }, 2*time.Second)
		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Probe driver is done waiting for its probes to finish.\n",
				debugging.Prefix,
			)
		}
		dataPointsLock.Lock()
		close(dataPoints)
		dataPoints = nil
		dataPointsLock.Unlock()
	}()
	return
}

func LoadGenerator(
	throughputCtx context.Context, // Stop our activity when we no longer need any throughput
	networkActivityCtx context.Context, // Create all network connections in this context.
	generateLoadCtx context.Context, // Stop adding additional throughput when we are stable.
	rampupInterval time.Duration,
	lgcGenerator func() lgc.LoadGeneratingConnection, // Use this to generate a new load-generating connection.
	loadGeneratingConnectionsCollection *lgc.LoadGeneratingConnectionCollection,
	mnp int,
	captureExtendedStats bool, // do we want to attempt to gather TCP information on these connections?
	debugging *debug.DebugWithPrefix, // How can we forget debugging?
) (seriesCommunicationChannel chan series.SeriesMessage[ThroughputDataPoint, uint64]) { // Send back all the instantaneous throughputs that we generate.
	seriesCommunicationChannel = make(chan series.SeriesMessage[ThroughputDataPoint, uint64])

	go func() {
		flowsCreated := uint64(0)

		flowsCreated += addFlows(
			networkActivityCtx,
			constants.StartingNumberOfLoadGeneratingConnections,
			loadGeneratingConnectionsCollection,
			lgcGenerator,
			debugging.Level,
		)

		nextSampleStartTime := time.Now().Add(rampupInterval)

		for currentInterval := uint64(0); true; currentInterval++ {

			// If the throughputCtx is canceled, then that means our work here is done ...
			if throughputCtx.Err() != nil {
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

			// Waiting is the hardest part -- that was a long time asleep
			// and we may have been cancelled during that time!
			if throughputCtx.Err() != nil {
				break
			}

			// Compute "instantaneous aggregate" goodput which is the number of
			// bytes transferred within the last second.
			var instantaneousThroughputTotal float64 = 0
			var instantaneousThroughputDataPoints uint = 0
			granularThroughputDatapoints := make([]GranularThroughputDataPoint, 0)
			now = time.Now() // Used to align granular throughput data
			allInvalid := true
			for i := range *loadGeneratingConnectionsCollection.LGCs {
				loadGeneratingConnectionsCollection.Lock.Lock()
				connectionState := (*loadGeneratingConnectionsCollection.LGCs)[i].Status()
				loadGeneratingConnectionsCollection.Lock.Unlock()
				switch connectionState {
				default:
					{
						error := fmt.Sprintf(
							"%v: Load-generating connection with id %d is in an unrecognizable state.\n",
							debugging,
							(*loadGeneratingConnectionsCollection.LGCs)[i].ClientId())
						fmt.Fprintf(os.Stderr, "%s", error)
						panic(error)
					}
				case lgc.LGC_STATUS_ERROR,
					lgc.LGC_STATUS_DONE:
					{
						if debug.IsDebug(debugging.Level) {
							fmt.Printf(
								"%v: Load-generating connection with id %d is invalid or complete ... skipping.\n",
								debugging,
								(*loadGeneratingConnectionsCollection.LGCs)[i].ClientId(),
							)
						}
						// TODO: Do we add null connection to throughput? and how do we define it? Throughput -1 or 0?
						granularThroughputDatapoints = append(
							granularThroughputDatapoints,
							GranularThroughputDataPoint{now, 0, uint32(i), 0, 0, ""},
						)
					}
				case lgc.LGC_STATUS_NOT_STARTED:
					{
						if debug.IsDebug(debugging.Level) {
							fmt.Printf(
								"%v: Load-generating connection with id %d has not finished starting; "+
									"it will not contribute throughput during this interval.\n",
								debugging,
								(*loadGeneratingConnectionsCollection.LGCs)[i].ClientId())
						}
					}
				case lgc.LGC_STATUS_RUNNING:
					{
						allInvalid = false
						currentTransferred, currentInterval :=
							(*loadGeneratingConnectionsCollection.LGCs)[i].TransferredInInterval()
						// normalize to a second-long interval!
						instantaneousConnectionThroughput := float64(
							currentTransferred,
						) / float64(
							currentInterval.Seconds(),
						)
						instantaneousThroughputTotal += instantaneousConnectionThroughput
						instantaneousThroughputDataPoints++

						tcpRtt := time.Duration(0 * time.Second)
						tcpCwnd := uint32(0)
						if captureExtendedStats && extendedstats.ExtendedStatsAvailable() {
							if stats := (*loadGeneratingConnectionsCollection.LGCs)[i].Stats(); stats != nil {
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
				}
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
				instantaneousThroughputTotal,
				int(instantaneousThroughputDataPoints),
				len(*loadGeneratingConnectionsCollection.LGCs),
				granularThroughputDatapoints,
			}

			seriesCommunicationChannel <- series.SeriesMessage[ThroughputDataPoint, uint64]{
				Type: series.SeriesMessageReserve, Bucket: currentInterval,
			}
			seriesCommunicationChannel <- series.SeriesMessage[ThroughputDataPoint, uint64]{
				Type: series.SeriesMessageMeasure, Bucket: currentInterval,
				Measure: utilities.Some[ThroughputDataPoint](throughputDataPoint),
			}

			if generateLoadCtx.Err() != nil {
				// No need to add additional data points because the controller told us
				// that we were stable. But, we want to continue taking measurements!
				if debug.IsDebug(debugging.Level) {
					fmt.Printf(
						"%v: Throughput is stable; not adding any additional load-generating connections.\n",
						debugging,
					)
				}
				continue
			}

			loadGeneratingConnectionsCollection.Lock.Lock()
			currentParallelConnectionCount, err :=
				loadGeneratingConnectionsCollection.Len()
			loadGeneratingConnectionsCollection.Lock.Unlock()

			if err != nil {
				if debug.IsWarn(debugging.Level) {
					fmt.Printf(
						"%v: Failed to get a count of the number of parallel load-generating connections: %v.\n",
						debugging,
						err,
					)
				}
			}
			if currentParallelConnectionCount < mnp {
				// Just add another constants.AdditiveNumberOfLoadGeneratingConnections flows -- that's our only job now!
				flowsCreated += addFlows(
					networkActivityCtx,
					constants.AdditiveNumberOfLoadGeneratingConnections,
					loadGeneratingConnectionsCollection,
					lgcGenerator,
					debugging.Level,
				)
			} else if debug.IsWarn(debugging.Level) {
				fmt.Printf(
					"%v: Maximum number of parallel transport-layer connections reached (%d). Not adding another.\n",
					debugging,
					mnp,
				)
			}
		}

		if debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) Stopping a load generator after creating %d flows.\n",
				debugging.Prefix, flowsCreated)
		}
	}()
	return
}
