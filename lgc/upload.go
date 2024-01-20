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
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/l4s"
	"github.com/network-quality/goresponsiveness/stats"
	"github.com/network-quality/goresponsiveness/traceable"
	"github.com/network-quality/goresponsiveness/utilities"
)

// TODO: All 64-bit fields that are accessed atomically must
// appear at the top of this struct.
type LoadGeneratingConnectionUpload struct {
	uploaded           uint64
	lastIntervalEnd    int64
	URL                string
	ConnectToAddr      string
	uploadStartTime    time.Time
	client             *http.Client
	debug              debug.DebugLevel
	InsecureSkipVerify bool
	KeyLogger          io.Writer
	clientId           uint64
	tracer             *httptrace.ClientTrace
	stats              stats.TraceStats
	status             LgcStatus
	congestionControl  *string
	statusLock         *sync.Mutex
	statusWaiter       *sync.Cond
}

func NewLoadGeneratingConnectionUpload(url string, keyLogger io.Writer, connectToAddr string,
	insecureSkipVerify bool, congestionControl *string,
) LoadGeneratingConnectionUpload {
	lgu := LoadGeneratingConnectionUpload{
		URL:                url,
		KeyLogger:          keyLogger,
		ConnectToAddr:      connectToAddr,
		InsecureSkipVerify: insecureSkipVerify,
		congestionControl:  congestionControl,
		statusLock:         &sync.Mutex{},
	}
	lgu.status = LGC_STATUS_NOT_STARTED
	lgu.statusWaiter = sync.NewCond(lgu.statusLock)
	return lgu
}

func (lgu *LoadGeneratingConnectionUpload) WaitUntilStarted(ctxt context.Context) bool {
	conditional := func() bool { return lgu.status != LGC_STATUS_NOT_STARTED }
	go utilities.ContextSignaler(ctxt, 500*time.Millisecond, &conditional, lgu.statusWaiter)
	return utilities.WaitWithContext(ctxt, &conditional, lgu.statusLock, lgu.statusWaiter)
}

