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
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/network-quality/goresponsiveness/ccw"
	"github.com/network-quality/goresponsiveness/config"
	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/datalogger"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/direction"
	"github.com/network-quality/goresponsiveness/executor"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/probe"
	"github.com/network-quality/goresponsiveness/qualityattenuation"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/series"
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
	configURL = flag.String(
		"url",
		"",
		"configuration URL (takes precedence over other configuration parts)",
	)
	debugCliFlag = flag.Bool(
		"debug",
		constants.DefaultDebug,
		"Enable debugging.",
	)
	detailedCliFlag = flag.Bool(
		"detailed",
		constants.DefaultDebug,
		"Enable detailed result output.",
	)
	rpmtimeout = flag.Int(
		"rpm.timeout",
		constants.DefaultTestTime,
		"Maximum time (in seconds) to spend calculating RPM (i.e., total test time.).",
	)
	rpmmad = flag.Int(
		"rpm.mad",
		constants.SpecParameterCliOptionsDefaults.Mad,
		"Moving average distance -- number of intervals considered during stability calculations.",
	)
	rpmid = flag.Int(
		"rpm.id",
		constants.SpecParameterCliOptionsDefaults.Id,
		"Duration of the interval between re-evaluating the network conditions (in seconds).",
	)
	rpmtmp = flag.Uint(
		"rpm.tmp",
		constants.SpecParameterCliOptionsDefaults.Tmp,
		"Percent of measurements to trim when calculating statistics about network conditions (between 0 and 100).",
	)
	rpmsdt = flag.Float64(
		"rpm.sdt",
		constants.SpecParameterCliOptionsDefaults.Sdt,
		"Cutoff in the standard deviation of measured values about network conditions between unstable and stable.",
	)
	rpmmnp = flag.Int(
		"rpm.mnp",
		constants.SpecParameterCliOptionsDefaults.Mnp,
		"Maximimum number of parallel connections to establish when attempting to reach working conditions.",
	)
	rpmmps = flag.Int(
		"rpm.mps",
		constants.SpecParameterCliOptionsDefaults.Mps,
		"Maximimum number of probes to send per second.",
	)
	rpmptc = flag.Float64(
		"rpm.ptc",
		constants.SpecParameterCliOptionsDefaults.Ptc,
		"Percentage of the (discovered) total network capacity that probes are allowed to consume.",
	)
	rpmp = flag.Int(
		"rpm.p",
		constants.SpecParameterCliOptionsDefaults.P,
		"Percentile of results to consider when calculating responsiveness.",
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
	printQualityAttenuation = flag.Bool(
		"quality-attenuation",
		false,
		"Print quality attenuation information.",
	)
	dataLoggerBaseFileName = flag.String(
		"logger-filename",
		"",
		"Store granular information about tests results in files with this basename. Time and information type will be appended (before the first .) to create separate log files. Disabled by default.",
	)
	connectToAddr = flag.String(
		"connect-to",
		"",
		"address (hostname or IP) to connect to (overriding DNS). Disabled by default.",
	)
	insecureSkipVerify = flag.Bool(
		"insecure-skip-verify",
		constants.DefaultInsecureSkipVerify,
		"Enable server certificate validation.",
	)
	prometheusStatsFilename = flag.String(
		"prometheus-stats-filename",
		"",
		"If filename specified, prometheus stats will be written. If specified file exists, it will be overwritten.",
	)
	showVersion = flag.Bool(
		"version",
		false,
		"Show version.",
	)
	calculateRelativeRpm = flag.Bool(
		"relative-rpm",
		false,
		"Calculate a relative RPM.",
	)
	withL4S          = flag.Bool("with-l4s", false, "Use L4S (with default TCP prague congestion control algorithm.)")
	withL4SAlgorithm = flag.String("with-l4s-algorithm", "", "Use L4S (with specified congestion control algorithm.)")

	parallelTestExecutionPolicy = constants.DefaultTestExecutionPolicy
)

