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
 * with Foobar. If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/network-quality/goresponsiveness/ccw"
	"github.com/network-quality/goresponsiveness/config"
	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/timeoutat"
	"github.com/network-quality/goresponsiveness/utilities"
)

var (
	// Variables to hold CLI arguments.
	configHost = flag.String(
		"config",
		constants.DefaultConfigHost,
		"name/IP of responsiveness configuration server.",
	)
	configPort = flag.Int(
		"port",
		constants.DefaultPortNumber,
		"port number on which to access responsiveness configuration server.",
	)
	configPath = flag.String(
		"path",
		"config",
		"path on the server to the configuration endpoint.",
	)
	debugCliFlag = flag.Bool(
		"debug",
		constants.DefaultDebug,
		"Enable debugging.",
	)
	strictFlag = flag.Bool(
		"strict",
		constants.DefaultStrict,
		"Whether to run the test in strict mode (measure HTTP get time on load-generating connection)",
	)
	sattimeout = flag.Int(
		"sattimeout",
		constants.DefaultTestTime,
		"Maximum time to spend measuring saturation.",
	)
	rpmtimeout = flag.Int(
		"rpmtimeout",
		constants.RPMCalculationTime,
		"Maximum time to spend calculating RPM.",
	)
	sslKeyFileName = flag.String(
		"ssl-key-file",
		"",
		"Store the per-session SSL key files in this file.",
	)
	profile = flag.String(
		"profile",
		"",
		"Enable client runtime profiling and specify storage location. Disabled by default.",
	)

	calculateExtendedStats = flag.Bool(
		"extended-stats",
		false,
		"Enable the collection and display of extended statistics -- may not be available on certain platforms.",
	)
)

