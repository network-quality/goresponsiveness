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

	"github.com/hawkinsw/goresponsiveness/ccw"
	"github.com/hawkinsw/goresponsiveness/lbc"
	"github.com/hawkinsw/goresponsiveness/ma"
	"github.com/hawkinsw/goresponsiveness/timeoutat"
	"github.com/hawkinsw/goresponsiveness/utilities"
)

var (
	// Variables to hold CLI arguments.
	configHost     = flag.String("config", "networkquality.example.com", "name/IP of responsiveness configuration server.")
	configPort     = flag.Int("port", 4043, "port number on which to access responsiveness configuration server.")
	configPath     = flag.String("path", "config", "path on the server to the configuration endpoint.")
	debug          = flag.Bool("debug", false, "Enable debugging.")
	timeout        = flag.Int("timeout", 20, "Maximum time to spend measuring.")
	sslKeyFileName = flag.String("ssl-key-file", "", "Store the per-session SSL key files in this file.")
	profile        = flag.String("profile", "", "Enable client runtime profiling and specify storage location. Disabled by default.")

	// Global configuration
	cooldownPeriod                time.Duration = 4 * time.Second
	robustnessProbeIterationCount int           = 5
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
		return fmt.Errorf("Error: Could not connect to configuration host %s: %v\n", configHost, err)
	}

	jsonConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error: Could not read configuration content downloaded from %s: %v\n", c.Source, err)
	}

	err = json.Unmarshal(jsonConfig, c)
	if err != nil {
		return fmt.Errorf("Error: Could not parse configuration returned from %s: %v\n", c.Source, err)
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
	return fmt.Sprintf("Version: %d\nSmall URL: %s\nLarge URL: %s\nUpload URL: %s\nEndpoint: %s\n", c.Version, c.Urls.SmallUrl, c.Urls.LargeUrl, c.Urls.UploadUrl, c.Test_Endpoint)
}

func (c *Config) IsValid() error {
	if parsedUrl, err := url.ParseRequestURI(c.Urls.LargeUrl); err != nil || parsedUrl.Scheme != "https" {
		return fmt.Errorf("Configuration url large_https_download_url is invalid: %s", utilities.Conditional(len(c.Urls.LargeUrl) != 0, c.Urls.LargeUrl, "Missing"))
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.SmallUrl); err != nil || parsedUrl.Scheme != "https" {
		return fmt.Errorf("Configuration url small_https_download_url is invalid: %s", utilities.Conditional(len(c.Urls.SmallUrl) != 0, c.Urls.SmallUrl, "Missing"))
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.UploadUrl); err != nil || parsedUrl.Scheme != "https" {
		return fmt.Errorf("Configuration url https_upload_url is invalid: %s", utilities.Conditional(len(c.Urls.UploadUrl) != 0, c.Urls.UploadUrl, "Missing"))
	}
	return nil
}

func toMbps(bytes float64) float64 {
	return toMBps(bytes) * float64(8)
}

func toMBps(bytes float64) float64 {
	return float64(bytes) / float64(1024*1024)
}

