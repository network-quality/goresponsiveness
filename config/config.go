package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
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
	configTransport := http2.Transport{}
	configTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	configClient := &http.Client{Transport: &configTransport}
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
			utilities.Conditional(
				len(c.Urls.LargeUrl) != 0,
				c.Urls.LargeUrl,
				"Missing",
			),
		)
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.SmallUrl); err != nil ||
		parsedUrl.Scheme != "https" {
		return fmt.Errorf(
			"Configuration url small_https_download_url is invalid: %s",
			utilities.Conditional(
				len(c.Urls.SmallUrl) != 0,
				c.Urls.SmallUrl,
				"Missing",
			),
		)
	}
	if parsedUrl, err := url.ParseRequestURI(c.Urls.UploadUrl); err != nil ||
		parsedUrl.Scheme != "https" {
		return fmt.Errorf(
			"Configuration url https_upload_url is invalid: %s",
			utilities.Conditional(
				len(c.Urls.UploadUrl) != 0,
				c.Urls.UploadUrl,
				"Missing",
			),
		)
	}
	return nil
}
