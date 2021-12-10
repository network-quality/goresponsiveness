package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

var (
	// Variables to hold CLI arguments.
	configHost = flag.String("config", "networkquality.example.com", "name/IP of responsiveness configuration server.")
	configPort = flag.Int("port", 4043, "port number on which to access responsiveness configuration server.")
	debug      = flag.Bool("debug", false, "Enable debugging.")
)

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

	time.Sleep(4 * time.Second)

	mcs := make([]*mc.LoadBearingConnection, 4)
	for i := range mcs {
		mcs[i] = &mc.LoadBearingConnection{Path: config.Urls.LargeUrl}
		mcCtx := context.Background()
		if !mcs[i].Start(mcCtx) {
			fmt.Printf("Error starting %dth MC!\n", i)
			return
		}
	}

	time.Sleep(4 * time.Second)

	for i := range mcs {
		mcs[i].Stop()
		fmt.Printf("mc[%d] read: %d\n", i, mcs[i].Downloaded())
	}

	time.Sleep(4 * time.Second)
}
