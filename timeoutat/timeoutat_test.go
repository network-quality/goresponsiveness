package timeoutat

import (
	"context"
	"testing"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
)

func TestTimeoutAt(t *testing.T) {
	testTime := 5 * time.Second
	testTimeLimit := 6 * time.Second

	now := time.Now()
	select {
	case <-TimeoutAt(context.Background(), time.Now().Add(testTime), debug.NoDebug):

	}
	then := time.Now()

	actualTime := then.Sub(now)

	if actualTime >= testTimeLimit {
		t.Fatalf("Should have taken 5 seconds but it really took %v!", actualTime)
	}
}
