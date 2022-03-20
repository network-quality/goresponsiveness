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

package lgc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

var chunkSize int = 5000

type LoadGeneratingConnection interface {
	Start(context.Context, bool) bool
	Transferred() uint64
	Client() *http.Client
	IsValid() bool
}

type LoadGeneratingConnectionDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
	debug      bool
	valid      bool
	KeyLogger  io.Writer
}

func (lbd *LoadGeneratingConnectionDownload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lbd.downloaded)
	if lbd.debug {
		fmt.Printf("download: Transferred: %v\n", transferred)
	}
	return transferred
}

func (lbd *LoadGeneratingConnectionDownload) Client() *http.Client {
	return lbd.client
}

type countingReader struct {
	n        *uint64
	ctx      context.Context
	readable io.Reader
}

func (cr *countingReader) Read(p []byte) (n int, err error) {
	if cr.ctx.Err() != nil {
		return 0, io.EOF
	}
	n, err = cr.readable.Read(p)
	atomic.AddUint64(cr.n, uint64(n))
	return
}

func (lbd *LoadGeneratingConnectionDownload) Start(
	ctx context.Context,
	debug bool,
) bool {
	lbd.downloaded = 0
	transport := http2.Transport{}

	if !utilities.IsInterfaceNil(lbd.KeyLogger) {
		if debug {
			fmt.Printf(
				"Using an SSL Key Logger for this load-generating download.\n",
			)
		}

		// The presence of a custom TLSClientConfig in a *generic* `transport`
		// means that go will default to HTTP/1.1 and cowardly avoid HTTP/2:
		// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L278
		// Also, it would appear that the API's choice of HTTP vs HTTP2 can
		// depend on whether the url contains
		// https:// or http://:
		// https://github.com/golang/go/blob/7ca6902c171b336d98adbb103d701a013229c806/src/net/http/transport.go#L74
		transport.TLSClientConfig = &tls.Config{
			KeyLogWriter:       lbd.KeyLogger,
			InsecureSkipVerify: true,
		}
	}

	lbd.client = &http.Client{Transport: &transport}
	lbd.debug = debug
	lbd.valid = true

	if debug {
		fmt.Printf("Started a load-generating download.\n")
	}
	go lbd.doDownload(ctx)
	return true
}
func (lbd *LoadGeneratingConnectionDownload) IsValid() bool {
	return lbd.valid
}

func (lbd *LoadGeneratingConnectionDownload) doDownload(ctx context.Context) {
	get, err := lbd.client.Get(lbd.Path)
	if err != nil {
		lbd.valid = false
		return
	}
	cr := &countingReader{n: &lbd.downloaded, ctx: ctx, readable: get.Body}
	_, _ = io.Copy(ioutil.Discard, cr)
	get.Body.Close()
	if lbd.debug {
		fmt.Printf("Ending a load-generating download.\n")
	}
}

type LoadGeneratingConnectionUpload struct {
	Path      string
	uploaded  uint64
	client    *http.Client
	debug     bool
	valid     bool
	KeyLogger io.Writer
}

func (lbu *LoadGeneratingConnectionUpload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lbu.uploaded)
	if lbu.debug {
		fmt.Printf("upload: Transferred: %v\n", transferred)
	}
	return transferred
}

func (lbu *LoadGeneratingConnectionUpload) Client() *http.Client {
	return lbu.client
}

func (lbu *LoadGeneratingConnectionUpload) IsValid() bool {
	return lbu.valid
}

type syntheticCountingReader struct {
	n   *uint64
	ctx context.Context
}

func (s *syntheticCountingReader) Read(p []byte) (n int, err error) {
	if s.ctx.Err() != nil {
		return 0, io.EOF
	}
	err = nil
	n = len(p)

	atomic.AddUint64(s.n, uint64(n))
	return
}

func (lbu *LoadGeneratingConnectionUpload) doUpload(ctx context.Context) bool {
	lbu.uploaded = 0
	s := &syntheticCountingReader{n: &lbu.uploaded, ctx: ctx}
	var resp *http.Response = nil
	var err error

	if resp, err = lbu.client.Post(lbu.Path, "application/octet-stream", s); err != nil {
		lbu.valid = false
	}
	resp.Body.Close()
	if lbu.debug {
		fmt.Printf("Ending a load-generating upload.\n")
	}
	return true
}

func (lbu *LoadGeneratingConnectionUpload) Start(
	ctx context.Context,
	debug bool,
) bool {
	lbu.uploaded = 0

	// See above for the rationale of doing http2.Transport{} here
	// to ensure that we are using h2.
	transport := http2.Transport{}

	if !utilities.IsInterfaceNil(lbu.KeyLogger) {
		if debug {
			fmt.Printf(
				"Using an SSL Key Logger for this load-generating upload.\n",
			)
		}
		transport.TLSClientConfig = &tls.Config{
			KeyLogWriter:       lbu.KeyLogger,
			InsecureSkipVerify: true,
		}
	}

	lbu.client = &http.Client{Transport: &transport}
	lbu.debug = debug
	lbu.valid = true

	if debug {
		fmt.Printf("Started a load-generating upload.\n")
	}
	go lbu.doUpload(ctx)
	return true
}
