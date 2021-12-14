package utilities

import "math"

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
