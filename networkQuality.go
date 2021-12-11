package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hawkinsw/goresponsiveness/ma"
	"github.com/hawkinsw/goresponsiveness/mc"
	_ "io"
	"io/ioutil"
	_ "log"
	"net/http"
	"time"
)

type ConfigUrls struct {
	SmallUrl  string `json:"small_https_download_url"`
	LargeUrl  string `json:"large_https_download_url"`
	UploadUrl string `json:"https_upload_url"`
}

type Config struct {
	Version int
	Urls    ConfigUrls `json:"urls"`
}

func (c *Config) String() string {
	return fmt.Sprintf("Version: %d\nSmall URL: %s\nLarge URL: %s\nUpload URL: %s", c.Version, c.Urls.SmallUrl, c.Urls.LargeUrl, c.Urls.UploadUrl)
}

func toMBs(bytes float64) float64 {
	return float64(bytes) / float64(1024*1024)
}

var (
	// Variables to hold CLI arguments.
	configHost = flag.String("config", "networkquality.example.com", "name/IP of responsiveness configuration server.")
	configPort = flag.Int("port", 4043, "port number on which to access responsiveness configuration server.")
	debug      = flag.Bool("debug", false, "Enable debugging.")
)

func saturate(ctx context.Context, saturated chan<- interface{}, lbcGenerator func() mc.MeasurableConnection) {
	mcs := make([]mc.MeasurableConnection, 4)
	mcsPreviousTransferred := make([]uint64, 4)
	for i := range mcs {
		//mcs[i] = &mc.LoadBearingUpload{Path: config.Urls.UploadUrl}
		mcs[i] = lbcGenerator()
		mcsPreviousTransferred[i] = 0
		if !mcs[i].Start(ctx) {
			fmt.Printf("Error starting %dth MC!\n", i)
			return
		}
	}

	previousMovingAverage := float64(0)
	movingAverage := ma.NewMovingAverage(4)
	//lastFlowIncrease := uint64(0)
	for currentIteration := uint64(0); true; currentIteration++ {

		// If we are cancelled, then stop.
		if ctx.Err() != nil {
			return
		}

		time.Sleep(time.Second)

		// 1. Calculate the most recent goodput.
		totalTransfer := uint64(0)
		for i := range mcs {
			previousTransferred := mcsPreviousTransferred[i]
			currentTransferred := mcs[i].Transferred()
			totalTransfer += (currentTransferred - previousTransferred)
			mcsPreviousTransferred[i] = currentTransferred
		}

		// 2. Calculate the delta
		movingAverage.AddMeasurement(totalTransfer)
		currentMovingAverage := movingAverage.CalculateAverage()
		movingAverageDelta := ((currentMovingAverage - previousMovingAverage) / (float64(currentMovingAverage+previousMovingAverage) / 2.0)) * float64(100)
		previousMovingAverage = currentMovingAverage

		fmt.Printf("Instantaneous goodput: %f MB.\n", toMBs(float64(totalTransfer)))
		fmt.Printf("Moving average: %f MB.\n", toMBs(currentMovingAverage))
		fmt.Printf("Moving average delta: %f.\n", movingAverageDelta)


		// 3. Are we stable or not?
		if currentIteration != 0 && movingAverageDelta < float64(5) {
			// We are stable!
			fmt.Printf("Stable\n")
			break
		} else {
			// We are unstable!
			fmt.Printf("Unstable\n")
		}
	}
	saturated <- struct{}{}
}

func main() {
	flag.Parse()

	configHostPort := fmt.Sprintf("%s:%d", *configHost, *configPort)
	configUrl := fmt.Sprintf("https://%s/config", configHostPort)

	configClient := &http.Client{}
	resp, err := configClient.Get(configUrl)
	if err != nil {
		fmt.Printf("Error connecting to %s: %v\n", configHostPort, err)
		return
	}

	jsonConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading content downloaded from %s: %v\n", configUrl, err)
		return
	}

	var config Config
	err = json.Unmarshal(jsonConfig, &config)
	if err != nil {
		fmt.Printf("Error parsing configuration returned from %s: %v\n", configUrl, err)
		return
	}

	if *debug {
		fmt.Printf("Configuration: %s\n", &config)
	}

	operatingCtx, cancelOperatingCtx := context.WithCancel(context.Background())

	uploadSaturationChannel := make(chan interface{})

	g := func() mc.MeasurableConnection {
					return &mc.LoadBearingDownload{Path: config.Urls.LargeUrl}
	}


	go saturate(operatingCtx, uploadSaturationChannel, g)

	select {
	case <-uploadSaturationChannel:
		{
			fmt.Printf("upload is saturated!\n")
		}
	}

	time.Sleep(10 * time.Second)

	cancelOperatingCtx()

	time.Sleep(4 * time.Second)
}
