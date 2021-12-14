package utilities

func PercentDifference(current float64, previous float64) (difference float64) {
	return ((current - previous) / (float64(current+previous) / 2.0)) * float64(100)
}
