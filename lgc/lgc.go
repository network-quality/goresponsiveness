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

package lgc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
)

type LoadGeneratingConnection interface {
	Start(context.Context, debug.DebugLevel) bool
	TransferredInInterval() (uint64, time.Duration)
	Client() *http.Client
	IsValid() bool
	ClientId() uint64
	Stats() *stats.TraceStats
	WaitUntilStarted(context.Context) bool
}

type LoadGeneratingConnectionCollection struct {
	Lock sync.Mutex
	LGCs *[]LoadGeneratingConnection
}

func NewLoadGeneratingConnectionCollection() LoadGeneratingConnectionCollection {
	return LoadGeneratingConnectionCollection{LGCs: new([]LoadGeneratingConnection)}
}

func (collection *LoadGeneratingConnectionCollection) Get(idx int) (*LoadGeneratingConnection, error) {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return nil, fmt.Errorf("collection is unlocked")
	}

	if idx > len(*collection.LGCs) {
		return nil, fmt.Errorf("index too large")
	}
	return &(*collection.LGCs)[idx], nil
}

func (collection *LoadGeneratingConnectionCollection) Append(conn LoadGeneratingConnection) error {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return fmt.Errorf("collection is unlocked")
	}
	*collection.LGCs = append(*collection.LGCs, conn)
	return nil
}

// TODO: All 64-bit fields that are accessed atomically must
// appear at the top of this struct.
type LoadGeneratingConnectionDownload struct {
	downloaded         uint64
	lastIntervalEnd    int64
	ConnectToAddr      string
	URL                string
	downloadStartTime  time.Time
	lastDownloaded     uint64
	client             *http.Client
	debug              debug.DebugLevel
	valid              bool
	validLock          *sync.Mutex
	InsecureSkipVerify bool
	KeyLogger          io.Writer
	clientId           uint64
	tracer             *httptrace.ClientTrace
	stats              stats.TraceStats
	validWaiter        *sync.Cond
}

func NewLoadGeneratingConnectionDownload(url string, keyLogger io.Writer, connectToAddr string, insecureSkipVerify bool) LoadGeneratingConnectionDownload {
	lgd := LoadGeneratingConnectionDownload{
		URL:                url,
		KeyLogger:          keyLogger,
		ConnectToAddr:      connectToAddr,
		InsecureSkipVerify: insecureSkipVerify,
		validLock:          &sync.Mutex{},
	}
	lgd.validWaiter = sync.NewCond(lgd.validLock)
	return lgd
}

func (lgd *LoadGeneratingConnectionDownload) WaitUntilStarted(ctxt context.Context) bool {
	conditional := func() bool { return lgd.valid }
	go utilities.ContextSignaler(ctxt, 500*time.Millisecond, &conditional, lgd.validWaiter)
	return utilities.WaitWithContext(ctxt, &conditional, lgd.validLock, lgd.validWaiter)
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
	parentCtx context.Context,
	debugLevel debug.DebugLevel,
) bool {
	lgd.downloaded = 0
	lgd.debug = debugLevel
	lgd.clientId = utilities.GenerateUniqueId()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: lgd.InsecureSkipVerify,
		},
	}

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
	transport.TLSClientConfig.InsecureSkipVerify = lgd.InsecureSkipVerify

	utilities.OverrideHostTransport(transport, lgd.ConnectToAddr)

	lgd.client = &http.Client{Transport: transport}
	lgd.validLock.Lock()
	lgd.valid = true
	lgd.validLock.Unlock()
	lgd.tracer = traceable.GenerateHttpTimingTracer(lgd, lgd.debug)

	if debug.IsDebug(lgd.debug) {
		fmt.Printf(
			"Started a load-generating download (id: %v).\n",
			lgd.clientId,
		)
	}

	go lgd.doDownload(parentCtx)
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
		lgd.URL,
		nil,
	); err != nil {
		lgd.validLock.Lock()
		lgd.valid = false
		lgd.validLock.Unlock()
		return
	}

	// Used to disable compression
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", utilities.UserAgent())

	lgd.downloadStartTime = time.Now()
	lgd.lastIntervalEnd = 0

	if get, err = lgd.client.Do(request); err != nil {
		lgd.validLock.Lock()
		lgd.valid = false
		lgd.validLock.Unlock()
		return
	}

	// Header.Get returns "" when not set
	if get.Header.Get("Content-Encoding") != "" {
		lgd.validLock.Lock()
		lgd.valid = false
		lgd.validLock.Unlock()
		fmt.Printf("Content-Encoding header was set (compression not allowed)")
		return
	}
	cr := &countingReader{n: &lgd.downloaded, ctx: ctx, readable: get.Body}
	_, _ = io.Copy(io.Discard, cr)
	get.Body.Close()
	if debug.IsDebug(lgd.debug) {
		fmt.Printf("Ending a load-generating download.\n")
	}
}

