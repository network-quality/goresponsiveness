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
	"github.com/network-quality/goresponsiveness/datalogger"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/ms"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/stabilizer"
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
	dataLoggerBaseFileName = flag.String(
		"logger-filename",
		"",
		"Store granular information about tests results in files with this basename. Time and information type will be appended (before the first .) to create separate log files. Disabled by default.",
	)
)

func main() {
	flag.Parse()

	timeoutDuration := time.Second * time.Duration(*sattimeout)
	timeoutAbsoluteTime := time.Now().Add(timeoutDuration)
	configHostPort := fmt.Sprintf("%s:%d", *configHost, *configPort)

	// This is the overall operating context of the program. All other
	// contexts descend from this one. Canceling this one cancels all
	// the others.
	operatingCtx, operatingCtxCancel := context.WithCancel(context.Background())

	// This context is used to control the load generators -- we cancel it when
	// the system has completed its work. (i.e, rpm and saturation are stable).
	// The *operator* contexts control stopping the goroutines that are running
	// the process; the *throughput* contexts control whether the load generators
	// continue to add new connections at every interval.
	uploadLoadGeneratorOperatorCtx, uploadLoadGeneratorOperatorCtxCancel := context.WithCancel(operatingCtx)
	downloadLoadGeneratorOperatorCtx, downloadLoadGeneratorOperatorCtxCancel := context.WithCancel(operatingCtx)

	// This context is used to control the load-generating network activity (i.e., it controls all
	// the connections that are open to do load generation and probing). Cancelling this context will close
	// all the network connections that are responsible for generating the load.
	lgNetworkActivityCtx, lgNetworkActivityCtxCancel := context.WithCancel(operatingCtx)

	// This context is used to control the activity of the prober.
	proberCtx, proberCtxCancel := context.WithCancel(operatingCtx)

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

	if err := config.Get(configHostPort, *configPath, sslKeyFileConcurrentWriter); err != nil {
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
	var selfProbeDataLogger datalogger.DataLogger[rpm.ProbeDataPoint] = nil
	var foreignProbeDataLogger datalogger.DataLogger[rpm.ProbeDataPoint] = nil
	var downloadThroughputDataLogger datalogger.DataLogger[rpm.ThroughputDataPoint] = nil
	var uploadThroughputDataLogger datalogger.DataLogger[rpm.ThroughputDataPoint] = nil
	var granularThroughputDataLogger datalogger.DataLogger[rpm.GranularThroughputDataPoint] = nil

	// User wants to log data
	if *dataLoggerBaseFileName != "" {
		var err error = nil
		unique := time.Now().UTC().Format("01-02-2006-15-04-05")

		dataLoggerSelfFilename := utilities.FilenameAppend(*dataLoggerBaseFileName, "-self-"+unique)
		dataLoggerForeignFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-foreign-"+unique,
		)
		dataLoggerDownloadThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-download-"+unique,
		)
		dataLoggerUploadThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-upload-"+unique,
		)
		dataLoggerGranularThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-granular-"+unique,
		)

		selfProbeDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ProbeDataPoint](
			dataLoggerSelfFilename,
		)
		if err != nil {
			fmt.Printf(
				"Warning: Could not create the file for storing self probe results (%s). Disabling functionality.\n",
				dataLoggerSelfFilename,
			)
			selfProbeDataLogger = nil
		}

		foreignProbeDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ProbeDataPoint](
			dataLoggerForeignFilename,
		)
		if err != nil {
			fmt.Printf(
				"Warning: Could not create the file for storing foreign probe results (%s). Disabling functionality.\n",
				dataLoggerForeignFilename,
			)
			foreignProbeDataLogger = nil
		}

		downloadThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ThroughputDataPoint](
			dataLoggerDownloadThroughputFilename,
		)
		if err != nil {
			fmt.Printf(
				"Warning: Could not create the file for storing download throughput results (%s). Disabling functionality.\n",
				dataLoggerDownloadThroughputFilename,
			)
			downloadThroughputDataLogger = nil
		}

		uploadThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ThroughputDataPoint](
			dataLoggerUploadThroughputFilename,
		)
		if err != nil {
			fmt.Printf(
				"Warning: Could not create the file for storing upload throughput results (%s). Disabling functionality.\n",
				dataLoggerUploadThroughputFilename,
			)
			uploadThroughputDataLogger = nil
		}

		granularThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.GranularThroughputDataPoint](
			dataLoggerGranularThroughputFilename,
		)
		if err != nil {
			fmt.Printf(
				"Warning: Could not create the file for storing granular throughput results (%s). Disabling functionality.\n",
				dataLoggerGranularThroughputFilename,
			)
			granularThroughputDataLogger = nil
		}
	}
	// If, for some reason, the data loggers are nil, make them Null Data Loggers so that we don't have conditional
	// code later.
	if selfProbeDataLogger == nil {
		selfProbeDataLogger = datalogger.CreateNullDataLogger[rpm.ProbeDataPoint]()
	}
	if foreignProbeDataLogger == nil {
		foreignProbeDataLogger = datalogger.CreateNullDataLogger[rpm.ProbeDataPoint]()
	}
	if downloadThroughputDataLogger == nil {
		downloadThroughputDataLogger = datalogger.CreateNullDataLogger[rpm.ThroughputDataPoint]()
	}
	if uploadThroughputDataLogger == nil {
		uploadThroughputDataLogger = datalogger.CreateNullDataLogger[rpm.ThroughputDataPoint]()
	}
	if granularThroughputDataLogger == nil {
		granularThroughputDataLogger = datalogger.CreateNullDataLogger[rpm.GranularThroughputDataPoint]()
	}

	/*
	 * Create (and then, ironically, name) two anonymous functions that, when invoked,
	 * will create load-generating connections for upload/download
	 */
	generate_lgd := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionDownload{
			Path:      config.Urls.LargeUrl,
			Host:      config.Urls.LargeUrlHost,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}
	generate_lgu := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionUpload{
			Path:      config.Urls.UploadUrl,
			Host:      config.Urls.UploadUrlHost,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}

	generateSelfProbeConfiguration := func() rpm.ProbeConfiguration {
		return rpm.ProbeConfiguration{
			URL:  config.Urls.SmallUrl,
			Host: config.Urls.SmallUrlHost,
		}
	}

	generateForeignProbeConfiguration := func() rpm.ProbeConfiguration {
		return rpm.ProbeConfiguration{
			URL:  config.Urls.SmallUrl,
			Host: config.Urls.SmallUrlHost,
		}
	}

	var downloadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "download")
	var uploadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "upload")
	var combinedProbeDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "combined probe")

	downloadLoadGeneratingConnectionCollection := lgc.NewLoadGeneratingConnectionCollection()
	uploadLoadGeneratingConnectionCollection := lgc.NewLoadGeneratingConnectionCollection()

	// TODO: Separate contexts for load generation and data collection. If we do that, if either of the two
	// data collection go routines stops well before the other, they will continue to send probes and we can
	// generate additional information!

	selfDownProbeConnectionCommunicationChannel, downloadThroughputChannel := rpm.LoadGenerator(
		lgNetworkActivityCtx,
		downloadLoadGeneratorOperatorCtx,
		time.Second,
		generate_lgd,
		&downloadLoadGeneratingConnectionCollection,
		downloadDebugging,
	)
	selfUpProbeConnectionCommunicationChannel, uploadThroughputChannel := rpm.LoadGenerator(
		lgNetworkActivityCtx,
		uploadLoadGeneratorOperatorCtx,
		time.Second,
		generate_lgu,
		&uploadLoadGeneratingConnectionCollection,
		uploadDebugging,
	)

	selfDownProbeConnection := <-selfDownProbeConnectionCommunicationChannel
	selfUpProbeConnection := <-selfUpProbeConnectionCommunicationChannel

	probeDataPointsChannel := rpm.CombinedProber(
		proberCtx,
		generateForeignProbeConfiguration,
		generateSelfProbeConfiguration,
		selfDownProbeConnection,
		selfUpProbeConnection,
		time.Millisecond*100,
		sslKeyFileConcurrentWriter,
		combinedProbeDebugging,
	)

	responsivenessIsStable := false
	downloadThroughputIsStable := false
	uploadThroughputIsStable := false

	// Test parameters:
	// 1. I: The number of previous instantaneous measurements to consider when generating
	//       the so-called instantaneous moving averages.
	// 2. K: The number of instantaneous moving averages to consider when determining stability.
	// 3: S: The standard deviation cutoff used to determine stability among the K preceding
	//       moving averages of a measurement.

	throughputI := constants.InstantaneousThroughputMeasurementCount
	probeI := constants.InstantaneousProbeMeasurementCount
	K := constants.InstantaneousMovingAverageStabilityCount
	S := constants.StabilityStandardDeviation

	downloadThroughputStabilizerDebugConfig := debug.NewDebugWithPrefix(debug.Debug, "Download Throughput Stabilizer")
	downloadThroughputStabilizerDebugLevel := debug.Error
	if *debugCliFlag {
		downloadThroughputStabilizerDebugLevel = debug.Debug
	}
	downloadThroughputStabilizer := stabilizer.NewThroughputStabilizer(throughputI, K, S, downloadThroughputStabilizerDebugLevel, downloadThroughputStabilizerDebugConfig)

	uploadThroughputStabilizerDebugConfig := debug.NewDebugWithPrefix(debug.Debug, "Upload Throughput Stabilizer")
	uploadThroughputStabilizerDebugLevel := debug.Error
	if *debugCliFlag {
		uploadThroughputStabilizerDebugLevel = debug.Debug
	}
	uploadThroughputStabilizer := stabilizer.NewThroughputStabilizer(throughputI, K, S, uploadThroughputStabilizerDebugLevel, uploadThroughputStabilizerDebugConfig)

	probeStabilizerDebugConfig := debug.NewDebugWithPrefix(debug.Debug, "Probe Stabilizer")
	probeStabilizerDebugLevel := debug.Error
	if *debugCliFlag {
		probeStabilizerDebugLevel = debug.Debug
	}
	probeStabilizer := stabilizer.NewProbeStabilizer(probeI, K, S, probeStabilizerDebugLevel, probeStabilizerDebugConfig)

	selfRtts := ms.NewInfiniteMathematicalSeries[float64]()
	foreignRtts := ms.NewInfiniteMathematicalSeries[float64]()

	// For later debugging output, record the last throughputs on load-generating connectings
	// and the number of open connections.
	lastUploadThroughputRate := float64(0)
	lastUploadThroughputOpenConnectionCount := int(0)
	lastDownloadThroughputRate := float64(0)
	lastDownloadThroughputOpenConnectionCount := int(0)

	// Every time that there is a new measurement, the possibility exists that the measurements become unstable.
	// This allows us to continue pushing until *everything* is stable at the same time.
