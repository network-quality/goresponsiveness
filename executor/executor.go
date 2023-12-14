package executor

import (
	"sync"
)

type ExecutionMethod int

const (
	Parallel ExecutionMethod = iota
	Serial
)

type ExecutionUnit func()

func (ep ExecutionMethod) ToString() string {
	switch ep {
	case Parallel:
		return "Parallel"
	case Serial:
		return "Serial"
	}
	return "Unrecognized execution method"
}

func Execute(executionMethod ExecutionMethod, executionUnits []ExecutionUnit) *sync.WaitGroup {
	waiter := &sync.WaitGroup{}

	// Make sure that we Add to the wait group all the execution units
	// before starting to run any -- there is a potential race condition
	// otherwise.
	(*waiter).Add(len(executionUnits))

	for _, executionUnit := range executionUnits {
		// Stupid capture in Go! Argh.
		executionUnit := executionUnit

		invoker := func() {
			executionUnit()
			(*waiter).Done()
		}
		switch executionMethod {
		case Parallel:
			go invoker()
		case Serial:
			invoker()
		default:
			panic("Invalid execution method value given.")
		}
	}

	return waiter
}