// TODO: All 64-bit fields that are accessed atomically must
// appear at the top of this struct.
type LoadGeneratingConnectionUpload struct {
	uploaded           uint64
	lastIntervalEnd    int64
	URL                string
	ConnectToAddr      string
	uploadStartTime    time.Time
	lastUploaded       uint64
	client             *http.Client
	debug              debug.DebugLevel
	valid              bool
	validLock          *sync.Mutex
	InsecureSkipVerify bool
	KeyLogger          io.Writer
	clientId           uint64
	validWaiter        *sync.Cond
}

func NewLoadGeneratingConnectionUpload(url string, keyLogger io.Writer, connectToAddr string, insecureSkipVerify bool) LoadGeneratingConnectionUpload {
	lgu := LoadGeneratingConnectionUpload{
		URL:                url,
		KeyLogger:          keyLogger,
		ConnectToAddr:      connectToAddr,
		InsecureSkipVerify: insecureSkipVerify,
		validLock:          &sync.Mutex{},
	}
	lgu.validWaiter = sync.NewCond(lgu.validLock)
	return lgu
}

func (lgu *LoadGeneratingConnectionUpload) WaitUntilStarted(ctxt context.Context) bool {
	conditional := func() bool { return lgu.valid }
	go utilities.ContextSignaler(ctxt, 500*time.Millisecond, &conditional, lgu.validWaiter)
	return utilities.WaitWithContext(ctxt, &conditional, lgu.validLock, lgu.validWaiter)
}

func (lgu *LoadGeneratingConnectionUpload) ClientId() uint64 {
	return lgu.clientId
}

func (lgu *LoadGeneratingConnectionUpload) TransferredInInterval() (uint64, time.Duration) {
	transferred := atomic.SwapUint64(&lgu.uploaded, 0)
	newIntervalEnd := (time.Now().Sub(lgu.uploadStartTime)).Nanoseconds()
	previousIntervalEnd := atomic.SwapInt64(&lgu.lastIntervalEnd, newIntervalEnd)
	intervalLength := time.Duration(newIntervalEnd - previousIntervalEnd)
	if debug.IsDebug(lgu.debug) {
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
	n = len(p)

	atomic.AddUint64(s.n, uint64(n))
	return
}

func (lgu *LoadGeneratingConnectionUpload) doUpload(ctx context.Context) bool {
	lgu.uploaded = 0
	s := &syntheticCountingReader{n: &lgu.uploaded, ctx: ctx}
	var resp *http.Response = nil
	var request *http.Request = nil
	var err error

	if request, err = http.NewRequest(
		"POST",
		lgu.URL,
		s,
	); err != nil {
		lgu.validLock.Lock()
		lgu.valid = false
		lgu.validLock.Unlock()
		return false
	}

	// Used to disable compression
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", utilities.UserAgent())

	lgu.uploadStartTime = time.Now()
	lgu.lastIntervalEnd = 0

	if resp, err = lgu.client.Do(request); err != nil {
		lgu.validLock.Lock()
		lgu.valid = false
		lgu.validLock.Unlock()
		return false
	}

	resp.Body.Close()
	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Ending a load-generating upload.\n")
	}
	return true
}

func (lgu *LoadGeneratingConnectionUpload) Start(
	parentCtx context.Context,
	debugLevel debug.DebugLevel,
) bool {
	lgu.uploaded = 0
	lgu.clientId = utilities.GenerateUniqueId()
	lgu.debug = debugLevel

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: lgu.InsecureSkipVerify,
		},
	}

	if !utilities.IsInterfaceNil(lgu.KeyLogger) {
		if debug.IsDebug(lgu.debug) {
			fmt.Printf(
				"Using an SSL Key Logger for this load-generating upload.\n",
			)
		}
		transport.TLSClientConfig.KeyLogWriter = lgu.KeyLogger
	}

	utilities.OverrideHostTransport(transport, lgu.ConnectToAddr)

	lgu.client = &http.Client{Transport: transport}

	lgu.validLock.Lock()
	lgu.valid = true
	lgu.validLock.Unlock()

	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Started a load-generating upload (id: %v).\n", lgu.clientId)
	}

	go lgu.doUpload(parentCtx)
	return true
}

func (lgu *LoadGeneratingConnectionUpload) Stats() *stats.TraceStats {
	// Get all your stats from the download side of the LGC.
	return nil
}
