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

package lbc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
)

var chunkSize int = 5000

type LoadBearingConnection interface {
	Start(context.Context, bool) bool
	Transferred() uint64
	Client() *http.Client
	IsValid() bool
}

type LoadBearingConnectionDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
	debug      bool
	valid      bool
}

func (lbd *LoadBearingConnectionDownload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lbd.downloaded)
	if lbd.debug {
		fmt.Printf("download: Transferred: %v\n", transferred)
	}
	return transferred
}

func (lbd *LoadBearingConnectionDownload) Client() *http.Client {
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

func (lbd *LoadBearingConnectionDownload) Start(ctx context.Context, debug bool) bool {
	lbd.downloaded = 0
	transport := http.Transport{}
	lbd.client = &http.Client{Transport: &transport}
	lbd.debug = debug
	lbd.valid = true

	// At some point this might be useful: It is a snippet of code that will enable
	// logging of per-session TLS key material in order to make debugging easier in
	// Wireshark.
	/*
		lbd.client = &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					KeyLogWriter: w,

					Rand:               utilities.RandZeroSource{}, // for reproducible output; don't do this.
					InsecureSkipVerify: true,                       // test server certificate is not trusted.
				},
			},
		}
	*/

	if debug {
		fmt.Printf("Started a load-bearing download.\n")
	}
	go lbd.doDownload(ctx)
	return true
}
func (lbd *LoadBearingConnectionDownload) IsValid() bool {
	return lbd.valid
}

func (lbd *LoadBearingConnectionDownload) doDownload(ctx context.Context) {
	get, err := lbd.client.Get(lbd.Path)
	if err != nil {
		lbd.valid = false
		return
	}
	cr := &countingReader{n: &lbd.downloaded, ctx: ctx, readable: get.Body}
	_, _ = io.Copy(ioutil.Discard, cr)
	lbd.valid = false
	get.Body.Close()
	if lbd.debug {
		fmt.Printf("Ending a load-bearing download.\n")
	}
}

type LoadBearingConnectionUpload struct {
	Path     string
	uploaded uint64
	client   *http.Client
	debug    bool
	valid    bool
}

func (lbu *LoadBearingConnectionUpload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lbu.uploaded)
	if lbu.debug {
		fmt.Printf("upload: Transferred: %v\n", transferred)
	}
	return transferred
}

func (lbu *LoadBearingConnectionUpload) Client() *http.Client {
	return lbu.client
}

func (lbu *LoadBearingConnectionUpload) IsValid() bool {
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

func (lbu *LoadBearingConnectionUpload) doUpload(ctx context.Context) bool {
	lbu.uploaded = 0
	s := &syntheticCountingReader{n: &lbu.uploaded, ctx: ctx}
	if resp, err := lbu.client.Post(lbu.Path, "application/octet-stream", s); err == nil {
		resp.Body.Close()
	}
	lbu.valid = false
	if lbu.debug {
		fmt.Printf("Ending a load-bearing upload.\n")
	}
	return true
}

func (lbu *LoadBearingConnectionUpload) Start(ctx context.Context, debug bool) bool {
	lbu.uploaded = 0
	transport := http.Transport{}
	lbu.client = &http.Client{Transport: &transport}
	lbu.debug = debug
	lbu.valid = true

	if debug {
		fmt.Printf("Started a load-bearing upload.\n")
	}
	go lbu.doUpload(ctx)
	return true
}
