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
	"fmt"
	"math"
	"math/rand"
	"os"
	"reflect"
	"sync/atomic"
	"time"
)

func IsInterfaceNil(ifc interface{}) bool {
	return ifc == nil ||
		(reflect.ValueOf(ifc).Kind() == reflect.Ptr && reflect.ValueOf(ifc).IsNil())
}

func SignedPercentDifference(
	current float64,
	previous float64,
) (difference float64) {
	//return ((current - previous) / (float64(current+previous) / 2.0)) * float64(
	//100,
	//	)
	return ((current - previous) / previous) * float64(
		100,
	)
}

func AbsPercentDifference(
	current float64,
	previous float64,
) (difference float64) {
	return (math.Abs(current-previous) / (float64(current+previous) / 2.0)) * float64(
		100,
	)
}

func Conditional(condition bool, t string, f string) string {
	if condition {
		return t
	}
	return f
}

func ToMbps(bytes float64) float64 {
	return ToMBps(bytes) * float64(8)
}

func ToMBps(bytes float64) float64 {
	return float64(bytes) / float64(1024*1024)
}

type MeasurementResult struct {
	Delay            time.Duration
	MeasurementCount uint16
	Err              error
}

func SeekForAppend(file *os.File) (err error) {
	_, err = file.Seek(0, 2)
	return
}

var GenerateConnectionId func() uint64 = func() func() uint64 {
	var nextConnectionId uint64 = 0
	return func() uint64 {
		return atomic.AddUint64(&nextConnectionId, 1)
	}
}()

type Optional[S any] struct {
	value S
	some  bool
}

func Some[S any](value S) Optional[S] {
	return Optional[S]{value: value, some: true}
}

func None[S any]() Optional[S] {
	return Optional[S]{some: false}
}

func IsNone[S any](optional Optional[S]) bool {
	return !optional.some
}

func IsSome[S any](optional Optional[S]) bool {
	return optional.some
}

func GetSome[S any](optional Optional[S]) S {
	if !optional.some {
		panic("Attempting to access Some of a None.")
	}
	return optional.value
}

func (optional Optional[S]) String() string {
	if IsSome(optional) {
		return fmt.Sprintf("Some: %v", optional.some)
	} else {
		return "None"
	}
}

func RandBetween(max int) int {
	return rand.New(rand.NewSource(int64(time.Now().Nanosecond()))).Int() % max
}
