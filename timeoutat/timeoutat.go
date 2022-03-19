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

package timeoutat

import (
	"context"
	"fmt"
	"time"
)

func TimeoutAt(
	ctx context.Context,
	when time.Time,
	debug bool,
) (response chan interface{}) {
	response = make(chan interface{})
	go func(ctx context.Context) {
		go func() {
			if debug {
				fmt.Printf("Timeout expected to end at %v\n", when)
			}
			select {
			case <-time.After(when.Sub(time.Now())):
			case <-ctx.Done():
			}
			response <- struct{}{}
			if debug {
				fmt.Printf("Timeout ended at %v\n", time.Now())
			}
		}()
	}(ctx)
	return
}
