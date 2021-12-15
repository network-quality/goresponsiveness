package utilities

import (
	"context"
	"io"
	"math"
	"net/http"
	"time"
)

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
	Err   error
}

func TimedSequentialGets(ctx context.Context, client_a *http.Client, client_b *http.Client, url string) chan GetLatency {
	responseChannel := make(chan GetLatency)
	go func() {
		before := time.Now()
		c_a, err := client_a.Get(url)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, Err: err}
		}
		// TODO: Make this interruptable somehow by using _ctx_.
		_, err = io.ReadAll(c_a.Body)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, Err: err}
		}
		c_b, err := client_b.Get(url)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, Err: err}
		}
		// TODO: Make this interruptable somehow by using _ctx_.
		_, err = io.ReadAll(c_b.Body)
		if err != nil {
			responseChannel <- GetLatency{Delay: 0, Err: err}
		}
		responseChannel <- GetLatency{Delay: time.Now().Sub(before), Err: nil}
	}()
	return responseChannel
}

type RandZeroSource struct{}

func (RandZeroSource) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = 0
	}

	return len(b), nil
}
