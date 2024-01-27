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
	"cmp"
	"fmt"
	"slices"
	"sync"

	"github.com/network-quality/goresponsiveness/utilities"
)

type WindowSeriesDuration int

const (
	Forever    WindowSeriesDuration = iota
	WindowOnly WindowSeriesDuration = iota
)

type WindowSeries[Data any, Bucket utilities.Number] interface {
	fmt.Stringer

	Reserve(b Bucket) error
	Fill(b Bucket, d Data) error

	Count() (some int, none int)

	ForEach(func(Bucket, *utilities.Optional[Data]))

	GetValues() []utilities.Optional[Data]
	Complete() bool
	GetType() WindowSeriesDuration

	ExtractBoundedSeries() WindowSeries[Data, Bucket]

	Append(appended *WindowSeries[Data, Bucket])
	BoundedAppend(appended *WindowSeries[Data, Bucket])

	GetBucketBounds() (Bucket, Bucket)

	SetTrimmingBucketBounds(Bucket, Bucket)
	ResetTrimmingBucketBounds()
}

type windowSeriesWindowOnlyImpl[Data any, Bucket utilities.Number] struct {
	windowSize         int
	data               []utilities.Pair[Bucket, utilities.Optional[Data]]
	latestIndex        int // invariant: newest data is there.
	empty              bool
	lock               sync.RWMutex
	lowerTrimmingBound utilities.Optional[Bucket]
	upperTrimmingBound utilities.Optional[Bucket]
}

