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
package series

import (
	"fmt"
	"sync"

	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type WindowSeriesDuration int

const (
	Forever    WindowSeriesDuration = iota
	WindowOnly WindowSeriesDuration = iota
)

type WindowSeries[Data any, Bucket constraints.Ordered] interface {
	fmt.Stringer

	Reserve(b Bucket) error
	Fill(b Bucket, d Data) error

	Count() (some int, none int)

	ForEach(func(Bucket, *utilities.Optional[Data]))

	GetValues() []utilities.Optional[Data]
	Complete() bool
	GetType() WindowSeriesDuration
}

type windowSeriesWindowOnlyImpl[Data any, Bucket constraints.Ordered] struct {
	windowSize  int
	data        []utilities.Pair[Bucket, utilities.Optional[Data]]
	latestIndex int
	empty       bool
}

/*
 * Beginning of WindowSeries interface methods.
 */

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Reserve(b Bucket) error {
	if !wsi.empty && b <= wsi.data[wsi.latestIndex].First {
		return fmt.Errorf("reserving must be monotonically increasing")
	}

	if wsi.empty {
		/* Special case if we are empty: The latestIndex is where we want this value to go! */
		wsi.data[wsi.latestIndex] = utilities.Pair[Bucket, utilities.Optional[Data]]{
			First: b, Second: utilities.None[Data](),
		}
	} else {
		/* Otherwise, bump ourselves forward and place the new reservation there. */
		wsi.latestIndex = wsi.nextIndex(wsi.latestIndex)
		wsi.data[wsi.latestIndex] = utilities.Pair[Bucket, utilities.Optional[Data]]{
			First: b, Second: utilities.None[Data](),
		}
	}
	wsi.empty = false
	return nil
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Fill(b Bucket, d Data) error {
	iterator := wsi.latestIndex
	for {
		if wsi.data[iterator].First == b {
			wsi.data[iterator].Second = utilities.Some[Data](d)
			return nil
		}
		iterator = wsi.nextIndex(iterator)
		if iterator == wsi.latestIndex {
			break
		}
	}
	return fmt.Errorf("attempting to fill a bucket that does not exist")
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Count() (some int, none int) {
	some = 0
	none = 0
	for _, v := range wsi.data {
		if utilities.IsSome[Data](v.Second) {
			some++
		} else {
			none++
		}
	}
	return
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Complete() bool {
	for _, v := range wsi.data {
		if utilities.IsNone(v.Second) {
			return false
		}
	}
	return true
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) nextIndex(currentIndex int) int {
	return (currentIndex + 1) % wsi.windowSize
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) previousIndex(currentIndex int) int {
	nextIndex := currentIndex - 1
	if nextIndex < 0 {
		nextIndex += wsi.windowSize
	}
	return nextIndex
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) toArray() []utilities.Optional[Data] {
	result := make([]utilities.Optional[Data], wsi.windowSize)

	iterator := wsi.latestIndex
	parallelIterator := 0
	for {
		result[parallelIterator] = wsi.data[iterator].Second
		iterator = wsi.previousIndex(iterator)
		parallelIterator++
		if iterator == wsi.latestIndex {
			break
		}
	}
	return result
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) GetValues() []utilities.Optional[Data] {
	return wsi.toArray()
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) GetType() WindowSeriesDuration {
	return WindowOnly
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) ForEach(eacher func(b Bucket, d *utilities.Optional[Data])) {
	for _, v := range wsi.data {
		eacher(v.First, &v.Second)
	}
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) String() string {
	result := fmt.Sprintf("Window series (window (%d) only, latest index: %v): ", wsi.windowSize, wsi.latestIndex)
	for _, v := range wsi.data {
		valueString := "None"
		if utilities.IsSome[Data](v.Second) {
			valueString = fmt.Sprintf("%v", utilities.GetSome[Data](v.Second))
		}
		result += fmt.Sprintf("%v: %v; ", v.First, valueString)
	}
	return result
}

func newWindowSeriesWindowOnlyImpl[Data any, Bucket constraints.Ordered](
	windowSize int,
) *windowSeriesWindowOnlyImpl[Data, Bucket] {
	result := windowSeriesWindowOnlyImpl[Data, Bucket]{windowSize: windowSize, latestIndex: 0, empty: true}

	result.data = make([]utilities.Pair[Bucket, utilities.Optional[Data]], windowSize)

	return &result
}

/*
 * End of WindowSeries interface methods.
 */

type windowSeriesForeverImpl[Data any, Bucket constraints.Ordered] struct {
	data  []utilities.Pair[Bucket, utilities.Optional[Data]]
	empty bool
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Reserve(b Bucket) error {
	if !wsi.empty && b <= wsi.data[len(wsi.data)-1].First {
		return fmt.Errorf("reserving must be monotonically increasing")
	}

	wsi.empty = false
	wsi.data = append(wsi.data, utilities.Pair[Bucket, utilities.Optional[Data]]{First: b, Second: utilities.None[Data]()})
	return nil
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Fill(b Bucket, d Data) error {
	for i := range wsi.data {
		if wsi.data[i].First == b {
			wsi.data[i].Second = utilities.Some[Data](d)
			return nil
		}
	}
	return fmt.Errorf("attempting to fill a bucket that does not exist")
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) GetValues() []utilities.Optional[Data] {
	result := make([]utilities.Optional[Data], len(wsi.data))

	for i, v := range utilities.Reverse(wsi.data) {
		result[i] = v.Second
	}

	return result
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Count() (some int, none int) {
	some = 0
	none = 0
	for _, v := range wsi.data {
		if utilities.IsSome[Data](v.Second) {
			some++
		} else {
			none++
		}
	}
	return
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Complete() bool {
	for _, v := range wsi.data {
		if utilities.IsNone(v.Second) {
			return false
		}
	}
	return true
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) GetType() WindowSeriesDuration {
	return Forever
}

func newWindowSeriesForeverImpl[Data any, Bucket constraints.Ordered]() *windowSeriesForeverImpl[Data, Bucket] {
	result := windowSeriesForeverImpl[Data, Bucket]{empty: true}

	result.data = nil

	return &result
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) ForEach(eacher func(b Bucket, d *utilities.Optional[Data])) {
	for _, v := range wsi.data {
		eacher(v.First, &v.Second)
	}
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) String() string {
	result := "Window series (forever): "
	for _, v := range wsi.data {
		valueString := "None"
		if utilities.IsSome[Data](v.Second) {
			valueString = fmt.Sprintf("%v", utilities.GetSome[Data](v.Second))
		}
		result += fmt.Sprintf("%v: %v; ", v.First, valueString)
	}
	return result
}

/*
 * End of WindowSeries interface methods.
 */

func NewWindowSeries[Data any, Bucket constraints.Ordered](tipe WindowSeriesDuration, windowSize int) WindowSeries[Data, Bucket] {
	if tipe == WindowOnly {
		return newWindowSeriesWindowOnlyImpl[Data, Bucket](windowSize)
	} else if tipe == Forever {
		return newWindowSeriesForeverImpl[Data, Bucket]()
	}
	panic("")
}

type NumericBucketGenerator[T utilities.Number] struct {
	mt           sync.Mutex
	currentValue T
}

func (bg *NumericBucketGenerator[T]) Generate() T {
	bg.mt.Lock()
	defer bg.mt.Unlock()

	bg.currentValue++
	return bg.currentValue
}

func NewNumericBucketGenerator[T utilities.Number](initialValue T) NumericBucketGenerator[T] {
	return NumericBucketGenerator[T]{currentValue: initialValue}
}
