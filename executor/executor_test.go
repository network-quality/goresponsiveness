package executor_test

import (
	"testing"
	"time"

	"github.com/network-quality/goresponsiveness/executor"
)

var countToFive = func() {
	time.Sleep(5 * time.Second)
}

var countToThree = func() {
	time.Sleep(3 * time.Second)
}

var executionUnits = []executor.ExecutionUnit{countToFive, countToThree}

func TestSerial(t *testing.T) {
	then := time.Now()
	waiter := executor.Execute(executor.Serial, executionUnits)
	waiter.Wait()
	when := time.Now()

	if when.Sub(then) < 7*time.Second {
		t.Fatalf("Execution did not happen serially -- the wait was too short: %v", when.Sub(then).Seconds())
	}
}

func TestParallel(t *testing.T) {
	then := time.Now()
	waiter := executor.Execute(executor.Parallel, executionUnits)
	waiter.Wait()
	when := time.Now()

	if when.Sub(then) > 6*time.Second {
		t.Fatalf("Execution did not happen in parallel -- the wait was too long: %v", when.Sub(then).Seconds())
	}
}

func TestExecutionMethodParallelToString(t *testing.T) {
	executionMethod := executor.Parallel

	if executionMethod.ToString() != "Parallel" {
		t.Fatalf("Incorrect result from ExecutionMethod.ToString; expected Parallel but got %v", executionMethod.ToString())
	}
}

func TestExecutionMethodSerialToString(t *testing.T) {
	executionMethod := executor.Serial

	if executionMethod.ToString() != "Serial" {
		t.Fatalf("Incorrect result from ExecutionMethod.ToString; expected Serial but got %v", executionMethod.ToString())
	}
}
