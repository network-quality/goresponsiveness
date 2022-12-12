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

package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

type ConfigUrls struct {
	SmallUrl      string `json:"small_https_download_url"`
	SmallUrlHost  string
	LargeUrl      string `json:"large_https_download_url"`
	LargeUrlHost  string
	UploadUrl     string `json:"https_upload_url"`
	UploadUrlHost string
}

type Config struct {
	Version       int
	Urls          ConfigUrls `json:"urls"`
	Source        string
	Test_Endpoint string
}

func (c *Config) Get(configHost string, configPath string, keyLogger io.Writer) error {
	configTransport := http2.Transport{}
	configTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	if !utilities.IsInterfaceNil(keyLogger) {
		configTransport.TLSClientConfig.KeyLogWriter = keyLogger
	}
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
			"could not connect to configuration host %s: %v",
			configHost,
			err,
		)
	}

	jsonConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf(
			"could not read configuration content downloaded from %s: %v",
			c.Source,
			err,
		)
	}

	err = json.Unmarshal(jsonConfig, c)
	if err != nil {
		return fmt.Errorf(
			"could not parse configuration returned from %s: %v",
			c.Source,
			err,
		)
	}

	if len(c.Test_Endpoint) != 0 {
		tempUrl, err := url.Parse(c.Urls.LargeUrl)
		if err != nil {
			return fmt.Errorf("error parsing large_https_download_url: %v", err)
		}
		c.Urls.LargeUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "" + tempUrl.Path
		c.Urls.LargeUrlHost = tempUrl.Host
		tempUrl, err = url.Parse(c.Urls.SmallUrl)
		if err != nil {
			return fmt.Errorf("error parsing small_https_download_url: %v", err)
		}
		c.Urls.SmallUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "" + tempUrl.Path
		c.Urls.SmallUrlHost = tempUrl.Host
		tempUrl, err = url.Parse(c.Urls.UploadUrl)
		if err != nil {
			return fmt.Errorf("error parsing https_upload_url: %v", err)
		}
		c.Urls.UploadUrl = tempUrl.Scheme + "://" + c.Test_Endpoint + "" + tempUrl.Path
		c.Urls.UploadUrlHost = tempUrl.Host
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
			"configuration url large_https_download_url is invalid: %s",
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
			"configuration url small_https_download_url is invalid: %s",
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
			"configuration url https_upload_url is invalid: %s",
			utilities.Conditional(
				len(c.Urls.UploadUrl) != 0,
				c.Urls.UploadUrl,
				"Missing",
			),
		)
	}
	return nil
}