timeout:
	for !(responsivenessIsStable && downloadThroughputIsStable && uploadThroughputIsStable) {
		select {

		case downloadThroughputMeasurement := <-downloadThroughputChannel:
			{
				downloadThroughputStabilizer.AddMeasurement(downloadThroughputMeasurement)
				downloadThroughputIsStable = downloadThroughputStabilizer.IsStable()
				if *debugCliFlag {
					fmt.Printf(
						"################# Download is instantaneously %s.\n", utilities.Conditional(downloadThroughputIsStable, "stable", "unstable"))
				}
				downloadThroughputDataLogger.LogRecord(downloadThroughputMeasurement)
				for i := range downloadThroughputMeasurement.GranularThroughputDataPoints {
					datapoint := downloadThroughputMeasurement.GranularThroughputDataPoints[i]
					datapoint.Direction = "Download"
					granularThroughputDataLogger.LogRecord(datapoint)
				}

				lastDownloadThroughputRate = downloadThroughputMeasurement.Throughput
				lastDownloadThroughputOpenConnectionCount = downloadThroughputMeasurement.Connections
			}

		case uploadThroughputMeasurement := <-uploadThroughputChannel:
			{
				uploadThroughputStabilizer.AddMeasurement(uploadThroughputMeasurement)
				uploadThroughputIsStable = uploadThroughputStabilizer.IsStable()
				if *debugCliFlag {
					fmt.Printf(
						"################# Upload is instantaneously %s.\n", utilities.Conditional(uploadThroughputIsStable, "stable", "unstable"))
				}
				uploadThroughputDataLogger.LogRecord(uploadThroughputMeasurement)
				for i := range uploadThroughputMeasurement.GranularThroughputDataPoints {
					datapoint := uploadThroughputMeasurement.GranularThroughputDataPoints[i]
					datapoint.Direction = "Upload"
					granularThroughputDataLogger.LogRecord(datapoint)
				}

				lastUploadThroughputRate = uploadThroughputMeasurement.Throughput
				lastUploadThroughputOpenConnectionCount = uploadThroughputMeasurement.Connections
			}
		case probeMeasurement := <-probeDataPointsChannel:
			{
				probeStabilizer.AddMeasurement(probeMeasurement)

				// Check stabilization immediately -- this could change if we wait. Not sure if the immediacy
				// is *actually* important, but it can't hurt?
				responsivenessIsStable = probeStabilizer.IsStable()

				if *debugCliFlag {
					fmt.Printf(
						"################# Responsiveness is instantaneously %s.\n", utilities.Conditional(responsivenessIsStable, "stable", "unstable"))
				}
				if probeMeasurement.Type == rpm.Foreign {
					for range utilities.Iota(0, int(probeMeasurement.RoundTripCount)) {
						foreignRtts.AddElement(probeMeasurement.Duration.Seconds() / float64(probeMeasurement.RoundTripCount))

					}
				} else if probeMeasurement.Type == rpm.SelfDown || probeMeasurement.Type == rpm.SelfUp {
					selfRtts.AddElement(probeMeasurement.Duration.Seconds())
				}

				// There may be more than one round trip accumulated together. If that is the case,
				// we will blow them apart in to three separate measurements and each one will just
				// be 1 / measurement.RoundTripCount of the total length.

				if probeMeasurement.Type == rpm.Foreign {
					foreignProbeDataLogger.LogRecord(probeMeasurement)
				} else if probeMeasurement.Type == rpm.SelfDown || probeMeasurement.Type == rpm.SelfUp {
					selfProbeDataLogger.LogRecord(probeMeasurement)
				}
			}
		case <-timeoutChannel:
			{
				break timeout
			}
		}
	}

	// TODO: Reset timeout to RPM timeout stat?

	// Did the test run to stability?
	testRanToStability := (downloadThroughputIsStable && uploadThroughputIsStable && responsivenessIsStable)

	if *debugCliFlag {
		fmt.Printf("Stopping all the load generating data generators (stability: %s).\n", utilities.Conditional(testRanToStability, "success", "failure"))
	}

	/* At this point there are
	1. Load generators running
	-- uploadLoadGeneratorOperatorCtx
	-- downloadLoadGeneratorOperatorCtx
	2. Network connections opened by those load generators:
	-- lgNetworkActivityCtx
	3. Probes
	-- proberCtx
	*/

	// First, stop the load generators and the probes
	proberCtxCancel()
	downloadLoadGeneratorOperatorCtxCancel()
	uploadLoadGeneratorOperatorCtxCancel()

	// Second, calculate the extended stats (if the user requested)

	extendedStats := extendedstats.AggregateExtendedStats{}
	if *calculateExtendedStats {
		if extendedstats.ExtendedStatsAvailable() {
			downloadLoadGeneratingConnectionCollection.Lock.Lock()
			for i := 0; i < len(*downloadLoadGeneratingConnectionCollection.LGCs); i++ {
				// Assume that extended statistics are available -- the check was done explicitly at
				// program startup if the calculateExtendedStats flag was set by the user on the command line.
				if err := extendedStats.IncorporateConnectionStats((*downloadLoadGeneratingConnectionCollection.LGCs)[i].Stats().ConnInfo.Conn); err != nil {
					fmt.Fprintf(
						os.Stderr,
						"Warning: Could not add extended stats for the connection: %v\n",
						err,
					)
				}
			}
			downloadLoadGeneratingConnectionCollection.Lock.Unlock()

			// We do not trace upload connections!
		} else {
			// TODO: Should we just log here?
			panic("Extended stats are not available but the user requested their calculation.")
		}
	}

	// Third, stop the network connections opened by the load generators.
	lgNetworkActivityCtxCancel()

	// Finally, stop the world.
	operatingCtxCancel()

	// Calculate the RPM

	selfProbeRoundTripTimeP90 := selfRtts.Percentile(90)
	// The specification indicates that we want to calculate the foreign probes as such:
	// 1/3*tcp_foreign + 1/3*tls_foreign + 1/3*http_foreign
	// where tcp_foreign, tls_foreign, http_foreign are the P90 RTTs for the connection
	// of the tcp, tls and http connections, respectively. However, we cannot break out
	// the individual RTTs so we assume that they are roughly equal. The good news is that
	// we already did that roughly-equal split up when we added them to the foreignRtts IMS.
	foreignProbeRoundTripTimeP90 := foreignRtts.Percentile(90)

	// This is 60 because we measure in seconds not ms
	rpm := 60.0 / (float64(selfProbeRoundTripTimeP90+foreignProbeRoundTripTimeP90) / 2.0)

	if *debugCliFlag {
		fmt.Printf(
			"Total Load-Generating Round Trips: %d, Total New-Connection Round Trips: %d, P90 LG RTT: %f, P90 NC RTT: %f\n",
			selfRtts.Size(),
			foreignRtts.Size(),
			selfProbeRoundTripTimeP90,
			foreignProbeRoundTripTimeP90,
		)
	}

	if !testRanToStability {
		fmt.Printf("Test did not run to stability, these results are estimates:\n")
	}
	fmt.Printf("RPM: %5.0f\n", rpm)

	fmt.Printf(
		"Download: %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(lastDownloadThroughputRate),
		utilities.ToMBps(lastDownloadThroughputRate),
		lastDownloadThroughputOpenConnectionCount,
	)
	fmt.Printf(
		"Upload:   %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(lastUploadThroughputRate),
		utilities.ToMBps(lastUploadThroughputRate),
		lastUploadThroughputOpenConnectionCount,
	)

	if *calculateExtendedStats {
		fmt.Println(extendedStats.Repr())
	}

	selfProbeDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the self data logger.\n")
	}
	selfProbeDataLogger.Close()

	foreignProbeDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the foreign data logger.\n")
	}
	foreignProbeDataLogger.Close()

	downloadThroughputDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the download throughput data logger.\n")
	}
	downloadThroughputDataLogger.Close()

	uploadThroughputDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the upload throughput data logger.\n")
	}
	uploadThroughputDataLogger.Close()

	granularThroughputDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the granular throughput data logger.\n")
	}
	granularThroughputDataLogger.Close()

	if *debugCliFlag {
		fmt.Printf("In debugging mode, we will cool down.\n")
		time.Sleep(constants.CooldownPeriod)
		fmt.Printf("Done cooling down.\n")
	}

}
