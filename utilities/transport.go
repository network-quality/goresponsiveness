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

package utilities

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

func OverrideHostTransport(transport *http.Transport, connectToAddr string) {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		if len(connectToAddr) > 0 {
			addr = net.JoinHostPort(connectToAddr, port)
		}

		return dialer.DialContext(ctx, network, addr)
	}

	if t2, err := http2.ConfigureTransports(transport); err != nil {
		panic("Panic: Could not properly upgrade an http/1.1 transport for http/2.")
	} else {
		t2.StrictMaxConcurrentStreams = true
	}
}
