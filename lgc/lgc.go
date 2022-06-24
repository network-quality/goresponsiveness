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
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/net/http2"
)

type LoadGeneratingConnection interface {
	Start(context.Context, debug.DebugLevel) bool
	TransferredInInterval() (uint64, time.Duration)
	Client() *http.Client
	IsValid() bool
	ClientId() uint64
	Stats() *stats.TraceStats
}

// TODO: All 64-bit fields that are accessed atomically must
// appear at the top of this struct.
type LoadGeneratingConnectionDownload struct {
	downloaded        uint64
	lastIntervalEnd   int64
	Path              string
	downloadStartTime time.Time
	lastDownloaded    uint64
	client            *http.Client
	debug             debug.DebugLevel
	valid             bool
	KeyLogger         io.Writer
	clientId          uint64
	tracer            *httptrace.ClientTrace
	stats             stats.TraceStats
}

func (lgd *LoadGeneratingConnectionDownload) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {
	lgd.stats.DnsStartTime = now
	lgd.stats.DnsStart = dnsStartInfo
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
	lgd.stats.DnsDoneTime = now
	lgd.stats.DnsDone = dnsDoneInfo
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"DNS Done for %v: %v\n",
			lgd.ClientId(),
			lgd.stats.DnsDone,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetConnectStartTime(
	now time.Time,
) {
	lgd.stats.ConnectStartTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"TCP Start for %v at %v\n",
			lgd.ClientId(),
			lgd.stats.ConnectStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {
	lgd.stats.ConnectDoneTime = now
	lgd.stats.ConnectDoneError = err
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"TCP Done for %v (with error %v) @ %v\n",
			lgd.ClientId(),
			lgd.stats.ConnectDoneError,
			lgd.stats.ConnectDoneTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetGetConnTime(now time.Time) {
	lgd.stats.GetConnectionStartTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started getting connection for %v @ %v\n",
			lgd.ClientId(),
			lgd.stats.GetConnectionStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	if gotConnInfo.Reused {
		fmt.Printf("Unexpectedly reusing a connection!\n")
		panic(!gotConnInfo.Reused)
	}
	lgd.stats.GetConnectionDoneTime = now
	lgd.stats.ConnInfo = gotConnInfo
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Got connection for %v at %v with info %v\n",
			lgd.ClientId(),
			lgd.stats.GetConnectionDoneTime,
			lgd.stats.ConnInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetTLSHandshakeStartTime(
	now time.Time,
) {
	lgd.stats.TLSStartTime = utilities.Some(now)
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started TLS Handshake for %v @ %v\n",
			lgd.ClientId(),
			lgd.stats.TLSStartTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
	lgd.stats.TLSDoneTime = utilities.Some(now)
	lgd.stats.TLSConnInfo = connectionState
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Completed TLS handshake for %v at %v with info %v\n",
			lgd.ClientId(),
			lgd.stats.TLSDoneTime,
			lgd.stats.TLSConnInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetHttpWroteRequestTimeInfo(
	now time.Time,
	info httptrace.WroteRequestInfo,
) {
	lgd.stats.HttpWroteRequestTime = now
	lgd.stats.HttpInfo = info
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"(lgd) Http finished writing request for %v at %v with info %v\n",
			lgd.ClientId(),
			lgd.stats.HttpWroteRequestTime,
			lgd.stats.HttpInfo,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) SetHttpResponseReadyTime(
	now time.Time,
) {
	lgd.stats.HttpResponseReadyTime = now
	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Got the first byte of HTTP response headers for %v at %v\n",
			lgd.ClientId(),
			lgd.stats.HttpResponseReadyTime,
		)
	}
}

func (lgd *LoadGeneratingConnectionDownload) ClientId() uint64 {
	return lgd.clientId
}

func (lgd *LoadGeneratingConnectionDownload) TransferredInInterval() (uint64, time.Duration) {
	transferred := atomic.SwapUint64(&lgd.downloaded, 0)
	newIntervalEnd := (time.Now().Sub(lgd.downloadStartTime)).Nanoseconds()
	previousIntervalEnd := atomic.SwapInt64(&lgd.lastIntervalEnd, newIntervalEnd)
	intervalLength := time.Duration(newIntervalEnd - previousIntervalEnd)
	if debug.IsDebug(lgd.debug) {
		fmt.Printf("download: Transferred: %v bytes in %v.\n", transferred, intervalLength)
	}
	return transferred, intervalLength
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
	lgd.clientId = utilities.GenerateConnectionId()
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

	//lgd.client = &http.Client{Transport: &transport}
	lgd.client = &http.Client{}
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
func (lgd *LoadGeneratingConnectionDownload) IsValid() bool {
	return lgd.valid
}

func (lgd *LoadGeneratingConnectionDownload) Stats() *stats.TraceStats {
	return &lgd.stats
}

func (lgd *LoadGeneratingConnectionDownload) doDownload(ctx context.Context) {
	var request *http.Request = nil
	var get *http.Response = nil
	var err error = nil

	if request, err = http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, lgd.tracer),
		"GET",
		lgd.Path,
		nil,
	); err != nil {
		lgd.valid = false
		return
	}

	lgd.downloadStartTime = time.Now()
	lgd.lastIntervalEnd = 0

	if get, err = lgd.client.Do(request); err != nil {
		lgd.valid = false
		return
	}
	cr := &countingReader{n: &lgd.downloaded, ctx: ctx, readable: get.Body}
	_, _ = io.Copy(ioutil.Discard, cr)
	get.Body.Close()
	if debug.IsDebug(lgd.debug) {
		fmt.Printf("Ending a load-generating download.\n")
	}
}

// TODO: All 64-bit fields that are accessed atomically must
// appear at the top of this struct.
type LoadGeneratingConnectionUpload struct {
	uploaded        uint64
	lastIntervalEnd int64
	Path            string
	uploadStartTime time.Time
	lastUploaded    uint64
	client          *http.Client
	debug           debug.DebugLevel
	valid           bool
	KeyLogger       io.Writer
	clientId        uint64
}

func (lgu *LoadGeneratingConnectionUpload) ClientId() uint64 {
	return lgu.clientId
}

func (lgd *LoadGeneratingConnectionUpload) TransferredInInterval() (uint64, time.Duration) {
	transferred := atomic.SwapUint64(&lgd.uploaded, 0)
	newIntervalEnd := (time.Now().Sub(lgd.uploadStartTime)).Nanoseconds()
	previousIntervalEnd := atomic.SwapInt64(&lgd.lastIntervalEnd, newIntervalEnd)
	intervalLength := time.Duration(newIntervalEnd - previousIntervalEnd)
	if debug.IsDebug(lgd.debug) {
		fmt.Printf("upload: Transferred: %v bytes in %v.\n", transferred, intervalLength)
	}
	return transferred, intervalLength
}

func (lgu *LoadGeneratingConnectionUpload) Client() *http.Client {
	return lgu.client
}

func (lgu *LoadGeneratingConnectionUpload) IsValid() bool {
	return lgu.valid
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
	/* This is a workaround. We are going to have to add streaming to the wasm go runtime.
	 * Rather than streaming the contents of the file to the server, tt attempts to read
	 * the entire file to be sent *before* it flushes a read for the first time!
	 */
	n = 512
	if len(p) < 512 {
		n = len(p)
	}
	if *s.n > 1024 {
		return 0, io.EOF
	}
	atomic.AddUint64(s.n, uint64(n))
	return
}

func (lgu *LoadGeneratingConnectionUpload) doUpload(ctx context.Context) bool {
	lgu.uploaded = 0
	s := &syntheticCountingReader{n: &lgu.uploaded, ctx: ctx}
	var resp *http.Response = nil
	var err error

	lgu.uploadStartTime = time.Now()
	lgu.lastIntervalEnd = 0

	if resp, err = lgu.client.Post(lgu.Path, "application/octet-stream", s); err != nil {
		fmt.Printf("Failed to start an upload post: %v\n", err)
		lgu.valid = false
		return false
	}
	resp.Body.Close()
	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Ending a load-generating upload.\n")
	}
	return true
}

func (lgu *LoadGeneratingConnectionUpload) Start(
	ctx context.Context,
	debugLevel debug.DebugLevel,
) bool {
	lgu.uploaded = 0
	lgu.clientId = utilities.GenerateConnectionId()
	lgu.debug = debugLevel

	// See above for the rationale of doing http2.Transport{} here
	// to ensure that we are using h2.
	transport := http2.Transport{}
	transport.TLSClientConfig = &tls.Config{}

	if !utilities.IsInterfaceNil(lgu.KeyLogger) {
		if debug.IsDebug(lgu.debug) {
			fmt.Printf(
				"Using an SSL Key Logger for this load-generating upload.\n",
			)
		}
		transport.TLSClientConfig.KeyLogWriter = lgu.KeyLogger
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	//lgu.client = &http.Client{Transport: &transport}
	lgu.client = &http.Client{}
	lgu.valid = true

	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Started a load-generating upload (id: %v).\n", lgu.clientId)
	}
	go lgu.doUpload(ctx)
	return true
}

func (lgu *LoadGeneratingConnectionUpload) Stats() *stats.TraceStats {
	// Get all your stats from the download side of the LGC.
	return nil
}
