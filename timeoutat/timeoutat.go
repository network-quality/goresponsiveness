package timeoutat

import (
	"context"
	"fmt"
	"time"
)

func TimeoutAt(ctx context.Context, when time.Time, debug bool) (response chan interface{}) {
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