func main() {
	// Add one final command-line argument
	flag.BoolFunc("rpm.parallel", "Parallel test execution policy.", func(value string) error {
		if value != "true" {
			return fmt.Errorf("-parallel can only be used to enable parallel test execution policy")
		}
		parallelTestExecutionPolicy = executor.Parallel
		return nil
	})

	flag.Parse()

	if *showVersion {
		fmt.Fprintf(os.Stdout, "goresponsiveness %s\n", utilities.GitVersion)
		os.Exit(0)
	}

	var debugLevel debug.DebugLevel = debug.Error

	if *debugCliFlag {
		debugLevel = debug.Debug
	}

	specParameters, err := rpm.SpecParametersFromArguments(*rpmtimeout, *rpmmad, *rpmid,
		*rpmtmp, *rpmsdt, *rpmmnp, *rpmmps, *rpmptc, *rpmp, parallelTestExecutionPolicy)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error: There was an error configuring the test with user-supplied parameters: %v\n",
			err,
		)
		os.Exit(1)
	}

	if debug.IsDebug(debugLevel) {
		fmt.Printf("Running the test according to the following spec parameters:\n%v\n", specParameters.ToString())
	}

	var configHostPort string

	// if user specified a full URL, use that and set the various parts we need out of it
	if len(*configURL) > 0 {
		parsedURL, err := url.ParseRequestURI(*configURL)
		if err != nil {
			fmt.Printf("Error: Could not parse %q: %s", *configURL, err)
			os.Exit(1)
		}

		*configHost = parsedURL.Hostname()
		*configPath = parsedURL.Path
		// We don't explicitly care about configuring the *configPort.
		configHostPort = parsedURL.Host // host or host:port
	} else {
		configHostPort = fmt.Sprintf("%s:%d", *configHost, *configPort)
	}

	// This is the overall operating context of the program. All other
	// contexts descend from this one. Canceling this one cancels all
	// the others.
	operatingCtx, operatingCtxCancel := context.WithCancel(context.Background())

	config := &config.Config{
		ConnectToAddr: *connectToAddr,
	}

	if *calculateExtendedStats && !extendedstats.ExtendedStatsAvailable() {
		*calculateExtendedStats = false
		fmt.Fprintf(
			os.Stderr,
			"Warning: Calculation of extended statistics was requested but is not supported on this platform.\n",
		)
	}

	var sslKeyFileConcurrentWriter *ccw.ConcurrentWriter = nil
	if *sslKeyFileName != "" {
		if sslKeyFileHandle, err := os.OpenFile(*sslKeyFileName, os.O_RDWR|os.O_CREATE, os.FileMode(0o600)); err != nil {
			fmt.Printf("Could not open the requested SSL key logging file for writing: %v!\n", err)
			sslKeyFileConcurrentWriter = nil
		} else {
			if err = utilities.SeekForAppend(sslKeyFileHandle); err != nil {
				fmt.Printf("Could not seek to the end of the SSL key logging file: %v!\n", err)
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

	var congestionControlChosen *string = nil
	if *withL4S || *withL4SAlgorithm != "" {
		congestionControlChosen = &constants.DefaultL4SCongestionControlAlgorithm
		if *withL4SAlgorithm != "" {
			congestionControlChosen = withL4SAlgorithm
		}
	}

	if congestionControlChosen != nil && debug.IsDebug(debugLevel) {
		fmt.Printf("Doing congestion control with the %v algorithm.\n", *congestionControlChosen)
	}

	if err := config.Get(configHostPort, *configPath, *insecureSkipVerify,
		sslKeyFileConcurrentWriter); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	if err := config.IsValid(); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Error: Invalid configuration returned from %s: %v\n",
			config.Source,
			err,
		)
		os.Exit(1)
	}
	if debug.IsDebug(debugLevel) {
		fmt.Printf("Configuration: %s\n", config)
	}

	downloadDirection := direction.Direction{}
	uploadDirection := direction.Direction{}

	// User wants to log data
	if *dataLoggerBaseFileName != "" {
		var err error = nil
		unique := time.Now().UTC().Format("01-02-2006-15-04-05")

		dataLoggerDownloadThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-download-"+unique,
		)
		dataLoggerUploadThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-upload-"+unique,
		)

		dataLoggerDownloadGranularThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-download-granular-"+unique,
		)

		dataLoggerUploadGranularThroughputFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-throughput-upload-granular-"+unique,
		)

		dataLoggerSelfFilename := utilities.FilenameAppend(*dataLoggerBaseFileName, "-self-"+unique)
		dataLoggerForeignFilename := utilities.FilenameAppend(
			*dataLoggerBaseFileName,
			"-foreign-"+unique,
		)

		selfProbeDataLogger, err := datalogger.CreateCSVDataLogger[probe.ProbeDataPoint](
			dataLoggerSelfFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing self probe results (%s). Disabling functionality.\n",
				dataLoggerSelfFilename,
			)
			selfProbeDataLogger = nil
		}
		uploadDirection.SelfProbeDataLogger = selfProbeDataLogger
		downloadDirection.SelfProbeDataLogger = selfProbeDataLogger

		foreignProbeDataLogger, err := datalogger.CreateCSVDataLogger[probe.ProbeDataPoint](
			dataLoggerForeignFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing foreign probe results (%s). Disabling functionality.\n",
				dataLoggerForeignFilename,
			)
			foreignProbeDataLogger = nil
		}
		uploadDirection.ForeignProbeDataLogger = foreignProbeDataLogger
		downloadDirection.ForeignProbeDataLogger = foreignProbeDataLogger

		downloadDirection.ThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ThroughputDataPoint](
			dataLoggerDownloadThroughputFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing download throughput results (%s). Disabling functionality.\n",
				dataLoggerDownloadThroughputFilename,
			)
			downloadDirection.ThroughputDataLogger = nil
		}
		uploadDirection.ThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.ThroughputDataPoint](
			dataLoggerUploadThroughputFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing upload throughput results (%s). Disabling functionality.\n",
				dataLoggerUploadThroughputFilename,
			)
			uploadDirection.ThroughputDataLogger = nil
		}

		downloadDirection.GranularThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.GranularThroughputDataPoint](
			dataLoggerDownloadGranularThroughputFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing download granular throughput results (%s). Disabling functionality.\n",
				dataLoggerDownloadGranularThroughputFilename,
			)
			downloadDirection.GranularThroughputDataLogger = nil
		}
		uploadDirection.GranularThroughputDataLogger, err = datalogger.CreateCSVDataLogger[rpm.GranularThroughputDataPoint](
			dataLoggerUploadGranularThroughputFilename,
		)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Warning: Could not create the file for storing upload granular throughput results (%s). Disabling functionality.\n",
				dataLoggerUploadGranularThroughputFilename,
			)
			uploadDirection.GranularThroughputDataLogger = nil
		}

	}
	// If, for some reason, the data loggers are nil, make them Null Data Loggers so that we don't have conditional
	// code later.
	if downloadDirection.SelfProbeDataLogger == nil {
		downloadDirection.SelfProbeDataLogger = datalogger.CreateNullDataLogger[probe.ProbeDataPoint]()
	}
	if uploadDirection.SelfProbeDataLogger == nil {
		uploadDirection.SelfProbeDataLogger = datalogger.CreateNullDataLogger[probe.ProbeDataPoint]()
	}

	if downloadDirection.ForeignProbeDataLogger == nil {
		downloadDirection.ForeignProbeDataLogger = datalogger.CreateNullDataLogger[probe.ProbeDataPoint]()
	}
	if uploadDirection.ForeignProbeDataLogger == nil {
		uploadDirection.ForeignProbeDataLogger = datalogger.CreateNullDataLogger[probe.ProbeDataPoint]()
	}

	if downloadDirection.ThroughputDataLogger == nil {
		downloadDirection.ThroughputDataLogger = datalogger.CreateNullDataLogger[rpm.ThroughputDataPoint]()
	}
	if uploadDirection.ThroughputDataLogger == nil {
		uploadDirection.ThroughputDataLogger = datalogger.CreateNullDataLogger[rpm.ThroughputDataPoint]()
	}

	if downloadDirection.GranularThroughputDataLogger == nil {
		downloadDirection.GranularThroughputDataLogger =
			datalogger.CreateNullDataLogger[rpm.GranularThroughputDataPoint]()
	}
	if uploadDirection.GranularThroughputDataLogger == nil {
		uploadDirection.GranularThroughputDataLogger =
			datalogger.CreateNullDataLogger[rpm.GranularThroughputDataPoint]()
	}

	/*
	 * Create (and then, ironically, name) two anonymous functions that, when invoked,
	 * will create load-generating connections for upload/download
	 */
	downloadDirection.CreateLgdc = func() lgc.LoadGeneratingConnection {
		lgd := lgc.NewLoadGeneratingConnectionDownload(config.Urls.LargeUrl,
			sslKeyFileConcurrentWriter, config.ConnectToAddr, *insecureSkipVerify, congestionControlChosen)
		return &lgd
	}
	uploadDirection.CreateLgdc = func() lgc.LoadGeneratingConnection {
		lgu := lgc.NewLoadGeneratingConnectionUpload(config.Urls.UploadUrl,
			sslKeyFileConcurrentWriter, config.ConnectToAddr, *insecureSkipVerify, congestionControlChosen)
		return &lgu
	}

	downloadDirection.DirectionDebugging = debug.NewDebugWithPrefix(debugLevel, "download")
	downloadDirection.ProbeDebugging = debug.NewDebugWithPrefix(debugLevel, "download probe")

	uploadDirection.DirectionDebugging = debug.NewDebugWithPrefix(debugLevel, "upload")
	uploadDirection.ProbeDebugging = debug.NewDebugWithPrefix(debugLevel, "upload probe")

	downloadDirection.Lgcc = lgc.NewLoadGeneratingConnectionCollection()
	uploadDirection.Lgcc = lgc.NewLoadGeneratingConnectionCollection()

	uploadDirection.ExtendedStatsEligible = true
	downloadDirection.ExtendedStatsEligible = true

	generateSelfProbeConfiguration := func() probe.ProbeConfiguration {
		return probe.ProbeConfiguration{
			URL:                config.Urls.SmallUrl,
			ConnectToAddr:      config.ConnectToAddr,
			InsecureSkipVerify: *insecureSkipVerify,
			CongestionControl:  congestionControlChosen,
		}
	}

	generateForeignProbeConfiguration := func() probe.ProbeConfiguration {
		return probe.ProbeConfiguration{
			URL:                config.Urls.SmallUrl,
			ConnectToAddr:      config.ConnectToAddr,
			InsecureSkipVerify: *insecureSkipVerify,
			CongestionControl:  congestionControlChosen,
		}
	}

	downloadDirection.DirectionLabel = "Download"
	uploadDirection.DirectionLabel = "Upload"

	directions := []*direction.Direction{&downloadDirection, &uploadDirection}

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
				"Error: Profiling requested but could not open the log file ( %s ) for writing: %v\n",
				*profile,
				err,
			)
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	globalNumericBucketGenerator := series.NewNumericBucketGenerator[uint64](0)

	var baselineRpm *rpm.Rpm[float64] = nil

	if *calculateRelativeRpm {
		baselineForeignDownloadRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)
		baselineFauxSelfDownloadRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)
		baselineStableResponsiveness := false
		baselineProbeDebugging := debug.NewDebugWithPrefix(debugLevel, "Baseline RPM Calculation Probe")

		timeoutDuration := specParameters.TestTimeout
		timeoutAbsoluteTime := time.Now().Add(timeoutDuration)

		timeoutChannel := timeoutat.TimeoutAt(
			operatingCtx,
			timeoutAbsoluteTime,
			debugLevel,
		)
		if debug.IsDebug(debugLevel) {
			fmt.Printf("Baseline RPM calculation will end no later than %v\n", timeoutAbsoluteTime)
		}

		baselineProberOperatorCtx, baselineProberOperatorCtxCancel := context.WithCancel(operatingCtx)

		// This context is used to control the network activity (i.e., it controls all
		// the connections that are open to do load generation and probing).
		baselineNetworkActivityCtx, baselineNetworkActivityCtxCancel := context.WithCancel(operatingCtx)

		baselineResponsivenessStabilizerDebugConfig :=
			debug.NewDebugWithPrefix(debug.Debug, "Baseline Responsiveness Stabilizer")
		baselineResponsivenessStabilizerDebugLevel := debug.Error
		if *debugCliFlag {
			baselineResponsivenessStabilizerDebugLevel = debug.Debug
		}
		baselineResponsivenessStabilizer := stabilizer.NewStabilizer[int64, uint64](
			specParameters.MovingAvgDist, specParameters.StdDevTolerance,
			specParameters.TrimmedMeanPct, "milliseconds",
			baselineResponsivenessStabilizerDebugLevel,
			baselineResponsivenessStabilizerDebugConfig)

		baselineStabilityCheckTime := time.Now().Add(specParameters.EvalInterval)
		baselineStabilityCheckTimeChannel := timeoutat.TimeoutAt(
			operatingCtx,
			baselineStabilityCheckTime,
			debugLevel,
		)

		responsivenessStabilizationCommunicationChannel := rpm.ResponsivenessProber(
			baselineProberOperatorCtx,
			baselineNetworkActivityCtx,
			generateForeignProbeConfiguration,
			generateSelfProbeConfiguration,
			nil,
			&globalNumericBucketGenerator,
			lgc.LGC_DOWN,
			specParameters.ProbeInterval,
			sslKeyFileConcurrentWriter,
			*calculateExtendedStats,
			baselineProbeDebugging,
		)

		lowerBucketBound, upperBucketBound := uint64(0), uint64(0)
	baseline_responsiveness_timeout:
		for !baselineStableResponsiveness {
			select {
			case probeMeasurement := <-responsivenessStabilizationCommunicationChannel:
				{
					switch probeMeasurement.Type {
					case series.SeriesMessageReserve:
						{
							bucket := probeMeasurement.Bucket
							if *debugCliFlag {
								fmt.Printf("baseline: Reserving a responsiveness bucket with id %v.\n", bucket)
							}
							baselineResponsivenessStabilizer.Reserve(bucket)
							baselineForeignDownloadRtts.Reserve(bucket)
							baselineFauxSelfDownloadRtts.Reserve(bucket)
						}
					case series.SeriesMessageMeasure:
						{
							bucket := probeMeasurement.Bucket
							measurement := utilities.GetSome(probeMeasurement.Measure)
							foreignDataPoint := measurement.Foreign

							if *debugCliFlag {
								fmt.Printf(
									"baseline: Filling a responsiveness bucket with id %v with value %v.\n", bucket, measurement)
							}
							baselineResponsivenessStabilizer.AddMeasurement(
								bucket, foreignDataPoint.Duration.Milliseconds())

							if err := baselineForeignDownloadRtts.Fill(
								bucket, foreignDataPoint.Duration.Seconds()); err != nil {
								fmt.Printf("Attempting to fill a bucket (id: %d) that does not exist (baselineForeignDownloadRtts)\n", bucket)
							}
							if err := baselineFauxSelfDownloadRtts.Fill(
								bucket, foreignDataPoint.Duration.Seconds()/3.0); err != nil {
								fmt.Printf("Attempting to fill a bucket (id: %d) that does not exist (baselineFauxSelfDownloadRtts)\n", bucket)
							}
						}
					}
				}
			case <-timeoutChannel:
				{
					break baseline_responsiveness_timeout
				}
			case <-baselineStabilityCheckTimeChannel:
				{
					if *debugCliFlag {
						fmt.Printf("baseline responsiveness stability interval is complete.\n")
					}

					baselineStabilityCheckTime = time.Now().Add(specParameters.EvalInterval)
					baselineStabilityCheckTimeChannel = timeoutat.TimeoutAt(
						operatingCtx,
						baselineStabilityCheckTime,
						debugLevel,
					)

					// Check stabilization immediately -- this could change if we wait. Not sure if the immediacy
					// is *actually* important, but it can't hurt?
					baselineStableResponsiveness = baselineResponsivenessStabilizer.IsStable()

					if *debugCliFlag {
						fmt.Printf(
							"baseline responsiveness is instantaneously %s.\n",
							utilities.Conditional(baselineStableResponsiveness, "stable", "unstable"))
					}

					// Do not tick an interval if we are stable. Doing so would expel one of the
					// intervals that we need for our RPM calculations!
					if !baselineStableResponsiveness {
						baselineResponsivenessStabilizer.Interval()
					}
				}
			}
		}
		baselineNetworkActivityCtxCancel()
		baselineProberOperatorCtxCancel()

		lowerBucketBound, upperBucketBound = baselineResponsivenessStabilizer.GetBounds()

		if *debugCliFlag {
			fmt.Printf("Baseline responsiveness stablizer bucket bounds: (%v, %v)\n", lowerBucketBound, upperBucketBound)
		}

		for _, label := range []string{"Unbounded ", ""} {
			baselineRpm = rpm.CalculateRpm(baselineFauxSelfDownloadRtts,
				baselineForeignDownloadRtts, specParameters.TrimmedMeanPct, specParameters.Percentile)

			fmt.Printf("%vBaseline RPM: %5.0f (P%d)\n", label, baselineRpm.PNRpm, specParameters.Percentile)
			fmt.Printf("%vBaseline RPM: %5.0f (Single-Sided %v%% Trimmed Mean)\n",
				label, baselineRpm.MeanRpm, specParameters.TrimmedMeanPct)

			baselineFauxSelfDownloadRtts.SetTrimmingBucketBounds(lowerBucketBound, upperBucketBound)
			baselineForeignDownloadRtts.SetTrimmingBucketBounds(lowerBucketBound, upperBucketBound)
		}
	}

	var selfRttsQualityAttenuation *qualityattenuation.SimpleQualityAttenuation = nil
	if *printQualityAttenuation {
		selfRttsQualityAttenuation = qualityattenuation.NewSimpleQualityAttenuation()
	}

	directionExecutionUnits := make([]executor.ExecutionUnit, 0)

	for _, direction := range directions {
		// Make a copy here to make sure that we do not get go-wierdness in our closure (see https://github.com/golang/go/discussions/56010).
		direction := direction

		directionExecutionUnit := func() {
			timeoutDuration := specParameters.TestTimeout
			timeoutAbsoluteTime := time.Now().Add(timeoutDuration)

			timeoutChannel := timeoutat.TimeoutAt(
				operatingCtx,
				timeoutAbsoluteTime,
				debugLevel,
			)
			if debug.IsDebug(debugLevel) {
				fmt.Printf("%s Test will end no later than %v\n",
					direction.DirectionLabel, timeoutAbsoluteTime)
			}

			throughputOperatorCtx, throughputOperatorCtxCancel := context.WithCancel(operatingCtx)
			proberOperatorCtx, proberOperatorCtxCancel := context.WithCancel(operatingCtx)

			// This context is used to control the network activity (i.e., it controls all
			// the connections that are open to do load generation and probing). Cancelling this context will close
			// all the network connections that are responsible for generating the load.
			probeNetworkActivityCtx, probeNetworkActivityCtxCancel := context.WithCancel(operatingCtx)
			throughputCtx, throughputCtxCancel := context.WithCancel(operatingCtx)
			direction.ThroughputActivityCtx, direction.ThroughputActivityCtxCancel = &throughputCtx, &throughputCtxCancel

			lgStabilizationCommunicationChannel := rpm.LoadGenerator(
				throughputOperatorCtx,
				*direction.ThroughputActivityCtx,
				specParameters.EvalInterval,
				direction.CreateLgdc,
				&direction.Lgcc,
				&globalNumericBucketGenerator,
				specParameters.MaxParallelConns,
				specParameters.EvalInterval,
				*calculateExtendedStats,
				direction.DirectionDebugging,
			)

			throughputStabilizerDebugConfig := debug.NewDebugWithPrefix(debug.Debug,
				fmt.Sprintf("%v Throughput Stabilizer", direction.DirectionLabel))
			downloadThroughputStabilizerDebugLevel := debug.Error
			if *debugCliFlag {
				downloadThroughputStabilizerDebugLevel = debug.Debug
			}
			throughputStabilizer := stabilizer.NewStabilizer[float64, uint64](
				specParameters.MovingAvgDist, specParameters.StdDevTolerance, 0, "bytes",
				downloadThroughputStabilizerDebugLevel, throughputStabilizerDebugConfig)

			responsivenessStabilizerDebugConfig := debug.NewDebugWithPrefix(debug.Debug,
				fmt.Sprintf("%v Responsiveness Stabilizer", direction.DirectionLabel))
			responsivenessStabilizerDebugLevel := debug.Error
			if *debugCliFlag {
				responsivenessStabilizerDebugLevel = debug.Debug
			}
			responsivenessStabilizer := stabilizer.NewStabilizer[int64, uint64](
				specParameters.MovingAvgDist, specParameters.StdDevTolerance,
				specParameters.TrimmedMeanPct, "milliseconds",
				responsivenessStabilizerDebugLevel, responsivenessStabilizerDebugConfig)

			// For later debugging output, record the last throughputs on load-generating connectings
			// and the number of open connections.
			lastThroughputRate := float64(0)
			lastThroughputOpenConnectionCount := int(0)

			stabilityCheckTime := time.Now().Add(specParameters.EvalInterval)
			stabilityCheckTimeChannel := timeoutat.TimeoutAt(
				operatingCtx,
				stabilityCheckTime,
				debugLevel,
			)

		lg_timeout:
			for !direction.StableThroughput {
				select {
				case throughputMeasurement := <-lgStabilizationCommunicationChannel:
					{
						switch throughputMeasurement.Type {
						case series.SeriesMessageReserve:
							{
								throughputStabilizer.Reserve(throughputMeasurement.Bucket)
								if *debugCliFlag {
									fmt.Printf(
										"%s: Reserving a throughput bucket with id %v.\n",
										direction.DirectionLabel, throughputMeasurement.Bucket)
								}
							}
						case series.SeriesMessageMeasure:
							{
								bucket := throughputMeasurement.Bucket
								measurement := utilities.GetSome(throughputMeasurement.Measure)

								throughputStabilizer.AddMeasurement(bucket, measurement.Throughput)

								direction.ThroughputDataLogger.LogRecord(measurement)
								for _, v := range measurement.GranularThroughputDataPoints {
									v.Direction = "Download"
									direction.GranularThroughputDataLogger.LogRecord(v)
								}

								lastThroughputRate = measurement.Throughput
								lastThroughputOpenConnectionCount = measurement.Connections
							}
						}
					}
				case <-stabilityCheckTimeChannel:
					{
						if *debugCliFlag {
							fmt.Printf(
								"%v throughput stability interval is complete.\n", direction.DirectionLabel)
						}
						stabilityCheckTime = time.Now().Add(specParameters.EvalInterval)
						stabilityCheckTimeChannel = timeoutat.TimeoutAt(
							operatingCtx,
							stabilityCheckTime,
							debugLevel,
						)

						direction.StableThroughput = throughputStabilizer.IsStable()
						if *debugCliFlag {
							fmt.Printf(
								"%v is instantaneously %s.\n", direction.DirectionLabel,
								utilities.Conditional(direction.StableThroughput, "stable", "unstable"))
						}

						throughputStabilizer.Interval()
					}
				case <-timeoutChannel:
					{
						break lg_timeout
					}
				}
			}

			if direction.StableThroughput {
				if *debugCliFlag {
					fmt.Printf("Throughput is stable; beginning responsiveness testing.\n")
				}
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Throughput stability could not be reached. Making the test 15 seconds longer to calculate speculative RPM results.\n")
				speculativeTimeoutDuration := time.Second * 15
				speculativeAbsoluteTimeoutTime := time.Now().Add(speculativeTimeoutDuration)
				timeoutChannel = timeoutat.TimeoutAt(
					operatingCtx,
					speculativeAbsoluteTimeoutTime,
					debugLevel,
				)
			}

			direction.SelfRtts = series.NewWindowSeries[float64, uint64](series.Forever, 0)
			direction.ForeignRtts = series.NewWindowSeries[float64, uint64](series.Forever, 0)

			responsivenessStabilizationCommunicationChannel := rpm.ResponsivenessProber(
				proberOperatorCtx,
				probeNetworkActivityCtx,
				generateForeignProbeConfiguration,
				generateSelfProbeConfiguration,
				&direction.Lgcc,
				&globalNumericBucketGenerator,
				direction.CreateLgdc().Direction(), // TODO: This could be better!
				specParameters.ProbeInterval,
				sslKeyFileConcurrentWriter,
				*calculateExtendedStats,
				direction.ProbeDebugging,
			)

		responsiveness_timeout:
			for !direction.StableResponsiveness {
				select {
				case probeMeasurement := <-responsivenessStabilizationCommunicationChannel:
					{
						switch probeMeasurement.Type {
						case series.SeriesMessageReserve:
							{
								bucket := probeMeasurement.Bucket
								if *debugCliFlag {
									fmt.Printf(
										"%s: Reserving a responsiveness bucket with id %v.\n", direction.DirectionLabel, bucket)
								}
								responsivenessStabilizer.Reserve(bucket)
								direction.ForeignRtts.Reserve(bucket)
								direction.SelfRtts.Reserve(bucket)
							}
						case series.SeriesMessageMeasure:
							{
								bucket := probeMeasurement.Bucket
								measurement := utilities.GetSome(probeMeasurement.Measure)
								foreignDataPoint := measurement.Foreign
								selfDataPoint := measurement.Self

								if *debugCliFlag {
									fmt.Printf(
										"%s: Filling a responsiveness bucket with id %v with value %v.\n",
										direction.DirectionLabel, bucket, measurement)
								}
								responsivenessStabilizer.AddMeasurement(bucket,
									(foreignDataPoint.Duration + selfDataPoint.Duration).Milliseconds())

								if err := direction.SelfRtts.Fill(bucket,
									selfDataPoint.Duration.Seconds()); err != nil {
									fmt.Printf("Attempting to fill a bucket (id: %d) that does not exist (perDirectionSelfRtts)\n", bucket)
								}

								if err := direction.ForeignRtts.Fill(bucket,
									foreignDataPoint.Duration.Seconds()); err != nil {
									fmt.Printf("Attempting to fill a bucket (id: %d) that does not exist (perDirectionForeignRtts)\n", bucket)
								}

								if selfRttsQualityAttenuation != nil {
									selfRttsQualityAttenuation.AddSample(selfDataPoint.Duration.Seconds())
								}

								direction.ForeignProbeDataLogger.LogRecord(*foreignDataPoint)
								direction.SelfProbeDataLogger.LogRecord(*selfDataPoint)

							}
						}
					}
				case throughputMeasurement := <-lgStabilizationCommunicationChannel:
					{
						switch throughputMeasurement.Type {
						case series.SeriesMessageReserve:
							{
								// We are no longer tracking stability, so reservation messages are useless!
								if *debugCliFlag {
									fmt.Printf(
										"%s: Discarding a throughput bucket with id %v when ascertaining responsiveness.\n",
										direction.DirectionLabel, throughputMeasurement.Bucket)
								}
							}
						case series.SeriesMessageMeasure:
							{
								measurement := utilities.GetSome(throughputMeasurement.Measure)

								if *debugCliFlag {
									fmt.Printf("Adding a throughput measurement (while ascertaining responsiveness).\n")
								}
								// There may be more than one round trip accumulated together. If that is the case,
								direction.ThroughputDataLogger.LogRecord(measurement)
								for _, v := range measurement.GranularThroughputDataPoints {
									v.Direction = direction.DirectionLabel
									direction.GranularThroughputDataLogger.LogRecord(v)
								}

								lastThroughputRate = measurement.Throughput
								lastThroughputOpenConnectionCount = measurement.Connections
							}
						}
					}
				case <-timeoutChannel:
					{
						if *debugCliFlag {
							fmt.Printf("%v responsiveness seeking interval has expired.\n", direction.DirectionLabel)
						}
						break responsiveness_timeout
					}
				case <-stabilityCheckTimeChannel:
					{
						if *debugCliFlag {
							fmt.Printf(
								"%v responsiveness stability interval is complete.\n", direction.DirectionLabel)
						}

						stabilityCheckTime = time.Now().Add(specParameters.EvalInterval)
						stabilityCheckTimeChannel = timeoutat.TimeoutAt(
							operatingCtx,
							stabilityCheckTime,
							debugLevel,
						)

						// Check stabilization immediately -- this could change if we wait. Not sure if the immediacy
						// is *actually* important, but it can't hurt?
						direction.StableResponsiveness = responsivenessStabilizer.IsStable()

						if *debugCliFlag {
							fmt.Printf(
								"%v responsiveness is instantaneously %s.\n", direction.DirectionLabel,
								utilities.Conditional(direction.StableResponsiveness, "stable", "unstable"))
						}

						// Do not tick an interval if we are stable. Doing so would expel one of the
						// intervals that we need for our RPM calculations!
						if !direction.StableResponsiveness {
							responsivenessStabilizer.Interval()
						}
					}
				}
			}

			// Did the test run to stability?
			testRanToStability := direction.StableThroughput && direction.StableResponsiveness

			if *debugCliFlag {
				fmt.Printf("Stopping all the load generating data generators (stability: %s).\n",
					utilities.Conditional(testRanToStability, "success", "failure"))
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

			// First, stop the load generator and the probe operators (but *not* the network activity)
			proberOperatorCtxCancel()
			throughputOperatorCtxCancel()

			// Second, calculate the extended stats (if the user requested and they are available for the direction)
			extendedStats := extendedstats.AggregateExtendedStats{}
			if *calculateExtendedStats && direction.ExtendedStatsEligible {
				if extendedstats.ExtendedStatsAvailable() {
					func() {
						// Put inside an IIFE so that we can use a defer!
						direction.Lgcc.Lock.Lock()
						defer direction.Lgcc.Lock.Unlock()

						lgcCount, err := direction.Lgcc.Len()
						if err != nil {
							fmt.Fprintf(
								os.Stderr,
								"Warning: Could not calculate the number of %v load-generating connections; aborting extended stats preparation.\n", direction.DirectionLabel,
							)
							return
						}

						for i := 0; i < lgcCount; i++ {
							// Assume that extended statistics are available -- the check was done explicitly at
							// program startup if the calculateExtendedStats flag was set by the user on the command line.
							currentLgc, _ := direction.Lgcc.Get(i)

							if currentLgc == nil || (*currentLgc).Stats() == nil {
								fmt.Fprintf(
									os.Stderr,
									"Warning: Could not add extended stats for the connection: The LGC was nil or there were no stats available.\n",
								)
								continue
							}
							if err := extendedStats.IncorporateConnectionStats(
								(*currentLgc).Stats().ConnInfo.Conn); err != nil {
								fmt.Fprintf(
									os.Stderr,
									"Warning: Could not add extended stats for the connection: %v.\n",
									err,
								)
							}
						}
					}()
				} else {
					// TODO: Should we just log here?
					panic("Extended stats are not available but the user requested their calculation.")
				}
			}

			// *Always* stop the probers! But, conditionally stop the througput.
			probeNetworkActivityCtxCancel()
			if parallelTestExecutionPolicy != executor.Parallel {
				if direction.ThroughputActivityCtxCancel == nil {
					panic(fmt.Sprintf("The cancellation function for the %v direction's throughput is nil!", direction.DirectionLabel))
				}
				(*direction.ThroughputActivityCtxCancel)()
			}

			direction.LowerBucketBound, direction.UpperBucketBound = responsivenessStabilizer.GetBounds()

			// Add a header to the results
			direction.FormattedResults += fmt.Sprintf("%v:\n", direction.DirectionLabel)

			if !testRanToStability {
				why := ""
				if !direction.StableThroughput {
					why += "throughput"
				}
				if !direction.StableResponsiveness {
					if len(why) != 0 {
						why += ", "
					}
					why += "responsiveness"
				}
				direction.FormattedResults += utilities.IndentOutput(
					fmt.Sprintf("Note: Test did not run to stability (%v), these results are estimates.\n", why), 1, "\t")
			}

			direction.FormattedResults += utilities.IndentOutput(fmt.Sprintf(
				"Throughput: %.3f Mbps (%.3f MBps), using %d parallel connections.\n",
				utilities.ToMbps(lastThroughputRate),
				utilities.ToMBps(lastThroughputRate),
				lastThroughputOpenConnectionCount,
			), 1, "\t")

			if *calculateExtendedStats {
				direction.FormattedResults += utilities.IndentOutput(
					fmt.Sprintf("%v", extendedStats.Repr()), 1, "\t")
			}

			var directionResult *rpm.Rpm[float64] = nil
			for _, label := range []string{"Unbounded ", ""} {
				directionResult = rpm.CalculateRpm(direction.SelfRtts, direction.ForeignRtts,
					specParameters.TrimmedMeanPct, specParameters.Percentile)
				if *debugCliFlag {
					direction.FormattedResults += utilities.IndentOutput(
						fmt.Sprintf("%vRPM Calculation Statistics:\n", label), 1, "\t")
					direction.FormattedResults += utilities.IndentOutput(directionResult.ToString(), 2, "\t")
				}

				direction.SelfRtts.SetTrimmingBucketBounds(
					direction.LowerBucketBound, direction.UpperBucketBound)
				direction.ForeignRtts.SetTrimmingBucketBounds(
					direction.LowerBucketBound, direction.UpperBucketBound)
			}

			if *debugCliFlag {
				direction.FormattedResults += utilities.IndentOutput(
					fmt.Sprintf("Bucket bounds: (%v, %v)\n",
						direction.LowerBucketBound, direction.UpperBucketBound), 1, "\t")
			}

			if *printQualityAttenuation {
				direction.FormattedResults += utilities.IndentOutput(
					"Quality Attenuation Statistics:\n", 1, "\t")
				direction.FormattedResults += utilities.IndentOutput(fmt.Sprintf(
					`	Number of losses:   %d
	Number of samples:  %d
	Min:                %.6fs
	Max:                %.6fs
	Mean:               %.6fs
	Variance:           %.6fs
	Standard Deviation: %.6fs
	PDV(90):            %.6fs
	PDV(99):            %.6fs
	P(90):              %.6fs
	P(99):              %.6fs
	RPM:                %.0f
	Gaming QoO:         %.0f
`, selfRttsQualityAttenuation.GetNumberOfLosses(),
					selfRttsQualityAttenuation.GetNumberOfSamples(),
					selfRttsQualityAttenuation.GetMinimum(),
					selfRttsQualityAttenuation.GetMaximum(),
					selfRttsQualityAttenuation.GetAverage(),
					selfRttsQualityAttenuation.GetVariance(),
					selfRttsQualityAttenuation.GetStandardDeviation(),
					selfRttsQualityAttenuation.GetPDV(90),
					selfRttsQualityAttenuation.GetPDV(99),
					selfRttsQualityAttenuation.GetPercentile(90),
					selfRttsQualityAttenuation.GetPercentile(99),
					selfRttsQualityAttenuation.GetRPM(),
					selfRttsQualityAttenuation.GetGamingQoO()), 1, "\t")
			}

			direction.FormattedResults += utilities.IndentOutput(fmt.Sprintf(
				"RPM: %.0f (P%d)\n", directionResult.PNRpm, specParameters.Percentile), 1, "\t")
			direction.FormattedResults += utilities.IndentOutput(fmt.Sprintf(
				"RPM: %.0f (Single-Sided %v%% Trimmed Mean)\n", directionResult.MeanRpm,
				specParameters.TrimmedMeanPct), 1, "\t")

			if len(*prometheusStatsFilename) > 0 {
				var testStable int
				if testRanToStability {
					testStable = 1
				}
				var buffer bytes.Buffer
				buffer.WriteString(fmt.Sprintf("networkquality_%v_test_stable %d\n",
					strings.ToLower(direction.DirectionLabel), testStable))
				buffer.WriteString(fmt.Sprintf("networkquality_%v_p90_rpm_value %d\n",
					strings.ToLower(direction.DirectionLabel), int64(directionResult.PNRpm)))
				buffer.WriteString(fmt.Sprintf("networkquality_%v_trimmed_rpm_value %d\n",
					strings.ToLower(direction.DirectionLabel),
					int64(directionResult.MeanRpm)))

				buffer.WriteString(fmt.Sprintf("networkquality_%v_bits_per_second %d\n",
					strings.ToLower(direction.DirectionLabel), int64(lastThroughputRate)))
				buffer.WriteString(fmt.Sprintf("networkquality_%v_connections %d\n",
					strings.ToLower(direction.DirectionLabel),
					int64(lastThroughputOpenConnectionCount)))

				if err := os.WriteFile(*prometheusStatsFilename, buffer.Bytes(), 0o644); err != nil {
					fmt.Printf("could not write %s: %s", *prometheusStatsFilename, err)
					os.Exit(1)
				}
			}

			direction.ThroughputDataLogger.Export()
			if *debugCliFlag {
				fmt.Printf("Closing the %v throughput data logger.\n", direction.DirectionLabel)
			}
			direction.ThroughputDataLogger.Close()

			direction.GranularThroughputDataLogger.Export()
			if *debugCliFlag {
				fmt.Printf("Closing the %v granular throughput data logger.\n", direction.DirectionLabel)
			}
			direction.GranularThroughputDataLogger.Close()

			if *debugCliFlag {
				fmt.Printf("In debugging mode, we will cool down after tests.\n")
				time.Sleep(constants.CooldownPeriod)
				fmt.Printf("Done cooling down.\n")
			}
		}
		directionExecutionUnits = append(directionExecutionUnits, directionExecutionUnit)
	} // End of direction testing.

	waiter := executor.Execute(parallelTestExecutionPolicy, directionExecutionUnits)
	waiter.Wait()

	// If we were testing in parallel mode, then the throughputs for each direction are still
	// running. We left them running in case one of the directions reached stability before the
	// other!
	if parallelTestExecutionPolicy == executor.Parallel {
		for _, direction := range directions {
			if *debugCliFlag {
				fmt.Printf("Stopping the throughput connections for the %v test.\n", direction.DirectionLabel)
			}
			if direction.ThroughputActivityCtxCancel == nil {
				panic(fmt.Sprintf("The cancellation function for the %v direction's throughput is nil!", direction.DirectionLabel))
			}
			if (*direction.ThroughputActivityCtx).Err() != nil {
				fmt.Fprintf(os.Stderr, "Warning: The throughput for the %v direction was already cancelled but should have been ongoing.\n", direction.DirectionLabel)
				continue
			}
			(*direction.ThroughputActivityCtxCancel)()
		}
	} else {
		for _, direction := range directions {
			if direction.ThroughputActivityCtxCancel == nil {
				panic(fmt.Sprintf("The cancellation function for the %v direction's throughput is nil!", direction.DirectionLabel))
			}
			if (*direction.ThroughputActivityCtx).Err() == nil {
				fmt.Fprintf(os.Stderr, "Warning: The throughput for the %v direction should have already been stopped but it was not.\n", direction.DirectionLabel)
			}
		}
	}

	fmt.Printf("Results:\n")
	fmt.Printf("========\n")
	// Print out the formatted results from each of the directions.
	for _, direction := range directions {
		fmt.Print(direction.FormattedResults)
		fmt.Printf("========\n")
	}

	if *debugCliFlag {
		unboundedAllSelfRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)
		unboundedAllForeignRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)

		unboundedAllSelfRtts.Append(&downloadDirection.SelfRtts)
		unboundedAllSelfRtts.Append(&uploadDirection.SelfRtts)
		unboundedAllForeignRtts.Append(&downloadDirection.ForeignRtts)
		unboundedAllForeignRtts.Append(&uploadDirection.ForeignRtts)

		result := rpm.CalculateRpm(unboundedAllSelfRtts, unboundedAllForeignRtts,
			specParameters.TrimmedMeanPct, specParameters.Percentile)

		fmt.Printf("Unbounded Final RPM Calculation stats:\n%v\n", result.ToString())

		fmt.Printf("Unbounded Final RPM: %.0f (P%d)\n", result.PNRpm, specParameters.Percentile)
		fmt.Printf("Unbounded Final RPM: %.0f (Single-Sided %v%% Trimmed Mean)\n",
			result.MeanRpm, specParameters.TrimmedMeanPct)
		fmt.Printf("\n")
	}

	boundedAllSelfRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)
	boundedAllForeignRtts := series.NewWindowSeries[float64, uint64](series.Forever, 0)

	// Now, if the test had a stable responsiveness measurement, then only consider the
	// probe measurements that are in the MAD intervals. On the other hand, if the test
	// did not stabilize, use all measurements to calculate the RPM.
	if downloadDirection.StableResponsiveness {
		boundedAllSelfRtts.BoundedAppend(&downloadDirection.SelfRtts)
		boundedAllForeignRtts.BoundedAppend(&downloadDirection.ForeignRtts)
	} else {
		boundedAllSelfRtts.Append(&downloadDirection.SelfRtts)
		boundedAllForeignRtts.Append(&downloadDirection.ForeignRtts)
	}
	if uploadDirection.StableResponsiveness {
		boundedAllSelfRtts.BoundedAppend(&uploadDirection.SelfRtts)
		boundedAllForeignRtts.BoundedAppend(&uploadDirection.ForeignRtts)
	} else {
		boundedAllSelfRtts.Append(&uploadDirection.SelfRtts)
		boundedAllForeignRtts.Append(&uploadDirection.ForeignRtts)
	}

	result := rpm.CalculateRpm(boundedAllSelfRtts, boundedAllForeignRtts,
		specParameters.TrimmedMeanPct, specParameters.Percentile)

	if *debugCliFlag || *detailedCliFlag {
		fmt.Printf("Final RPM Calculation stats:\n%v\n",
			utilities.IndentOutput(result.ToString(), 1, "\t"))
	}

	fmt.Printf("Final RPM: %.0f (P%d)\n", result.PNRpm, specParameters.Percentile)
	fmt.Printf("Final RPM: %.0f (Single-Sided %v%% Trimmed Mean)\n",
		result.MeanRpm, specParameters.TrimmedMeanPct)

	if *detailedCliFlag {
		fmt.Printf("Final RPM (Self Only): %.0f (P%d)\n", result.SelfPNRpm, specParameters.Percentile)
		fmt.Printf("Final RPM (Self Only): %.0f (Single-Sided %v%% Trimmed Mean)\n",
			result.SelfMeanRpm, specParameters.TrimmedMeanPct)

		fmt.Printf("Final RPM (Foreign Only): %.0f (P%d)\n", result.ForeignPNRpm, specParameters.Percentile)
		fmt.Printf("Final RPM (Foreign Only): %.0f (Single-Sided %v%% Trimmed Mean)\n",
			result.ForeignMeanRpm, specParameters.TrimmedMeanPct)
	}

	if *calculateRelativeRpm {
		if baselineRpm == nil {
			fmt.Printf("User requested relative RPM calculation but an unloaded RPM was not calculated.")
		} else {
			relativeRpmFactorP := (result.PNRpm / baselineRpm.PNRpm) * 100.0
			relativeRpmFactorTM := (result.MeanRpm / baselineRpm.MeanRpm) * 100.0
			fmt.Printf("Working-Conditions Effect: Final RPM is %5.0f%% of baseline RPM (P%d)\n",
				relativeRpmFactorP, specParameters.Percentile)
			fmt.Printf("Working-Conditions Effect: Final RPM is %5.0f%% of baseline RPM (Single-Sided %v%% Trimmed Mean)\n",
				relativeRpmFactorTM, specParameters.TrimmedMeanPct)
		}
	}

	// Stop the world.
	operatingCtxCancel()

	// Note: We do *not* have to export/close the upload *and* download
	// sides of the self/foreign probe data loggers because they both
	// refer to the same logger. Closing/exporting one will close/export
	// the other.
	uploadDirection.SelfProbeDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the self data loggers.\n")
	}
	uploadDirection.SelfProbeDataLogger.Close()

	uploadDirection.ForeignProbeDataLogger.Export()
	if *debugCliFlag {
		fmt.Printf("Closing the foreign data loggers.\n")
	}
	uploadDirection.ForeignProbeDataLogger.Close()
}
