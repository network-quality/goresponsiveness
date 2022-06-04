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
	"crypto/tls"
	"flag"
	"fmt"
	_ "io"
	_ "log"
	"net"
	"net/http"
	"os"
	"runtime/pprof"
	"time"

	"github.com/network-quality/goresponsiveness/ccw"
	"github.com/network-quality/goresponsiveness/config"
	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/lgc"
	"github.com/network-quality/goresponsiveness/rpm"
	"github.com/network-quality/goresponsiveness/timeoutat"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
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
	timeout = flag.Int(
		"timeout",
		constants.DefaultTestTime,
		"Maximum time to spend measuring.",
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
)

func main() {
	flag.Parse()

	timeoutDuration := time.Second * time.Duration(*timeout)
	timeoutAbsoluteTime := time.Now().Add(timeoutDuration)
	configHostPort := fmt.Sprintf("%s:%d", *configHost, *configPort)
	operatingCtx, cancelOperatingCtx := context.WithCancel(context.Background())
	saturationCtx, cancelSaturationCtx := context.WithCancel(
		context.Background(),
	)
	config := &config.Config{}
	var debugLevel debug.DebugLevel = debug.Error

	if *debugCliFlag {
		debugLevel = debug.Debug
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
	generate_lbd := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionDownload{
			Path:      config.Urls.LargeUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}
	generate_lbu := func() lgc.LoadGeneratingConnection {
		return &lgc.LoadGeneratingConnectionUpload{
			Path:      config.Urls.UploadUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}

	var downloadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "download")
	var uploadDebugging *debug.DebugWithPrefix = debug.NewDebugWithPrefix(debugLevel, "upload")

	downloadSaturationChannel := rpm.Saturate(
		saturationCtx,
		operatingCtx,
		generate_lbd,
		downloadDebugging,
	)
	uploadSaturationChannel := rpm.Saturate(
		saturationCtx,
		operatingCtx,
		generate_lbu,
		uploadDebugging,
	)

	saturationTimeout := false
	uploadSaturated := false
	downloadSaturated := false
	downloadSaturation := rpm.SaturationResult{}
	uploadSaturation := rpm.SaturationResult{}

	for !(uploadSaturated && downloadSaturated) {
		select {
		case downloadSaturation = <-downloadSaturationChannel:
			{
				downloadSaturated = true
				if *debugCliFlag {
					fmt.Printf(
						"################# download is %s saturated (%fMBps, %d flows)!\n",
						utilities.Conditional(
							saturationTimeout,
							"(provisionally)",
							"",
						),
						utilities.ToMBps(downloadSaturation.RateBps),
						len(downloadSaturation.LGCs),
					)
				}
			}
		case uploadSaturation = <-uploadSaturationChannel:
			{
				uploadSaturated = true
				if *debugCliFlag {
					fmt.Printf(
						"################# upload is %s saturated (%fMBps, %d flows)!\n",
						utilities.Conditional(
							saturationTimeout,
							"(provisionally)",
							"",
						),
						utilities.ToMBps(uploadSaturation.RateBps),
						len(uploadSaturation.LGCs),
					)
				}
			}
		case <-timeoutChannel:
			{
				if saturationTimeout {
					// We already timedout on saturation. This signal means that
					// we are timedout on getting the provisional saturation. We
					// will exit!
					fmt.Fprint(
						os.Stderr,
						"Error: Saturation could not be completed in time and no provisional rates could be assessed. Test failed.\n",
					)
					cancelOperatingCtx()
					if *debugCliFlag {
						time.Sleep(constants.CooldownPeriod)
					}
					return
				}
				saturationTimeout = true

				// We timed out attempting to saturate the link. So, we will
				// shut down all the saturation xfers
				cancelSaturationCtx()
				// and then we will give ourselves some additional time in order
				// to calculate a provisional saturation.
				timeoutAbsoluteTime = time.Now().
					Add(constants.RPMCalculationTime)
				timeoutChannel = timeoutat.TimeoutAt(
					operatingCtx,
					timeoutAbsoluteTime,
					debugLevel,
				)
				if *debugCliFlag {
					fmt.Printf(
						"################# timeout reaching saturation!\n",
					)
				}
			}
		}
	}

	// Give ourselves no more than 15 seconds to complete the RPM calculation.
	// This is conditional because (above) we may have already added the time.
	// We did it up there so that we could also limit the amount of time waiting
	// for a conditional saturation calculation.
	if !saturationTimeout {
		timeoutAbsoluteTime = time.Now().Add(constants.RPMCalculationTime)
		timeoutChannel = timeoutat.TimeoutAt(
			operatingCtx,
			timeoutAbsoluteTime,
			debugLevel,
		)
	}

	totalMeasurements := uint64(0)
	totalMeasurementTimes := float64(0)
	measurementTimeout := false

	for i := 0; i < constants.MeasurementProbeCount && !measurementTimeout; i++ {
		if len(downloadSaturation.LGCs) == 0 {
			continue
		}
		randomLGCsIndex := utilities.RandBetween(len(downloadSaturation.LGCs))
		if !downloadSaturation.LGCs[randomLGCsIndex].IsValid() {
			if *debugCliFlag {
				fmt.Printf(
					"%v: The randomly selected saturated connection (with id %d) was invalid. Skipping.\n",
					debugCliFlag,
					downloadSaturation.LGCs[randomLGCsIndex].ClientId(),
				)
			}

			// Protect against pathological cases where we continuously select
			// invalid connections and never
			// do the select below
			if time.Since(timeoutAbsoluteTime) > 0 {
				if *debugCliFlag {
					fmt.Printf(
						"Pathologically could not find valid saturated connections use for measurement.\n",
					)
				}
				break
			}
			continue
		}

		if *debugCliFlag {
			// Note: This code is just an example of how to use utilities.GetTCPInfo.
			rawConn := downloadSaturation.LGCs[randomLGCsIndex].Stats().ConnInfo.Conn
			tlsConn, ok := rawConn.(*tls.Conn)
			if !ok {
				fmt.Printf("OOPS: Could not get the TCP info for the connection (not a TLS connection)!\n")
			}
			tcpConn, ok := tlsConn.NetConn().(*net.TCPConn)
			if !ok {
				fmt.Printf("OOPS: Could not get the TCP info for the connection (not a TCP connection)!\n")
			}
			if info, err := utilities.GetTCPInfo(tcpConn); err != nil {
				fmt.Printf("OOPS: Could not get the TCP info for the connection: %v!\n", err)
			} else {
				utilities.PrintTCPInfo(info)
			}
		}

		unsaturatedMeasurementTransport := http2.Transport{}
		unsaturatedMeasurementTransport.TLSClientConfig = &tls.Config{}
		if sslKeyFileConcurrentWriter != nil {
			unsaturatedMeasurementTransport.TLSClientConfig.KeyLogWriter = sslKeyFileConcurrentWriter
		}
		unsaturatedMeasurementTransport.TLSClientConfig.InsecureSkipVerify = true
		newClient := http.Client{Transport: &unsaturatedMeasurementTransport}

		unsaturatedMeasurementProbe := rpm.NewProbe(&newClient, debugLevel)

		saturatedMeasurementProbe := rpm.NewProbe(
			downloadSaturation.LGCs[randomLGCsIndex].Client(),
			debugLevel,
		)

		select {
		case <-timeoutChannel:
			{
				measurementTimeout = true
			}
		case sequentialMeasurementTimes := <-rpm.CalculateProbeMeasurements(operatingCtx, *strictFlag, saturatedMeasurementProbe, unsaturatedMeasurementProbe, config.Urls.SmallUrl, debugLevel):
			{
				if sequentialMeasurementTimes.Err != nil {
					fmt.Printf(
						"Failed to calculate a time for sequential measurements: %v\n",
						sequentialMeasurementTimes.Err,
					)
					continue
				}

				if debug.IsDebug(debugLevel) {
					fmt.Printf("unsaturatedMeasurementProbe: %v\n", unsaturatedMeasurementProbe)
				}
				// We know that we have a good Sequential measurement.
				totalMeasurements += uint64(sequentialMeasurementTimes.MeasurementCount)
				totalMeasurementTimes += sequentialMeasurementTimes.Delay.Seconds()
				if debug.IsDebug(debugLevel) {
					fmt.Printf(
						"most-recent sequential measurement time: %v; most-recent sequential measurement count: %v\n",
						sequentialMeasurementTimes.Delay.Seconds(),
						sequentialMeasurementTimes.MeasurementCount,
					)
				}
			}
		}
	}

	fmt.Printf(
		"Download: %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(downloadSaturation.RateBps),
		utilities.ToMBps(downloadSaturation.RateBps),
		len(downloadSaturation.LGCs),
	)
	fmt.Printf(
		"Upload:   %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(uploadSaturation.RateBps),
		utilities.ToMBps(uploadSaturation.RateBps),
		len(uploadSaturation.LGCs),
	)

	if totalMeasurements != 0 {
		// "... it sums the five time values for each probe, and divides by the
		// total
		// number of probes to compute an average probe duration.  The
		// reciprocal of this, normalized to 60 seconds, gives the Round-trips
		// Per Minute (RPM)."
		// "average probe duration" = totalMeasurementTimes / totalMeasurements.
		// The reciprocol of this = 1 / (totalMeasurementTimes / totalMeasurements) <-
		// semantically the probes-per-second.
		// Normalized to 60 seconds: 60 * (1
		// / ((totalMeasurementTimes / totalMeasurements)))) <- semantically the number of
		// probes per minute.
		rpm := float64(
			time.Minute.Seconds(),
		) / (totalMeasurementTimes / (float64(totalMeasurements)))
		fmt.Printf("Total measurements: %d\n", totalMeasurements)
		fmt.Printf("RPM: %5.0f\n", rpm)
	} else {
		fmt.Printf("Error occurred calculating RPM -- no probe measurements received.\n")
	}

	cancelOperatingCtx()
	if *debugCliFlag {
		fmt.Printf("In debugging mode, we will cool down.\n")
		time.Sleep(constants.CooldownPeriod)
		fmt.Printf("Done cooling down.\n")
	}
}
