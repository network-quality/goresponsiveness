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

package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/extendedstats"
	"github.com/network-quality/goresponsiveness/utilities"
)

type ProbeType int64

type ProbeConfiguration struct {
	ConnectToAddr      string
	URL                string
	Host               string
	InsecureSkipVerify bool
}

type ProbeDataPoint struct {
	Time           time.Time     `Description:"Time of the generation of the data point."                    Formatter:"Format"  FormatterArgument:"01-02-2006-15-04-05.000"`
	RoundTripCount uint64        `Description:"The number of round trips measured by this data point."`
	Duration       time.Duration `Description:"The duration for this measurement."                           Formatter:"Seconds"`
	TCPRtt         time.Duration `Description:"The underlying connection's RTT at probe time."               Formatter:"Seconds"`
	TCPCwnd        uint32        `Description:"The underlying connection's congestion window at probe time."`
	Type           ProbeType     `Description:"The type of the probe."                                       Formatter:"Value"`
}

const (
	SelfUp ProbeType = iota
	SelfDown
	Foreign
)

type ProbeRoundTripCountType uint16

const (
	DefaultDownRoundTripCount ProbeRoundTripCountType = 1
	SelfUpRoundTripCount      ProbeRoundTripCountType = 1
	SelfDownRoundTripCount    ProbeRoundTripCountType = 1
	ForeignRoundTripCount     ProbeRoundTripCountType = 3
)

func (pt ProbeType) Value() string {
	if pt == SelfUp {
		return "SelfUp"
	} else if pt == SelfDown {
		return "SelfDown"
	}
	return "Foreign"
}

func Probe(
	managingCtx context.Context,
	waitGroup *sync.WaitGroup,
	client *http.Client,
	probeUrl string,
	probeHost string, // optional: for use with a test_endpoint
	probeType ProbeType,
	result *chan ProbeDataPoint,
	captureExtendedStats bool,
	debugging *debug.DebugWithPrefix,
) error {

	if waitGroup != nil {
		waitGroup.Add(1)
		defer waitGroup.Done()
	}

	if client == nil {
		return fmt.Errorf("cannot start a probe with a nil client")
	}

	probeId := utilities.GenerateUniqueId()
	probeTracer := NewProbeTracer(client, probeType, probeId, debugging)
	time_before_probe := time.Now()
	probe_req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(managingCtx, probeTracer.trace),
		"GET",
		probeUrl,
		nil,
	)
	if err != nil {
		return err
	}

	// Used to disable compression
	probe_req.Header.Set("Accept-Encoding", "identity")
	probe_req.Header.Set("User-Agent", utilities.UserAgent())

	probe_resp, err := client.Do(probe_req)
	if err != nil {
		return err
	}

	// Header.Get returns "" when not set
	if probe_resp.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("Content-Encoding header was set (compression not allowed)")
	}

	// TODO: Make this interruptable somehow by using _ctx_.
	_, err = io.ReadAll(probe_resp.Body)
	if err != nil {
		return err
	}
	time_after_probe := time.Now()

	// Depending on whether we think that Close() requires another RTT (via TCP), we
	// may need to move this before/after capturing the after time.
	probe_resp.Body.Close()

	sanity := time_after_probe.Sub(time_before_probe)

	// When the probe is run on a load-generating connection (a self probe) there should
	// only be a single round trip that is measured. We will take the accumulation of all these
	// values just to be sure, though. Because of how this traced connection was launched, most
	// of the values will be 0 (or very small where the time that go takes for delivering callbacks
	// and doing context switches pokes through). When it is !isSelfProbe then the values will
	// be significant and we want to add them regardless!
	totalDelay := probeTracer.GetTLSAndHttpHeaderDelta() + probeTracer.GetHttpDownloadDelta(
		time_after_probe,
	) + probeTracer.GetTCPDelta()

	// We must have reused the connection if we are a self probe!
	if (probeType == SelfUp || probeType == SelfDown) && !probeTracer.stats.ConnectionReused {
		panic(!probeTracer.stats.ConnectionReused)
	}

	if debug.IsDebug(debugging.Level) {
		fmt.Printf(
			"(%s) (%s Probe %v) sanity vs total: %v vs %v\n",
			debugging.Prefix,
			probeType.Value(),
			probeId,
			sanity,
			totalDelay,
		)
	}
	roundTripCount := DefaultDownRoundTripCount
	if probeType == Foreign {
		roundTripCount = ForeignRoundTripCount
	}
	// Careful!!! It's possible that this channel has been closed because the Prober that
	// started it has been stopped. Writing to a closed channel will cause a panic. It might not
	// matter because a panic just stops the go thread containing the paniced code and we are in
	// a go thread that executes only this function.
	defer func() {
		isThreadPanicing := recover()
		if isThreadPanicing != nil && debug.IsDebug(debugging.Level) {
			fmt.Printf(
				"(%s) (%s Probe %v) Probe attempted to write to the result channel after its invoker ended (official reason: %v).\n",
				debugging.Prefix,
				probeType.Value(),
				probeId,
				isThreadPanicing,
			)
		}
	}()
	tcpRtt := time.Duration(0 * time.Second)
	tcpCwnd := uint32(0)
	// TODO: Only get the extended stats for a connection if the user has requested them overall.
	if captureExtendedStats && extendedstats.ExtendedStatsAvailable() {
		tcpInfo, err := extendedstats.GetTCPInfo(probeTracer.stats.ConnInfo.Conn)
		if err == nil {
			tcpRtt = time.Duration(tcpInfo.Rtt) * time.Microsecond
			tcpCwnd = tcpInfo.Snd_cwnd
		} else {
			fmt.Printf("Warning: Could not fetch the extended stats for a probe: %v\n", err)
		}
	}
	dataPoint := ProbeDataPoint{
		Time:           time_before_probe,
		RoundTripCount: uint64(roundTripCount),
		Duration:       totalDelay,
		TCPRtt:         tcpRtt,
		TCPCwnd:        tcpCwnd,
		Type:           probeType,
	}
	*result <- dataPoint
	return nil
}
