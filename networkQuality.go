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
	"encoding/json"
	"flag"
	"fmt"
	_ "io"
	"io/ioutil"
	_ "log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/network-quality/goresponsiveness/ccw"
	"github.com/network-quality/goresponsiveness/constants"
	"github.com/network-quality/goresponsiveness/lbc"
	"github.com/network-quality/goresponsiveness/ma"
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
	debug   = flag.Bool("debug", constants.DefaultDebug, "Enable debugging.")
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

type ConfigUrls struct {
	SmallUrl  string `json:"small_https_download_url"`
	LargeUrl  string `json:"large_https_download_url"`
	UploadUrl string `json:"https_upload_url"`
}

type Config struct {
	Version       int
	Urls          ConfigUrls `json:"urls"`
	Source        string
	Test_Endpoint string
}

func (c *Config) Get(configHost string, configPath string) error {
	configClient := &http.Client{}
	// Extraneous /s in URLs is normally okay, but the Apple CDN does not
	// like them. Make sure that we put exactly one (1) / between the host
	// and the path.
	if !strings.HasPrefix(configPath, "/") {
		configPath = "/" + configPath
	}
	c.Source = fmt.Sprintf("https://%s%s", configHost, configPath)
	resp, err := configClient.Get(c.Source)
	if err != nil {
		return fmt.Errorf(
			"Error: Could not connect to configuration host %s: %v\n",
			configHost,
			err,
		)
	}

	jsonConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf(
			"Error: Could not read configuration content downloaded from %s: %v\n",
			c.Source,
			err,
		)
	}

	err = json.Unmarshal(jsonConfig, c)
	if err != nil {
		return fmt.Errorf(
			"Error: Could not parse configuration returned from %s: %v\n",
			c.Source,
			err,
		)
	}

	//if len(c.Test_Endpoint) != 0 {
	if false {
		tempUrl, err := url.Parse(c.Urls.LargeUrl)
		if err != nil {
			return fmt.Errorf("Error parsing large_https_download_url: %v", err)
		}
		c.Urls.LargeUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "/" + tempUrl.Path
		tempUrl, err = url.Parse(c.Urls.SmallUrl)
		if err != nil {
			return fmt.Errorf("Error parsing small_https_download_url: %v", err)
		}
		c.Urls.SmallUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "/" + tempUrl.Path
		tempUrl, err = url.Parse(c.Urls.UploadUrl)
		if err != nil {
			return fmt.Errorf("Error parsing https_upload_url: %v", err)
		}
		c.Urls.UploadUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "/" + tempUrl.Path
	}
	return nil
}

func (c *Config) String() string {
	return fmt.Sprintf(
		"Version: %d\nSmall URL: %s\nLarge URL: %s\nUpload URL: %s\nEndpoint: %s\n",
		c.Version,
		c.Urls.SmallUrl,
		c.Urls.LargeUrl,
		c.Urls.UploadUrl,
		c.Test_Endpoint,
	)
}

func (c *Config) IsValid() error {
	if parsedUrl, err := url.ParseRequestURI(c.Urls.LargeUrl); err != nil ||
		parsedUrl.Scheme != "https" {
		return fmt.Errorf(
			"Configuration url large_https_download_url is invalid: %s",
			utilities.Conditional(len(c.Urls.LargeUrl) != 0, c.Urls.LargeUrl, "Missing"),
		)
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.SmallUrl); err != nil ||
		parsedUrl.Scheme != "https" {
		return fmt.Errorf(
			"Configuration url small_https_download_url is invalid: %s",
			utilities.Conditional(len(c.Urls.SmallUrl) != 0, c.Urls.SmallUrl, "Missing"),
		)
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.UploadUrl); err != nil ||
		parsedUrl.Scheme != "https" {
		return fmt.Errorf(
			"Configuration url https_upload_url is invalid: %s",
			utilities.Conditional(len(c.Urls.UploadUrl) != 0, c.Urls.UploadUrl, "Missing"),
		)
	}
	return nil
}

func addFlows(
	ctx context.Context,
	toAdd uint64,
	lbcs *[]lbc.LoadBearingConnection,
	lbcsPreviousTransferred *[]uint64,
	lbcGenerator func() lbc.LoadBearingConnection,
	debug bool,
) {
	for i := uint64(0); i < toAdd; i++ {
		*lbcs = append(*lbcs, lbcGenerator())
		*lbcsPreviousTransferred = append(*lbcsPreviousTransferred, 0)
		if !(*lbcs)[len(*lbcs)-1].Start(ctx, debug) {
			fmt.Printf("Error starting %dth LBC!\n", i)
			return
		}
	}
}

