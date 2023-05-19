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
	"github.com/network-quality/goresponsiveness/utilities"
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
		newGenerator := lgcGenerator()
		lgcc.Append(newGenerator)
		// Second, try to start the connection.
		if !newGenerator.Start(ctx, debug) {
			// If there was an error, we'll make sure that the caller knows it.
			fmt.Printf(
				"Error starting lgc with id %d!\n", newGenerator.ClientId(),
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

func CombinedProber(
	proberCtx context.Context,
	networkActivityCtx context.Context,
	foreignProbeConfigurationGenerator func() probe.ProbeConfiguration,
	selfProbeConfigurationGenerator func() probe.ProbeConfiguration,
	selfDownProbeConnection lgc.LoadGeneratingConnection,
	selfUpProbeConnection lgc.LoadGeneratingConnection,
	probeInterval time.Duration,
	keyLogger io.Writer,
	captureExtendedStats bool,
	debugging *debug.DebugWithPrefix,
) (dataPoints chan probe.ProbeDataPoint) {
	// Make a channel to send back all the generated data points
	// when we are probing.
	dataPoints = make(chan probe.ProbeDataPoint)

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
			transport := &http.Transport{}
			transport.TLSClientConfig = &tls.Config{}
			transport.Proxy = http.ProxyFromEnvironment

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

			transport.TLSClientConfig.InsecureSkipVerify =
				foreignProbeConfiguration.InsecureSkipVerify

			utilities.OverrideHostTransport(transport,
				foreignProbeConfiguration.ConnectToAddr)

			foreignProbeClient := &http.Client{Transport: transport}

			// Start Foreign Connection Prober
			probeCount++
			go probe.Probe(
				networkActivityCtx,
				&wg,
				foreignProbeClient,
				foreignProbeConfiguration.URL,
				foreignProbeConfiguration.Host,
				probe.Foreign,
				&dataPoints,
				captureExtendedStats,
				debugging,
			)

			// Start Self Download Connection Prober

			// TODO: Make the following sanity check more than just a check.
			// We only want to start a SelfDown probe on a connection that is
			// in the RUNNING state.
			if selfDownProbeConnection.Status() == lgc.LGC_STATUS_RUNNING {
				go probe.Probe(
					networkActivityCtx,
					&wg,
					selfDownProbeConnection.Client(),
					selfProbeConfiguration.URL,
					selfProbeConfiguration.Host,
					probe.SelfDown,
					&dataPoints,
					captureExtendedStats,
					debugging,
				)
			} else {
				panic(fmt.Sprintf("(%s) Combined probe driver evidently lost its underlying connection (Status: %v).\n",
					debugging.Prefix, selfDownProbeConnection.Status()))
			}

			// Start Self Upload Connection Prober

			// TODO: Make the following sanity check more than just a check.
			// We only want to start a SelfDown probe on a connection that is
			// in the RUNNING state.
			if selfUpProbeConnection.Status() == lgc.LGC_STATUS_RUNNING {
				go probe.Probe(
					proberCtx,
					&wg,
					selfUpProbeConnection.Client(),
					selfProbeConfiguration.URL,
					selfProbeConfiguration.Host,
					probe.SelfUp,
					&dataPoints,
					captureExtendedStats,
					debugging,
				)
			} else {
				panic(fmt.Sprintf("(%s) Combined probe driver evidently lost its underlying connection (Status: %v).\n",
					debugging.Prefix, selfUpProbeConnection.Status()))
			}
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
	loadGeneratingConnectionsCollection *lgc.LoadGeneratingConnectionCollection,
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
			loadGeneratingConnectionsCollection,
			lgcGenerator,
			debugging.Level,
		)

		// We have at least a single load-generating channel. This channel will be the one that
		// the self probes use.
		go func() {
			loadGeneratingConnectionsCollection.Lock.Lock()
			zerothConnection, err := loadGeneratingConnectionsCollection.Get(0)
			loadGeneratingConnectionsCollection.Lock.Unlock()
			if err != nil {
				panic("Could not get the zeroth connection!\n")
			}
			// We are going to wait until it is started.
			if !(*zerothConnection).WaitUntilStarted(loadGeneratorCtx) {
				fmt.Fprintf(os.Stderr, "Could not wait until the zeroth load-generating connection was started!\n")
				return
			}
			// Now that it is started, we will send it back to the caller so that
			// they can pass it on to the CombinedProber which will use it for the
			// self probes.
			probeConnectionCommunicationChannel <- *zerothConnection
		}()

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
			throughputCalculations <- throughputDataPoint

			// Just add another constants.AdditiveNumberOfLoadGeneratingConnections flows -- that's our only job now!
			flowsCreated += addFlows(
				networkActivityCtx,
				constants.AdditiveNumberOfLoadGeneratingConnections,
				loadGeneratingConnectionsCollection,
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
