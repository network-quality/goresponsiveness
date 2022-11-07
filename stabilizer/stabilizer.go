package stabilizer

type Stabilizer[T any] interface {
	AddMeasurement(T)
	IsStable() bool
}