type SaturationResult struct {
	RateBps float64
	Lbcs    []lbc.LoadBearingConnection
}

type Debugging struct {
	Prefix string
}

func NewDebugging(prefix string) *Debugging {
	return &Debugging{Prefix: prefix}
}

func (d *Debugging) String() string {
	return d.Prefix
}

func saturate(
	saturationCtx context.Context,
	operatingCtx context.Context,
	lbcGenerator func() lbc.LoadBearingConnection,
	debug *Debugging,
) (saturated chan SaturationResult) {
	saturated = make(chan SaturationResult)
	go func() {

		lbcs := make([]lbc.LoadBearingConnection, 0)
		lbcsPreviousTransferred := make([]uint64, 0)

		addFlows(
			saturationCtx,
			constants.StartingNumberOfLoadBearingConnections,
			&lbcs,
			&lbcsPreviousTransferred,
			lbcGenerator,
			debug != nil,
		)

		previousFlowIncreaseIteration := uint64(0)
		previousMovingAverage := float64(0)
		movingAverage := ma.NewMovingAverage(constants.MovingAverageIntervalCount)
		movingAverageAverage := ma.NewMovingAverage(constants.MovingAverageIntervalCount)

		nextSampleStartTime := time.Now().Add(time.Second)

		for currentIteration := uint64(0); true; currentIteration++ {

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
				if debug != nil {
					fmt.Printf("%v: Sleeping until %v\n", debug, nextSampleStartTime)
				}
				time.Sleep(nextSampleStartTime.Sub(now))
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Missed a one-second deadline.\n")
			}
			nextSampleStartTime = time.Now().Add(time.Second)

			// Compute "instantaneous aggregate" goodput which is the number of bytes transferred within the last second.
			totalTransfer := uint64(0)
			allInvalid := true
			for i := range lbcs {
				if !lbcs[i].IsValid() {
					if debug != nil {
						fmt.Printf(
							"%v: Load-bearing connection at index %d is invalid ... skipping.\n",
							debug,
							i,
						)
					}
					continue
				}
				allInvalid = false
				previousTransferred := lbcsPreviousTransferred[i]
				currentTransferred := lbcs[i].Transferred()
				totalTransfer += (currentTransferred - previousTransferred)
				lbcsPreviousTransferred[i] = currentTransferred
			}

			// For some reason, all the LBCs are invalid. This likely means that the network/server went away.
			if allInvalid {
				if debug != nil {
					fmt.Printf(
						"%v: All LBCs were invalid. Assuming that network/server went away.\n",
						debug,
					)
				}
				break
			}

			// Compute a moving average of the last constants.MovingAverageIntervalCount "instantaneous aggregate goodput" measurements
			movingAverage.AddMeasurement(float64(totalTransfer))
			currentMovingAverage := movingAverage.CalculateAverage()
			movingAverageAverage.AddMeasurement(currentMovingAverage)
			movingAverageDelta := utilities.SignedPercentDifference(
				currentMovingAverage,
				previousMovingAverage,
			)

			if debug != nil {
				fmt.Printf(
					"%v: Instantaneous goodput: %f MB.\n",
					debug,
					utilities.ToMBps(float64(totalTransfer)),
				)
				fmt.Printf(
					"%v: Previous moving average: %f MB.\n",
					debug,
					utilities.ToMBps(previousMovingAverage),
				)
				fmt.Printf(
					"%v: Current moving average: %f MB.\n",
					debug,
					utilities.ToMBps(currentMovingAverage),
				)
				fmt.Printf("%v: Moving average delta: %f.\n", debug, movingAverageDelta)
			}

			previousMovingAverage = currentMovingAverage

			// Special case: We won't make any adjustments on the first iteration.
			if currentIteration == 0 {
				continue
			}

			// If moving average > "previous" moving average + InstabilityDelta:
			if movingAverageDelta > constants.InstabilityDelta {
				// Network did not yet reach saturation. If no flows added within the last 4 seconds, add 4 more flows
				if (currentIteration - previousFlowIncreaseIteration) > uint64(
					constants.MovingAverageStabilitySpan,
				) {
					if debug != nil {
						fmt.Printf(
							"%v: Adding flows because we are unsaturated and waited a while.\n",
							debug,
						)
					}
					addFlows(
						saturationCtx,
						constants.AdditiveNumberOfLoadBearingConnections,
						&lbcs,
						&lbcsPreviousTransferred,
						lbcGenerator,
						debug != nil,
					)
					previousFlowIncreaseIteration = currentIteration
				} else {
					if debug != nil {
						fmt.Printf("%v: We are unsaturated, but it still too early to add anything.\n", debug)
					}
				}
			} else { // Else, network reached saturation for the current flow count.
				if debug != nil {
					fmt.Printf("%v: Network reached saturation with current flow count.\n", debug)
				}
				// If new flows added and for 4 seconds the moving average throughput did not change: network reached stable saturation
				if (currentIteration-previousFlowIncreaseIteration) < uint64(constants.MovingAverageStabilitySpan) && movingAverageAverage.AllSequentialIncreasesLessThan(float64(5)) {
					if debug != nil {
						fmt.Printf("%v: New flows added within the last four seconds and the moving-average average is consistent!\n", debug)
					}
					break
				} else {
					// Else, add four more flows
					if debug != nil {
						fmt.Printf("%v: New flows to add to try to increase our saturation!\n", debug)
					}
					addFlows(saturationCtx, constants.AdditiveNumberOfLoadBearingConnections, &lbcs, &lbcsPreviousTransferred, lbcGenerator, debug != nil)
					previousFlowIncreaseIteration = currentIteration
				}
			}

		}
		saturated <- SaturationResult{RateBps: movingAverage.CalculateAverage(), Lbcs: lbcs}
	}()
	return
}