func main() {
	flag.Parse()

	timeoutDuration := time.Second * time.Duration(*sattimeout)
	timeoutAbsoluteTime := time.Now().Add(timeoutDuration)
	configHostPort := fmt.Sprintf("%s:%d", *configHost, *configPort)
	operatingCtx, cancelOperatingCtx := context.WithCancel(context.Background())
	lgDataCollectionCtx, cancelLGDataCollectionCtx := context.WithCancel(
		context.Background(),
	)
	newConnectionProberCtx, newConnectionProberCtxCancel := context.WithCancel(
		context.Background(),
	)
	config := &config.Config{}
	var debugLevel debug.DebugLevel = debug.Error

	if *debugCliFlag {
		debugLevel = debug.Debug
	}

	if *calculateExtendedStats && !extendedstats.ExtendedStatsAvailable() {
		*calculateExtendedStats = false
		fmt.Printf(
			"Warning: Calculation of extended statistics was requested but they are not supported on this platform.\n",
		)
	}

	if err := config.Get(configHostPort, *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}
	if err := config.IsValid(); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error: Invalid configuration returned from %s: %v\n",
			config.Source,
			err,
		)
		return
	}
	if debug.IsDebug(debugLevel) {
		fmt.Printf("Configuration: %s\n", config)
	}

	timeoutChannel := timeoutat.TimeoutAt(
		operatingCtx,
		timeoutAbsoluteTime,
		debugLevel,
	)
	if debug.IsDebug(debugLevel) {
		fmt.Printf("Test will end earlier than %v\n", timeoutAbsoluteTime)
	}

	// print the banner
	dt := time.Now().UTC()
	fmt.Printf(
		"%s UTC Go Responsiveness to %s...\n",
		dt.Format("01-02-2006 15:04:05"),
		configHostPort,
	)

	if len(*profile) != 0 {
		f, err := os.Create(*profile)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Error: Profiling requested with storage in %s but that file could not be opened: %v\n",
				*profile,
				err,
			)
			return
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var sslKeyFileConcurrentWriter *ccw.ConcurrentWriter = nil
	if *sslKeyFileName != "" {
		if sslKeyFileHandle, err := os.OpenFile(*sslKeyFileName, os.O_RDWR|os.O_CREATE, os.FileMode(0600)); err != nil {
			fmt.Printf("Could not open the keyfile for writing: %v!\n", err)
			sslKeyFileConcurrentWriter = nil
		} else {
			if err = utilities.SeekForAppend(sslKeyFileHandle); err != nil {
				fmt.Printf("Could not seek to the end of the key file: %v!\n", err)
				sslKeyFileConcurrentWriter = nil
			} else {
				if debug.IsDebug(debugLevel) {
					fmt.Printf("Doing SSL key logging through file %v\n", *sslKeyFileName)
				}
				sslKeyFileConcurrentWriter = ccw.NewConcurrentFileWriter(sslKeyFileHandle)
				defer sslKeyFileHandle.Close()
			}
		}
	}

	/*
	 * Create (and then, ironically, name) two anonymous functions that, when invoked,
	 * will create load-generating connections for upload/download/
	 */
	generate_lgd := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionDownload{
			Path:      config.Urls.LargeUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}
	generate_lgu := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionUpload{
			Path:      config.Urls.UploadUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}

	generate_lg_probe_configuration := func() rpm.ProbeConfiguration {
		return rpm.ProbeConfiguration{URL: config.Urls.SmallUrl, Interval: 100 * time.Millisecond}
	}

	generate_nc_probe_configuration := func() rpm.ProbeConfiguration {
		return rpm.ProbeConfiguration{URL: config.Urls.SmallUrl, Interval: 100 * time.Millisecond}
	}

	var downloadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "download")
	var uploadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "upload")
	var newConnectionDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "new connection probe")

	// TODO: Separate contexts for load generation and data collection. If we do that, if either of the two
	// data collection go routines stops well before the other, they will continue to send probes and we can
	// generate additional information!

	downloadDataCollectionChannel := rpm.LGCollectData(
		lgDataCollectionCtx,
		operatingCtx,
		generate_lgd,
		generate_lg_probe_configuration,
		downloadDebugging,
	)
	uploadDataCollectionChannel := rpm.LGCollectData(
		lgDataCollectionCtx,
		operatingCtx,
		generate_lgu,
		generate_lg_probe_configuration,
		uploadDebugging,
	)

	newConnectionProbeDataPoints := rpm.Prober(
		newConnectionProberCtx,
		generate_nc_probe_configuration,
		sslKeyFileConcurrentWriter,
		newConnectionDebugging,
	)

	dataCollectionTimeout := false
	uploadDataCollectionComplete := false
	downloadDataCollectionComple := false
	downloadDataCollectionResult := rpm.LGDataCollectionResult{}
	uploadDataCollectionResult := rpm.LGDataCollectionResult{}

	for !(uploadDataCollectionComplete && downloadDataCollectionComple) {
		select {
		case downloadDataCollectionResult = <-downloadDataCollectionChannel:
			{
				downloadDataCollectionComple = true
				if *debugCliFlag {
					fmt.Printf(
						"################# download load-generating data collection is %s complete (%fMBps, %d flows)!\n",
						utilities.Conditional(
							dataCollectionTimeout,
							"(provisionally)",
							"",
						),
						utilities.ToMBps(downloadDataCollectionResult.RateBps),
						len(downloadDataCollectionResult.LGCs),
					)
				}
			}
		case uploadDataCollectionResult = <-uploadDataCollectionChannel:
			{
				uploadDataCollectionComplete = true
				if *debugCliFlag {
					fmt.Printf(
						"################# upload load-generating data collection is %s complete (%fMBps, %d flows)!\n",
						utilities.Conditional(
							dataCollectionTimeout,
							"(provisionally)",
							"",
						),
						utilities.ToMBps(uploadDataCollectionResult.RateBps),
						len(uploadDataCollectionResult.LGCs),
					)
				}
			}
		case <-timeoutChannel:
			{
				if dataCollectionTimeout {
					// We already timedout on data collection. This signal means that
					// we are timedout on getting the provisional data collection. We
					// will exit!
					fmt.Fprint(
						os.Stderr,
						"Error: Load-Generating data collection could not be completed in time and no provisional data could be gathered. Test failed.\n",
					)
					cancelOperatingCtx()
					if *debugCliFlag {
						time.Sleep(constants.CooldownPeriod)
					}
					return
				}
				dataCollectionTimeout = true

				// We timed out attempting to collect data about the link. So, we will
				// shut down all the collection xfers
				cancelLGDataCollectionCtx()
				// and then we will give ourselves some additional time in order
				// to complete provisional data collection.
				timeoutAbsoluteTime = time.Now().
					Add(time.Second * time.Duration(*rpmtimeout))
				timeoutChannel = timeoutat.TimeoutAt(
					operatingCtx,
					timeoutAbsoluteTime,
					debugLevel,
				)
				if *debugCliFlag {
					fmt.Printf(
						"################# timeout collecting load-generating data!\n",
					)
				}
			}
		}
	}

	// Shutdown the new-connection prober!
	newConnectionProberCtxCancel()

	// In the new version we are no longer going to wait to send probes until after
	// saturation. When we get here we are now only going to compute the results
	// and/or extended statistics!

	extendedStats := extendedstats.ExtendedStats{}

	for i := 0; i < len(downloadDataCollectionResult.LGCs); i++ {
		// Assume that extended statistics are available -- the check was done explicitly at
		// program startup if the calculateExtendedStats flag was set by the user on the command line.
		if *calculateExtendedStats {
			if !extendedstats.ExtendedStatsAvailable() {
				panic("Extended stats are not available but the user requested their calculation.")
			}
			if err := extendedStats.IncorporateConnectionStats(downloadDataCollectionResult.LGCs[i].Stats().ConnInfo.Conn); err != nil {
				fmt.Fprintf(
					os.Stderr,
					"Warning: Could not add extended stats for the connection: %v",
					err,
				)
			}
		}
	}
	fmt.Printf(
		"Download: %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(downloadDataCollectionResult.RateBps),
		utilities.ToMBps(downloadDataCollectionResult.RateBps),
		len(downloadDataCollectionResult.LGCs),
	)
	fmt.Printf(
		"Upload:   %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(uploadDataCollectionResult.RateBps),
		utilities.ToMBps(uploadDataCollectionResult.RateBps),
		len(uploadDataCollectionResult.LGCs),
	)

	totalNewConnectionRoundTripTime := float64(0)
	totalNewConnectionRoundTrips := uint64(0)
	for ncDp := range newConnectionProbeDataPoints {
		totalNewConnectionRoundTripTime += ncDp.Duration.Seconds()
		totalNewConnectionRoundTrips += uint64(ncDp.RoundTripCount)
	}
	averageNewConnectionRoundTripTime := totalNewConnectionRoundTripTime / float64(
		totalNewConnectionRoundTrips,
	)
	newConnectionRpm := (1.0 / averageNewConnectionRoundTripTime) * 60.0
	if *debugCliFlag {
		fmt.Printf(
			"Total New-Connection Round Trips: %d, Total New-Connection Round Trip Time: %f, Average New-Connection Round Trip Time (in seconds): %f\n",
			totalNewConnectionRoundTrips,
			totalNewConnectionRoundTripTime,
			averageNewConnectionRoundTripTime,
		)
		fmt.Printf("(New-Connection) RPM: %f\n", newConnectionRpm)
	}

	totalLoadGeneratingRoundTripTime := float64(0)
	totalLoadGeneratingRoundTrips := uint64(0)
	for _, dp := range downloadDataCollectionResult.DataPoints {
		totalLoadGeneratingRoundTripTime += dp.Duration.Seconds()
		totalLoadGeneratingRoundTrips += uint64(dp.RoundTripCount)
	}
	for _, dp := range uploadDataCollectionResult.DataPoints {
		totalLoadGeneratingRoundTripTime += dp.Duration.Seconds()
		totalLoadGeneratingRoundTrips += uint64(dp.RoundTripCount)
	}
	averageLoadGeneratingRoundTripTime := totalLoadGeneratingRoundTripTime / float64(
		totalLoadGeneratingRoundTrips,
	)
	loadGeneratingRPM := (1.0 / averageLoadGeneratingRoundTripTime) * 60.0
	if *debugCliFlag {
		fmt.Printf(
			"Total Load-Generating Round Trips: %d, Total New-Connection Round Trip Time: %f, Average New-Connection Round Trip Time (in seconds): %f\n",
			totalLoadGeneratingRoundTrips,
			totalLoadGeneratingRoundTripTime,
			averageLoadGeneratingRoundTripTime,
		)
		fmt.Printf("(Load-Generating) RPM: %f\n", loadGeneratingRPM)
	}

	rpm := (newConnectionRpm + loadGeneratingRPM) / 2.0
	fmt.Printf("RPM: %5.0f\n", rpm)

	if *calculateExtendedStats {
		fmt.Println(extendedStats.Repr())
	}

	cancelOperatingCtx()
	if *debugCliFlag {
		fmt.Printf("In debugging mode, we will cool down.\n")
		time.Sleep(constants.CooldownPeriod)
		fmt.Printf("Done cooling down.\n")
	}
}
