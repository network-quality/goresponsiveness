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
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

var GenerateConnectionId func() uint64 = func() func() uint64 {
	var nextConnectionId uint64 = 0
	return func() uint64 {
		return atomic.AddUint64(&nextConnectionId, 1)
	}
}()

type LoadGeneratingConnection interface {
	Start(context.Context, debug.DebugLevel) bool
	Transferred() uint64
	Client() *http.Client
	IsValid() bool
	ClientId() uint64
}

type LoadGeneratingConnectionStats struct {
	dnsStart                  httptrace.DNSStartInfo
	dnsDone                   httptrace.DNSDoneInfo
	connInfo                  httptrace.GotConnInfo
	httpInfo                  httptrace.WroteRequestInfo
	tlsConnInfo               tls.ConnectionState
	connectDoneError          error
	dnsStartTime              time.Time
	dnsDoneTime               time.Time
	tlsStartTime              time.Time
	tlsCompleteTime           time.Time
	connectStartTime          time.Time
	connectDoneTime           time.Time
	getConnectionStartTime    time.Time
	getConnectionCompleteTime time.Time
}

type LoadGeneratingConnectionDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
	debug      debug.DebugLevel
	valid      bool
	KeyLogger  io.Writer
	clientId   uint64
	tracer     *httptrace.ClientTrace
	stats      LoadGeneratingConnectionStats
}

func (lgd *LoadGeneratingConnectionDownload) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {
	lgd.stats.dnsStartTime = now
	lgd.stats.dnsStart = dnsStartInfo
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"DNS Start for %v: %v\n",
			lgd.ClientId(),
			dnsStartInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetDnsDoneTimeInfo(
	now time.Time,
	dnsDoneInfo httptrace.DNSDoneInfo,
) {
	lgd.stats.dnsDoneTime = now
	lgd.stats.dnsDone = dnsDoneInfo
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"DNS Done for %v: %v\n",
			lgd.ClientId(),
			lgd.stats.dnsDone,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetConnectStartTime(
	now time.Time,
) {
	lgd.stats.connectStartTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"TCP Start for %v at %v\n",
			lgd.ClientId(),
			lgd.stats.connectStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {
	lgd.stats.connectDoneTime = now
	lgd.stats.connectDoneError = err
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"TCP Done for %v (with error %v) @ %v\n",
			lgd.ClientId(),
			lgd.stats.connectDoneError,
			lgd.stats.connectDoneTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetGetConnTime(now time.Time) {
	lgd.stats.getConnectionStartTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started getting connection for %v @ %v\n",
			lgd.ClientId(),
			lgd.stats.getConnectionStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	lgd.stats.getConnectionCompleteTime = now
	lgd.stats.connInfo = gotConnInfo
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Got connection for %v at %v with info %v\n",
			lgd.ClientId(),
			lgd.stats.getConnectionCompleteTime,
			lgd.stats.connInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetTLSHandshakeStartTime(
	now time.Time,
) {
	lgd.stats.tlsStartTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started TLS Handshake for %v @ %v\n",
			lgd.ClientId(),
			lgd.stats.tlsStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
	lgd.stats.tlsCompleteTime = now
	lgd.stats.tlsConnInfo = connectionState
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Completed TLS handshake for %v at %v with info %v\n",
			lgd.ClientId(),
			lgd.stats.tlsCompleteTime,
			lgd.stats.tlsConnInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) ClientId() uint64 {
	return lgd.clientId
}

func (lgd *LoadGeneratingConnectionDownload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lgd.downloaded)
	if debug.IsDebug(lgd.debug) {
		fmt.Printf("download: Transferred: %v\n", transferred)
	}
	return transferred
}

func (lgd *LoadGeneratingConnectionDownload) Client() *http.Client {
	return lgd.client
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

func (lgd *LoadGeneratingConnectionDownload) Start(
	ctx context.Context,
	debugLevel debug.DebugLevel,
) bool {
	lgd.downloaded = 0
	lgd.clientId = GenerateConnectionId()
	transport := http2.Transport{}
	transport.TLSClientConfig = &tls.Config{}

	if !utilities.IsInterfaceNil(lgd.KeyLogger) {
		if debug.IsDebug(lgd.debug) {
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
		transport.TLSClientConfig.KeyLogWriter = lgd.KeyLogger
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	lgd.client = &http.Client{Transport: &transport}
	lgd.debug = debugLevel
	lgd.valid = true
	lgd.tracer = traceable.GenerateHttpTimingTracer(lgd, lgd.debug)

	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started a load-generating download (id: %v).\n",
			lgd.clientId,
		)
	}
	go lgd.doDownload(ctx)
	return true
}
func (lbd *LoadGeneratingConnectionDownload) IsValid() bool {
	return lbd.valid
}

func (lbd *LoadGeneratingConnectionDownload) doDownload(ctx context.Context) {
	var request *http.Request = nil
	var get *http.Response = nil
	var err error = nil

	if request, err = http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, lbd.tracer),
		"GET",
		lbd.Path,
		nil,
	); err != nil {
		lbd.valid = false
		return
	}

	if get, err = lbd.client.Do(request); err != nil {
		lbd.valid = false
		return
	}
	cr := &countingReader{n: &lbd.downloaded, ctx: ctx, readable: get.Body}
	_, _ = io.Copy(ioutil.Discard, cr)
	get.Body.Close()
	if debug.IsDebug(lbd.debug) {
		fmt.Printf("Ending a load-generating download.\n")
	}
}

type LoadGeneratingConnectionUpload struct {
	Path      string
	uploaded  uint64
	client    *http.Client
	debug     debug.DebugLevel
	valid     bool
	KeyLogger io.Writer
	clientId  uint64
}

func (lbu *LoadGeneratingConnectionUpload) ClientId() uint64 {
	return lbu.clientId
}

func (lbu *LoadGeneratingConnectionUpload) Transferred() uint64 {
	transferred := atomic.LoadUint64(&lbu.uploaded)
	if debug.IsDebug(lbu.debug) {
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
		return false
	}
	resp.Body.Close()
	if debug.IsDebug(lbu.debug) {
		fmt.Printf("Ending a load-generating upload.\n")
	}
	return true
}

func (lbu *LoadGeneratingConnectionUpload) Start(
	ctx context.Context,
	debugLevel debug.DebugLevel,
) bool {
	lbu.uploaded = 0
	lbu.clientId = GenerateConnectionId()
	lbu.debug = debugLevel

	// See above for the rationale of doing http2.Transport{} here
	// to ensure that we are using h2.
	transport := http2.Transport{}
	transport.TLSClientConfig = &tls.Config{}

	if !utilities.IsInterfaceNil(lbu.KeyLogger) {
		if debug.IsDebug(lbu.debug) {
			fmt.Printf(
				"Using an SSL Key Logger for this load-generating upload.\n",
			)
		}
		transport.TLSClientConfig.KeyLogWriter = lbu.KeyLogger
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	lbu.client = &http.Client{Transport: &transport}
	lbu.valid = true

	if debug.IsDebug(lbu.debug) {
		fmt.Printf("Started a load-generating upload (id: %v).\n", lbu.clientId)
	}
	go lbu.doUpload(ctx)
	return true
}