func (lgu *LoadGeneratingConnectionUpload) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {
	lgu.stats.DnsStartTime = now
	lgu.stats.DnsStart = dnsStartInfo
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"DNS Start for %v: %v\n",
			lgu.ClientId(),
			dnsStartInfo,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetDnsDoneTimeInfo(
	now time.Time,
	dnsDoneInfo httptrace.DNSDoneInfo,
) {
	lgu.stats.DnsDoneTime = now
	lgu.stats.DnsDone = dnsDoneInfo
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"DNS Done for %v: %v\n",
			lgu.ClientId(),
			lgu.stats.DnsDone,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetConnectStartTime(
	now time.Time,
) {
	lgu.stats.ConnectStartTime = now
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"TCP Start for %v at %v\n",
			lgu.ClientId(),
			lgu.stats.ConnectStartTime,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {
	lgu.stats.ConnectDoneTime = now
	lgu.stats.ConnectDoneError = err
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"TCP Done for %v (with error %v) @ %v\n",
			lgu.ClientId(),
			lgu.stats.ConnectDoneError,
			lgu.stats.ConnectDoneTime,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetGetConnTime(now time.Time) {
	lgu.stats.GetConnectionStartTime = now
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"Started getting connection for %v @ %v\n",
			lgu.ClientId(),
			lgu.stats.GetConnectionStartTime,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	if gotConnInfo.Reused {
		fmt.Printf("Unexpectedly reusing a connection!\n")
		panic(!gotConnInfo.Reused)
	}
	lgu.stats.GetConnectionDoneTime = now
	lgu.stats.ConnInfo = gotConnInfo
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"Got connection for %v at %v with info %v\n",
			lgu.ClientId(),
			lgu.stats.GetConnectionDoneTime,
			lgu.stats.ConnInfo,
		)
	}

	if lgu.congestionControl != nil {
		if debug.IsDebug(lgu.debug) {
			fmt.Printf(
				"Attempting to set congestion control algorithm to %v for connection %v at %v with info %v\n",
				*lgu.congestionControl,
				lgu.ClientId(),
				lgu.stats.GetConnectionDoneTime,
				lgu.stats.ConnInfo,
			)
		}
		if err := l4s.SetL4S(lgu.stats.ConnInfo.Conn, lgu.congestionControl); err != nil {
			fmt.Fprintf(
				os.Stderr,
				"Error setting L4S for %v at %v: %v\n",
				lgu.ClientId(),
				lgu.stats.GetConnectionDoneTime,
				err.Error(),
			)
		}
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetTLSHandshakeStartTime(
	now time.Time,
) {
	lgu.stats.TLSStartTime = utilities.Some(now)
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"Started TLS Handshake for %v @ %v\n",
			lgu.ClientId(),
			lgu.stats.TLSStartTime,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
	lgu.stats.TLSDoneTime = utilities.Some(now)
	lgu.stats.TLSConnInfo = connectionState
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"Completed TLS handshake for %v at %v with info %v\n",
			lgu.ClientId(),
			lgu.stats.TLSDoneTime,
			lgu.stats.TLSConnInfo,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetHttpWroteRequestTimeInfo(
	now time.Time,
	info httptrace.WroteRequestInfo,
) {
	lgu.stats.HttpWroteRequestTime = now
	lgu.stats.HttpInfo = info
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"(lgu) Http finished writing request for %v at %v with info %v\n",
			lgu.ClientId(),
			lgu.stats.HttpWroteRequestTime,
			lgu.stats.HttpInfo,
		)
	}
}

func (lgu *LoadGeneratingConnectionUpload) SetHttpResponseReadyTime(
	now time.Time,
) {
	lgu.stats.HttpResponseReadyTime = now
	if debug.IsDebug(lgu.debug) {
		fmt.Printf(
			"Got the first byte of HTTP response headers for %v at %v\n",
			lgu.ClientId(),
			lgu.stats.HttpResponseReadyTime,
		)
	}
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

func (lgu *LoadGeneratingConnectionUpload) Status() LgcStatus {
	return lgu.status
}

func (lgd *LoadGeneratingConnectionUpload) Direction() LgcDirection {
	return LGC_UP
}

type syntheticCountingReader struct {
	n   *uint64
	ctx context.Context
	lgu *LoadGeneratingConnectionUpload
}

func (s *syntheticCountingReader) Read(p []byte) (n int, err error) {
	if s.ctx.Err() != nil {
		return 0, io.EOF
	}
	if *s.n == 0 {
		s.lgu.statusLock.Lock()
		s.lgu.status = LGC_STATUS_RUNNING
		s.lgu.statusWaiter.Broadcast()
		s.lgu.statusLock.Unlock()
	}
	err = nil
	n = len(p)

	atomic.AddUint64(s.n, uint64(n))
	return
}

func (lgu *LoadGeneratingConnectionUpload) doUpload(ctx context.Context) error {
	lgu.uploaded = 0
	s := &syntheticCountingReader{n: &lgu.uploaded, ctx: ctx, lgu: lgu}
	var resp *http.Response = nil
	var request *http.Request = nil
	var err error

	if request, err = http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, lgu.tracer),
		"POST",
		lgu.URL,
		s,
	); err != nil {
		lgu.statusLock.Lock()
		lgu.status = LGC_STATUS_ERROR
		lgu.statusWaiter.Broadcast()
		lgu.statusLock.Unlock()
		return err
	}

	// Used to disable compression
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", utilities.UserAgent())

	lgu.uploadStartTime = time.Now()
	lgu.lastIntervalEnd = 0

	if resp, err = lgu.client.Do(request); err != nil {
		lgu.statusLock.Lock()
		lgu.status = LGC_STATUS_ERROR
		lgu.statusWaiter.Broadcast()
		lgu.statusLock.Unlock()
		return err
	}

	lgu.statusLock.Lock()
	lgu.status = LGC_STATUS_DONE
	lgu.statusWaiter.Broadcast()
	lgu.statusLock.Unlock()

	resp.Body.Close()
	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Ending a load-generating upload.\n")
	}
	return nil
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
	lgu.tracer = traceable.GenerateHttpTimingTracer(lgu, lgu.debug)

	if debug.IsDebug(lgu.debug) {
		fmt.Printf("Started a load-generating upload (id: %v).\n", lgu.clientId)
	}

	go lgu.doUpload(parentCtx)
	return true
}

func (lgu *LoadGeneratingConnectionUpload) Stats() *stats.TraceStats {
	return &lgu.stats
}
