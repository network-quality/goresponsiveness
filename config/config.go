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
	"net/http"
	"net/url"
	"strings"

	"github.com/network-quality/goresponsiveness/utilities"
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
	ConnectToAddr string `json:"test_endpoint"`
}

func (c *Config) Get(configHost string, configPath string, insecureSkipVerify bool, keyLogger io.Writer) error {
	configTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
		Proxy: http.ProxyFromEnvironment,
	}
	if !utilities.IsInterfaceNil(keyLogger) {
		configTransport.TLSClientConfig.KeyLogWriter = keyLogger
	}

	utilities.OverrideHostTransport(configTransport, c.ConnectToAddr)

	configClient := &http.Client{Transport: configTransport}

	// Extraneous /s in URLs is normally okay, but the Apple CDN does not
	// like them. Make sure that we put exactly one (1) / between the host
	// and the path.
	if !strings.HasPrefix(configPath, "/") {
		configPath = "/" + configPath
	}

	c.Source = fmt.Sprintf("https://%s%s", configHost, configPath)
	req, err := http.NewRequest("GET", c.Source, nil)
	if err != nil {
		return fmt.Errorf(
			"Error: Could not create request for configuration host %s: %v",
			configHost,
			err,
		)
	}

	req.Header.Set("User-Agent", utilities.UserAgent())

	resp, err := configClient.Do(req)
	if err != nil {
		return fmt.Errorf(
			"Error: could not connect to configuration host %s: %v",
			configHost,
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf(
			"Error: Configuration host %s returned %d for config request",
			configHost,
			resp.StatusCode,
		)
	}

	jsonConfig, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf(
			"Error: Could not read configuration content downloaded from %s: %v",
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

	return nil
}

func (c *Config) String() string {
	return fmt.Sprintf(
		"Version: %d\nSmall URL: %s\nLarge URL: %s\nUpload URL: %s\nEndpoint: %s\n",
		c.Version,
		c.Urls.SmallUrl,
		c.Urls.LargeUrl,
		c.Urls.UploadUrl,
		c.ConnectToAddr,
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