func addFlows(ctx context.Context, toAdd uint64, lbcs *[]lbc.LoadBearingConnection, lbcsPreviousTransferred *[]uint64, lbcGenerator func() lbc.LoadBearingConnection, debug bool) {
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

func saturate(saturationCtx context.Context, operatingCtx context.Context, lbcGenerator func() lbc.LoadBearingConnection, debug *Debugging) (saturated chan SaturationResult) {
	saturated = make(chan SaturationResult)
	go func() {

		lbcs := make([]lbc.LoadBearingConnection, 0)
		lbcsPreviousTransferred := make([]uint64, 0)

		// Create 4 load bearing connections
		addFlows(operatingCtx, 4, &lbcs, &lbcsPreviousTransferred, lbcGenerator, debug != nil)

		previousFlowIncreaseIteration := uint64(0)
		previousMovingAverage := float64(0)
		movingAverage := ma.NewMovingAverage(4)
		movingAverageAverage := ma.NewMovingAverage(4)

		nextSampleStartTime := time.Now().Add(time.Second)

		for currentIteration := uint64(0); true; currentIteration++ {

			// When the program stops operating, then stop.
			if operatingCtx.Err() != nil {
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
						fmt.Printf("%v: Load-bearing connection at index %d is invalid ... skipping.\n", debug, i)
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
					fmt.Printf("%v: All LBCs were invalid. Assuming that network/server went away.\n", debug)
				}
				break
			}

			// Compute a moving average of the last 4 "instantaneous aggregate goodput" measurements
			movingAverage.AddMeasurement(float64(totalTransfer))
			currentMovingAverage := movingAverage.CalculateAverage()
			movingAverageAverage.AddMeasurement(currentMovingAverage)
			movingAverageDelta := utilities.SignedPercentDifference(currentMovingAverage, previousMovingAverage)

			if debug != nil {
				fmt.Printf("%v: Instantaneous goodput: %f MB.\n", debug, toMBps(float64(totalTransfer)))
				fmt.Printf("%v: Previous moving average: %f MB.\n", debug, toMBps(previousMovingAverage))
				fmt.Printf("%v: Current moving average: %f MB.\n", debug, toMBps(currentMovingAverage))
				fmt.Printf("%v: Moving average delta: %f.\n", debug, movingAverageDelta)
			}

			previousMovingAverage = currentMovingAverage

			// Special case: We won't make any adjustments on the first iteration.
			if currentIteration == 0 {
				continue
			}

			// If moving average > "previous" moving average + 5%:
			if movingAverageDelta > float64(5) {
				// Network did not yet reach saturation. If no flows added within the last 4 seconds, add 4 more flows
				if (currentIteration - previousFlowIncreaseIteration) > 4 {
					if debug != nil {
						fmt.Printf("%v: Adding flows because we are unsaturated and waited a while.\n", debug)
					}
					addFlows(operatingCtx, 4, &lbcs, &lbcsPreviousTransferred, lbcGenerator, debug != nil)
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
				if (currentIteration-previousFlowIncreaseIteration) < 4 && movingAverageAverage.AllSequentialIncreasesLessThan(float64(5)) {
					if debug != nil {
						fmt.Printf("%v: New flows added within the last four seconds and the moving-average average is consistent!\n", debug)
					}
					break
				} else {
					// Else, add four more flows
					if debug != nil {
						fmt.Printf("%v: New flows to add to try to increase our saturation!\n", debug)
					}
					addFlows(operatingCtx, 4, &lbcs, &lbcsPreviousTransferred, lbcGenerator, debug != nil)
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
		fmt.Fprintf(os.Stderr, "Error: Invalid configuration returned from %s: %v\n", config.Source, err)
		return
	}
	if *debug {
		fmt.Printf("Configuration: %s\n", config)
	}

	timeoutChannel := timeoutat.TimeoutAt(operatingCtx, timeoutAbsoluteTime, *debug)
	if *debug {
		fmt.Printf("Test will end earlier than %v\n", timeoutAbsoluteTime)
	}

	if len(*profile) != 0 {
		f, err := os.Create(*profile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Profiling requested with storage in %s but that file could not be opened: %v\n", *profile, err)
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
		return &lbc.LoadBearingConnectionDownload{Path: config.Urls.LargeUrl, KeyLogger: sslKeyFileConcurrentWriter}
	}
	generate_lbu := func() lbc.LoadBearingConnection {
		return &lbc.LoadBearingConnectionUpload{Path: config.Urls.UploadUrl, KeyLogger: sslKeyFileConcurrentWriter}
	}

	var downloadDebugging *Debugging = nil
	var uploadDebugging *Debugging = nil
	if *debug {
		downloadDebugging = &Debugging{Prefix: "download"}
		uploadDebugging = &Debugging{Prefix: "upload"}
	}

	downloadSaturationChannel := saturate(saturationCtx, operatingCtx, generate_lbd, downloadDebugging)
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
					fmt.Printf("################# download is %s saturated (%fMBps, %d flows)!\n", utilities.Conditional(saturationTimeout, "(provisionally)", ""), toMBps(downloadSaturation.RateBps), len(downloadSaturation.Lbcs))
				}
			}
		case uploadSaturation = <-uploadSaturationChannel:
			{
				uploadSaturated = true
				if *debug {
					fmt.Printf("################# upload is %s saturated (%fMBps, %d flows)!\n", utilities.Conditional(saturationTimeout, "(provisionally)", ""), toMBps(uploadSaturation.RateBps), len(uploadSaturation.Lbcs))
				}
			}
		case <-timeoutChannel:
			{
				if saturationTimeout {
					// We already timedout on saturation. This signal means that
					// we are timedout on getting the provisional saturation. We
					// will exit!
					fmt.Fprint(os.Stderr, "Error: Saturation could not be completed in time and no provisional rates could be accessed. Test failed.\n")
					cancelOperatingCtx()
					if *debug {
						time.Sleep(cooldownPeriod)
					}
					return
				}
				saturationTimeout = true
				timeoutAbsoluteTime = time.Now().Add(5 * time.Second)
				timeoutChannel = timeoutat.TimeoutAt(operatingCtx, timeoutAbsoluteTime, *debug)
				cancelSaturationCtx()
				if *debug {
					fmt.Printf("################# timeout reaching saturation!\n")
				}
			}
		}
	}

	// If there was a timeout achieving saturation then we already added another 5 seconds
	// to the available time for testing. However, if saturation was achieved before the timeout
	// then we want to give ourselves another 5 seconds to calculate the RPM.
	if !saturationTimeout {
		timeoutAbsoluteTime = time.Now().Add(5 * time.Second)
		timeoutChannel = timeoutat.TimeoutAt(operatingCtx, timeoutAbsoluteTime, *debug)
	}

	totalRTTsCount := 0
	totalRTTTime := float64(0)
	rttTimeout := false

	for i := 0; i < robustnessProbeIterationCount && !rttTimeout; i++ {
		if len(downloadSaturation.Lbcs) == 0 {
			continue
		}
		randomLbcsIndex := rand.New(rand.NewSource(int64(time.Now().Nanosecond()))).Int() % len(downloadSaturation.Lbcs)
		if !downloadSaturation.Lbcs[randomLbcsIndex].IsValid() {
			if debug != nil {
				fmt.Printf("%v: The randomly selected download LBC (at index %d) was invalid. Skipping.\n", debug, randomLbcsIndex)
			}

			// Protect against pathological cases where we continuously select invalid connections and never
			// do the select below
			if time.Since(timeoutAbsoluteTime) > 0 {
				if *debug {
					fmt.Printf("Pathologically could not find valid LBCs to use for measurement.\n")
				}
				break
			}
			continue
		}
		select {
		case <-timeoutChannel:
			{
				rttTimeout = true
			}
		case fiveRTTsTime := <-utilities.TimedSequentialRTTs(operatingCtx, downloadSaturation.Lbcs[randomLbcsIndex].Client(), &http.Client{}, config.Urls.SmallUrl):
			{
				totalRTTsCount += 5
				totalRTTTime += fiveRTTsTime.Delay.Seconds()
				if *debug {
					fmt.Printf("fiveRTTsTime: %v\n", fiveRTTsTime.Delay.Seconds())
				}
			}
		}
	}

	fmt.Printf("Download: %f MBps (%f Mbps), using %d parallel connections.\n", toMBps(downloadSaturation.RateBps), toMbps(downloadSaturation.RateBps), len(downloadSaturation.Lbcs))
	fmt.Printf("Upload: %f MBps (%f Mbps), using %d parallel connections.\n", toMBps(uploadSaturation.RateBps), toMbps(uploadSaturation.RateBps), len(uploadSaturation.Lbcs))

	if totalRTTsCount != 0 {
		rpm := float64(60) / (totalRTTTime / (float64(totalRTTsCount)))
		fmt.Printf("Total RTTs measured: %d\n", totalRTTsCount)
		fmt.Printf("RPM: %v\n", rpm)
	} else {
		fmt.Printf("Error occurred calculating RPM -- no probe measurements received.\n")
	}

	cancelOperatingCtx()
	if *debug {
		time.Sleep(cooldownPeriod)
	}
}