/*
 * Beginning of WindowSeries interface methods.
 */

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Reserve(b Bucket) error {
	if !wsi.empty && b <= wsi.data[wsi.latestIndex].First {
		return fmt.Errorf("reserving must be monotonically increasing")
	}
	wsi.lock.Lock()
	defer wsi.lock.Unlock()

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

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) BucketBounds() (Bucket, Bucket) {
	newestBucket := wsi.data[wsi.latestIndex].First
	oldestBucket := wsi.data[wsi.nextIndex(wsi.latestIndex)].First

	return oldestBucket, newestBucket
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Fill(b Bucket, d Data) error {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	for _, v := range wsi.data {
		if utilities.IsNone(v.Second) {
			return false
		}
	}
	return true
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) nextIndex(currentIndex int) int {
	// Internal functions should be called with the lock held!
	if wsi.lock.TryLock() {
		panic("windowSeriesWindowOnlyImpl nextIndex called without lock held.")
	}
	return (currentIndex + 1) % wsi.windowSize
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) previousIndex(currentIndex int) int {
	// Internal functions should be called with the lock held!
	if wsi.lock.TryLock() {
		panic("windowSeriesWindowOnlyImpl nextIndex called without lock held.")
	}
	nextIndex := currentIndex - 1
	if nextIndex < 0 {
		nextIndex += wsi.windowSize
	}
	return nextIndex
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) toArray() []utilities.Optional[Data] {
	// Internal functions should be called with the lock held!
	if wsi.lock.TryLock() {
		panic("windowSeriesWindowOnlyImpl nextIndex called without lock held.")
	}
	result := make([]utilities.Optional[Data], wsi.windowSize)

	var lowerTrimmingBound, upperTrimmingBound Bucket
	hasBounds := false
	if utilities.IsSome(wsi.lowerTrimmingBound) {
		hasBounds = true
		lowerTrimmingBound = utilities.GetSome(wsi.lowerTrimmingBound)
		upperTrimmingBound = utilities.GetSome(wsi.upperTrimmingBound)
		result = make([]utilities.Optional[Data],
			int(upperTrimmingBound-lowerTrimmingBound)+1)
	}

	if wsi.empty {
		return result
	}
	iterator := wsi.latestIndex
	parallelIterator := 0
	for {
		if !hasBounds || (lowerTrimmingBound <= wsi.data[iterator].First &&
			wsi.data[iterator].First <= upperTrimmingBound) {
			result[parallelIterator] = wsi.data[iterator].Second
			parallelIterator++
		}
		iterator = wsi.previousIndex(iterator)
		if iterator == wsi.latestIndex {
			break
		}
	}
	return result
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) ExtractBoundedSeries() WindowSeries[Data, Bucket] {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()

	result := NewWindowSeries[Data, Bucket](WindowOnly, wsi.windowSize)

	var lowerTrimmingBound, upperTrimmingBound Bucket
	hasBounds := false
	if utilities.IsSome(wsi.lowerTrimmingBound) {
		hasBounds = true
		lowerTrimmingBound = utilities.GetSome(wsi.lowerTrimmingBound)
		upperTrimmingBound = utilities.GetSome(wsi.upperTrimmingBound)
	}

	for _, v := range wsi.data {
		if hasBounds && (v.First < lowerTrimmingBound || v.First > upperTrimmingBound) {
			continue
		}
		result.Reserve(v.First)
		if utilities.IsSome(v.Second) {
			result.Fill(v.First, utilities.GetSome(v.Second))
		}
	}
	return result
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) GetValues() []utilities.Optional[Data] {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	return wsi.toArray()
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) GetType() WindowSeriesDuration {
	return WindowOnly
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) ForEach(eacher func(b Bucket, d *utilities.Optional[Data])) {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	for _, v := range wsi.data {
		eacher(v.First, &v.Second)
	}
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) String() string {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) SetTrimmingBucketBounds(lower Bucket, upper Bucket) {
	wsi.lowerTrimmingBound = utilities.Some[Bucket](lower)
	wsi.upperTrimmingBound = utilities.Some[Bucket](upper)
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) ResetTrimmingBucketBounds() {
	wsi.lowerTrimmingBound = utilities.None[Bucket]()
	wsi.upperTrimmingBound = utilities.None[Bucket]()
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) GetBucketBounds() (Bucket, Bucket) {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	return wsi.data[wsi.nextIndex(wsi.latestIndex)].First, wsi.data[wsi.latestIndex].First
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) Append(appended *WindowSeries[Data, Bucket]) {
	panic("Append is unimplemented on a window-only Window Series")
}

func (wsi *windowSeriesWindowOnlyImpl[Data, Bucket]) BoundedAppend(appended *WindowSeries[Data, Bucket]) {
	panic("BoundedAppend is unimplemented on a window-only Window Series")
}

func newWindowSeriesWindowOnlyImpl[Data any, Bucket utilities.Number](
	windowSize int,
) *windowSeriesWindowOnlyImpl[Data, Bucket] {
	result := windowSeriesWindowOnlyImpl[Data, Bucket]{windowSize: windowSize, latestIndex: 0, empty: true}

	result.data = make([]utilities.Pair[Bucket, utilities.Optional[Data]], windowSize)

	return &result
}

/*
 * End of WindowSeries interface methods.
 */

type windowSeriesForeverImpl[Data any, Bucket utilities.Number] struct {
	data               []utilities.Pair[Bucket, utilities.Optional[Data]]
	empty              bool
	lock               sync.RWMutex
	lowerTrimmingBound utilities.Optional[Bucket]
	upperTrimmingBound utilities.Optional[Bucket]
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Reserve(b Bucket) error {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	if !wsi.empty && b <= wsi.data[len(wsi.data)-1].First {
		fmt.Printf("reserving must be monotonically increasing: %v vs %v", b, wsi.data[len(wsi.data)-1].First)
		return fmt.Errorf("reserving must be monotonically increasing")
	}

	wsi.empty = false
	wsi.data = append(wsi.data, utilities.Pair[Bucket, utilities.Optional[Data]]{First: b, Second: utilities.None[Data]()})
	return nil
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Fill(b Bucket, d Data) error {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	for i := range wsi.data {
		if wsi.data[i].First == b {
			wsi.data[i].Second = utilities.Some[Data](d)
			return nil
		}
	}
	return fmt.Errorf("attempting to fill a bucket that does not exist")
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) ExtractBoundedSeries() WindowSeries[Data, Bucket] {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()

	var lowerTrimmingBound, upperTrimmingBound Bucket
	hasBounds := false
	if utilities.IsSome(wsi.lowerTrimmingBound) {
		hasBounds = true
		lowerTrimmingBound = utilities.GetSome(wsi.lowerTrimmingBound)
		upperTrimmingBound = utilities.GetSome(wsi.upperTrimmingBound)
	}

	result := NewWindowSeries[Data, Bucket](Forever, 0)

	for _, v := range wsi.data {
		if hasBounds && (v.First < lowerTrimmingBound || v.First > upperTrimmingBound) {
			continue
		}
		result.Reserve(v.First)
		if utilities.IsSome(v.Second) {
			result.Fill(v.First, utilities.GetSome(v.Second))
		}
	}
	return result
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) GetValues() []utilities.Optional[Data] {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	result := make([]utilities.Optional[Data], len(wsi.data))

	var lowerTrimmingBound, upperTrimmingBound Bucket
	hasBounds := false
	if utilities.IsSome(wsi.lowerTrimmingBound) {
		hasBounds = true
		lowerTrimmingBound = utilities.GetSome(wsi.lowerTrimmingBound)
		upperTrimmingBound = utilities.GetSome(wsi.upperTrimmingBound)
		result = make([]utilities.Optional[Data],
			int(upperTrimmingBound-lowerTrimmingBound)+1)
	}

	index := 0
	for _, v := range wsi.data {
		if !hasBounds || (lowerTrimmingBound <= v.First && v.First <= upperTrimmingBound) {
			result[index] = v.Second
			index++
		}
	}

	return result
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Count() (some int, none int) {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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

func newWindowSeriesForeverImpl[Data any, Bucket utilities.Number]() *windowSeriesForeverImpl[Data, Bucket] {
	result := windowSeriesForeverImpl[Data, Bucket]{
		empty:              true,
		lowerTrimmingBound: utilities.None[Bucket](),
		upperTrimmingBound: utilities.None[Bucket](),
	}

	result.data = nil

	return &result
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) ForEach(eacher func(b Bucket, d *utilities.Optional[Data])) {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	for _, v := range wsi.data {
		eacher(v.First, &v.Second)
	}
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) String() string {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
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

func (wsi *windowSeriesForeverImpl[Data, Bucket]) SetTrimmingBucketBounds(lower Bucket, upper Bucket) {
	wsi.lowerTrimmingBound = utilities.Some[Bucket](lower)
	wsi.upperTrimmingBound = utilities.Some[Bucket](upper)
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) ResetTrimmingBucketBounds() {
	wsi.lowerTrimmingBound = utilities.None[Bucket]()
	wsi.upperTrimmingBound = utilities.None[Bucket]()
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) GetBucketBounds() (Bucket, Bucket) {
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	if wsi.empty {
		return 0, 0
	}
	return wsi.data[0].First, wsi.data[len(wsi.data)-1].First
}

// Sort the data according to the bucket ids (ascending)
func (wsi *windowSeriesForeverImpl[Data, Bucket]) sort() {
	slices.SortFunc(wsi.data, func(left, right utilities.Pair[Bucket, utilities.Optional[Data]]) int {
		return cmp.Compare(left.First, right.First)
	})
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) Append(appended *WindowSeries[Data, Bucket]) {
	result, ok := (*appended).(*windowSeriesForeverImpl[Data, Bucket])
	if !ok {
		panic("Cannot merge a forever window series with a non-forever window series.")
	}
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	result.lock.Lock()
	defer result.lock.Unlock()

	wsi.data = append(wsi.data, result.data...)
	// Because the series that we are appending in may have overlapping buckets,
	// we will sort them to maintain the invariant that the data items are sorted
	// by bucket ids (increasing).
	wsi.sort()
}

func (wsi *windowSeriesForeverImpl[Data, Bucket]) BoundedAppend(appended *WindowSeries[Data, Bucket]) {
	result, ok := (*appended).(*windowSeriesForeverImpl[Data, Bucket])
	if !ok {
		panic("Cannot merge a forever window series with a non-forever window series.")
	}
	wsi.lock.Lock()
	defer wsi.lock.Unlock()
	result.lock.Lock()
	defer result.lock.Unlock()

	if utilities.IsNone(result.lowerTrimmingBound) ||
		utilities.IsNone(result.upperTrimmingBound) {
		wsi.sort()
		wsi.data = append(wsi.data, result.data...)
		return
	} else {
		lowerTrimmingBound := utilities.GetSome(result.lowerTrimmingBound)
		upperTrimmingBound := utilities.GetSome(result.upperTrimmingBound)

		toAppend := utilities.Filter(result.data, func(
			element utilities.Pair[Bucket, utilities.Optional[Data]],
		) bool {
			bucket := element.First
			return lowerTrimmingBound <= bucket && bucket <= upperTrimmingBound
		})
		wsi.data = append(wsi.data, toAppend...)
	}
	// Because the series that we are appending in may have overlapping buckets,
	// we will sort them to maintain the invariant that the data items are sorted
	// by bucket ids (increasing).
	wsi.sort()
}

/*
 * End of WindowSeries interface methods.
 */

func NewWindowSeries[Data any, Bucket utilities.Number](tipe WindowSeriesDuration, windowSize int) WindowSeries[Data, Bucket] {
	if tipe == WindowOnly {
		return newWindowSeriesWindowOnlyImpl[Data, Bucket](windowSize)
	} else if tipe == Forever {
		return newWindowSeriesForeverImpl[Data, Bucket]()
	}
	panic("Attempting to create a new window series with an invalid type.")
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
