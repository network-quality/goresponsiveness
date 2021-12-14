package timeoutat

import (
	"context"
	"fmt"
	"time"
)

type TimeoutAt struct {
	when     time.Time
	response chan interface{}
}

func NewTimeoutAt(ctx context.Context, when time.Time, response chan interface{}) *TimeoutAt {
	timeoutAt := &TimeoutAt{when: when, response: response}
	timeoutAt.start(ctx)
	return timeoutAt
}

func (ta *TimeoutAt) start(ctx context.Context) {
	go func() {
		fmt.Printf("Timeout expected to end at %v\n", ta.when)
		select {
		case <-time.After(ta.when.Sub(time.Now())):
		case <-ctx.Done():
		}
		ta.response <- struct{}{}
		fmt.Printf("Timeout ended at %v\n", time.Now())
	}()
}
