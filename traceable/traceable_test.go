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

package traceable

import (
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
)

type CountingTraceable struct {
	Counter *uint64
}

func (lgd *CountingTraceable) SetDnsStartTimeInfo(
	now time.Time,
	dnsStartInfo httptrace.DNSStartInfo,
) {

}

func (lgd *CountingTraceable) SetDnsDoneTimeInfo(
	now time.Time,
	dnsDoneInfo httptrace.DNSDoneInfo,
) {

}

func (lgd *CountingTraceable) SetConnectStartTime(
	now time.Time,
) {

}

func (lgd *CountingTraceable) SetConnectDoneTimeError(
	now time.Time,
	err error,
) {

}

func (lgd *CountingTraceable) SetGetConnTime(now time.Time) {
}

func (lgd *CountingTraceable) SetGotConnTimeInfo(
	now time.Time,
	gotConnInfo httptrace.GotConnInfo,
) {
	atomic.AddUint64(lgd.Counter, 1)
}

func (lgd *CountingTraceable) SetTLSHandshakeStartTime(
	now time.Time,
) {
}

func (lgd *CountingTraceable) SetTLSHandshakeDoneTimeState(
	now time.Time,
	connectionState tls.ConnectionState,
) {
}

func (lgd *CountingTraceable) SetHttpWroteRequestTimeInfo(
	now time.Time,
	info httptrace.WroteRequestInfo,
) {
}

func (lgd *CountingTraceable) SetHttpResponseReadyTime(
	now time.Time,
) {
}

// Ensure that two different http client request tracers started with the same context
// do not compose and receive each other's callbacks.
func TestDuplicativeTraceables(t *testing.T) {

	singleCtx, singleCtxCancel_ := context.WithCancel(context.Background())
	defer singleCtxCancel_()

	client := http.Client{}

	counterA := new(uint64)
	countingTracerA := CountingTraceable{Counter: counterA}
	counterB := new(uint64)
	countingTracerB := CountingTraceable{Counter: counterB}

	debugging := debug.NewDebugWithPrefix(debug.Debug, "TestDuplicativeTraceables")
	request_a, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(
			singleCtx,
			GenerateHttpTimingTracer(&countingTracerA, debugging.Level),
		),
		"GET",
		"https://www.google.com/",
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create request A to GET google.com")
	}
	request_b, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(
			singleCtx,
			GenerateHttpTimingTracer(&countingTracerB, debugging.Level),
		),
		"GET",
		"https://www.google.com/",
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create request B to GET google.com")
	}

	requestWg := new(sync.WaitGroup)

	requestWg.Add(2)
	go func() {
		defer requestWg.Done()
		get, err := client.Do(request_a)
		if err != nil {
			return
		}
		_, _ = io.Copy(ioutil.Discard, get.Body)
		get.Body.Close()
	}()
	go func() {
		defer requestWg.Done()
		get, err := client.Do(request_b)
		if err != nil {
			return
		}
		_, _ = io.Copy(ioutil.Discard, get.Body)
		get.Body.Close()
	}()

	requestWg.Wait()

	if !(*counterA == 1 && *counterB == 1) {
		t.Fatalf(
			"Two separate tracers received overlapping callbacks on each other's http requests: (counterA: %d, counterB: %d)",
			*counterA,
			*counterB,
		)
	}

}
