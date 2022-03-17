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

package utilities

import (
	"context"
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"time"
)

func IsInterfaceNil(ifc interface{}) bool {
	return ifc == nil || (reflect.ValueOf(ifc).Kind() == reflect.Ptr && reflect.ValueOf(ifc).IsNil())
}

func SignedPercentDifference(current float64, previous float64) (difference float64) {
	return ((current - previous) / (float64(current+previous) / 2.0)) * float64(100)
}
func AbsPercentDifference(current float64, previous float64) (difference float64) {
	return (math.Abs(current-previous) / (float64(current+previous) / 2.0)) * float64(100)
}

func Conditional(condition bool, t string, f string) string {
	if condition {
		return t
	}
	return f
}

type GetLatency struct {
	Delay time.Duration
	RTTs  uint16
	Err   error
}

func CalculateSequentialRTTsTime(ctx context.Context, client_a *http.Client, client_b *http.Client, url string) chan GetLatency {
	responseChannel := make(chan GetLatency)
	go func() {
		before := time.Now()
		c_a, err := client_a.Get(url)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, RTTs: 0, Err: err}
			return
		}
		// TODO: Make this interruptable somehow by using _ctx_.
		_, err = io.ReadAll(c_a.Body)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, RTTs: 0, Err: err}
			return
		}
		c_a.Body.Close()

		c_b, err := client_b.Get(url)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, RTTs: 0, Err: err}
			return
		}
		// TODO: Make this interruptable somehow by using _ctx_.
		_, err = io.ReadAll(c_b.Body)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, Err: err}
			return
		}
		c_b.Body.Close()
		responseChannel <- GetLatency{Delay: time.Since(before), RTTs: 10, Err: nil}
	}()
	return responseChannel
}

func SeekForAppend(file *os.File) (err error) {
	_, err = file.Seek(0, 2)
	return
}