func main() {
	flag.Parse()

	timeoutDuration := time.Second * time.Duration(*timeout)
	timeoutAbsoluteTime := time.Now().Add(timeoutDuration)
	configHostPort := fmt.Sprintf("%s:%d", *configHost, *configPort)
	operatingCtx, cancelOperatingCtx := context.WithCancel(context.Background())
	saturationCtx, cancelSaturationCtx := context.WithCancel(context.Background())
	config := &Config{}

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
	if *debug {
		fmt.Printf("Configuration: %s\n", config)
	}

	timeoutChannel := timeoutat.TimeoutAt(operatingCtx, timeoutAbsoluteTime, *debug)
	if *debug {
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
				if *debug {
					fmt.Printf("Doing SSL key logging through file %v\n", *sslKeyFileName)
				}
				sslKeyFileConcurrentWriter = ccw.NewConcurrentFileWriter(sslKeyFileHandle)
				defer sslKeyFileHandle.Close()
			}
		}
	}

	generate_lbd := func() lbc.LoadBearingConnection {
		return &lbc.LoadBearingConnectionDownload{
			Path:      config.Urls.LargeUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}
	generate_lbu := func() lbc.LoadBearingConnection {
		return &lbc.LoadBearingConnectionUpload{
			Path:      config.Urls.UploadUrl,
			KeyLogger: sslKeyFileConcurrentWriter,
		}
	}

	var downloadDebugging *Debugging = nil
	var uploadDebugging *Debugging = nil
	if *debug {
		downloadDebugging = &Debugging{Prefix: "download"}
		uploadDebugging = &Debugging{Prefix: "upload"}
	}

	downloadSaturationChannel := saturate(
		saturationCtx,
		operatingCtx,
		generate_lbd,
		downloadDebugging,
	)
	uploadSaturationChannel := saturate(saturationCtx, operatingCtx, generate_lbu, uploadDebugging)

	saturationTimeout := false
	uploadSaturated := false
	downloadSaturated := false
	downloadSaturation := SaturationResult{}
	uploadSaturation := SaturationResult{}

	for !(uploadSaturated && downloadSaturated) {
		select {
		case downloadSaturation = <-downloadSaturationChannel:
			{
				downloadSaturated = true
				if *debug {
					fmt.Printf(
						"################# download is %s saturated (%fMBps, %d flows)!\n",
						utilities.Conditional(saturationTimeout, "(provisionally)", ""),
						utilities.ToMBps(downloadSaturation.RateBps),
						len(downloadSaturation.Lbcs),
					)
				}
			}
		case uploadSaturation = <-uploadSaturationChannel:
			{
				uploadSaturated = true
				if *debug {
					fmt.Printf(
						"################# upload is %s saturated (%fMBps, %d flows)!\n",
						utilities.Conditional(saturationTimeout, "(provisionally)", ""),
						utilities.ToMBps(uploadSaturation.RateBps),
						len(uploadSaturation.Lbcs),
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
						"Error: Saturation could not be completed in time and no provisional rates could be accessed. Test failed.\n",
					)
					cancelOperatingCtx()
					if *debug {
						time.Sleep(constants.CooldownPeriod)
					}
					return
				}
				saturationTimeout = true

				// We timed out attempting to saturate the link. So, we will shut down all the saturation xfers
				cancelSaturationCtx()
				// and then we will give ourselves some additional time in order to calculate a provisional saturation.
				timeoutAbsoluteTime = time.Now().Add(constants.RPMCalculationTime)
				timeoutChannel = timeoutat.TimeoutAt(operatingCtx, timeoutAbsoluteTime, *debug)
				if *debug {
					fmt.Printf("################# timeout reaching saturation!\n")
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
			*debug,
		)
	}

	totalRTsCount := uint64(0)
	totalRTTimes := float64(0)
	rttTimeout := false

	for i := 0; i < constants.RPMProbeCount && !rttTimeout; i++ {
		if len(downloadSaturation.Lbcs) == 0 {
			continue
		}
		randomLbcsIndex := rand.New(rand.NewSource(int64(time.Now().Nanosecond()))).
			Int() %
			len(
				downloadSaturation.Lbcs,
			)
		if !downloadSaturation.Lbcs[randomLbcsIndex].IsValid() {
			if *debug {
				fmt.Printf(
					"%v: The randomly selected download LBC (at index %d) was invalid. Skipping.\n",
					debug,
					randomLbcsIndex,
				)
			}

			// Protect against pathological cases where we continuously select
			// invalid connections and never
			// do the select below
			if time.Since(timeoutAbsoluteTime) > 0 {
				if *debug {
					fmt.Printf(
						"Pathologically could not find valid LBCs to use for measurement.\n",
					)
				}
				break
			}
			continue
		}

		newTransport := http2.Transport{}
		newTransport.TLSClientConfig = &tls.Config{
			KeyLogWriter:       sslKeyFileConcurrentWriter,
			InsecureSkipVerify: true,
		}
		newClient := http.Client{Transport: &newTransport}

		select {
		case <-timeoutChannel:
			{
				rttTimeout = true
			}
		case sequentialRTTimes := <-utilities.CalculateSequentialRTTsTime(operatingCtx, downloadSaturation.Lbcs[randomLbcsIndex].Client(), &newClient, config.Urls.SmallUrl):
			{
				if sequentialRTTimes.Err != nil {
					fmt.Printf(
						"Failed to calculate a time for sequential RTTs: %v\n",
						sequentialRTTimes.Err,
					)
					continue
				}
				// We know that we have a good Sequential RTT.
				totalRTsCount += uint64(sequentialRTTimes.RoundTripCount)
				totalRTTimes += sequentialRTTimes.Delay.Seconds()
				if *debug {
					fmt.Printf(
						"sequentialRTTsTime: %v\n",
						sequentialRTTimes.Delay.Seconds(),
					)
				}
			}
		}
	}

	fmt.Printf(
		"Download: %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(downloadSaturation.RateBps),
		utilities.ToMBps(downloadSaturation.RateBps),
		len(downloadSaturation.Lbcs),
	)
	fmt.Printf(
		"Upload:   %7.3f Mbps (%7.3f MBps), using %d parallel connections.\n",
		utilities.ToMbps(uploadSaturation.RateBps),
		utilities.ToMBps(uploadSaturation.RateBps),
		len(uploadSaturation.Lbcs),
	)

	if totalRTsCount != 0 {
		// "... it sums the five time values for each probe, and divides by the
		// total
		// number of probes to compute an average probe duration.  The
		// reciprocal of this, normalized to 60 seconds, gives the Round-trips
		// Per Minute (RPM)."
		// "average probe duration" = totalRTTimes / totalRTsCount.
		// The reciprocol of this = 1 / (totalRTTimes / totalRTsCount) <-
		// semantically the probes-per-second.
		// Normalized to 60 seconds: 60 * (1
		// / (totalRTTimes / totalRTsCount))) <- semantically the number of
		// probes per minute.
		// I am concerned because the draft seems to conflate the concept of a
		// probe
		// with a roundtrip. In other words, I think that we are missing a
		// multiplication by 5: DNS, TCP, TLS, HTTP GET, HTTP Download.
		rpm := float64(
			time.Minute.Seconds(),
		) / (totalRTTimes / (float64(totalRTsCount)))
		fmt.Printf("Total RTTs measured: %d\n", totalRTsCount)
		fmt.Printf("RPM: %5.0f\n", rpm)
	} else {
		fmt.Printf("Error occurred calculating RPM -- no probe measurements received.\n")
	}

	cancelOperatingCtx()
	if *debug {
		fmt.Printf("In debugging mode, we will cool down.\n")
		time.Sleep(constants.CooldownPeriod)
		fmt.Printf("Done cooling down.\n")
	}
}
