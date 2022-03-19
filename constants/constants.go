package constants

import "time"

var (
	// The initial number of connections on a LBC.
	StartingNumberOfLoadBearingConnections uint64 = 4
	// The number of intervals for which to account in a moving-average
	// calculation.
	MovingAverageIntervalCount int = 4
	// The number of intervals across which to consider a moving average stable.
	MovingAverageStabilitySpan int = 4
	// The number of connections to add to a LBC when unsaturated.
	AdditiveNumberOfLoadBearingConnections uint64 = 4
	// The cutoff of the percent difference that defines instability.
	InstabilityDelta float64 = 5

	// The amount of time that the client will cooldown if it is in debug mode.
	CooldownPeriod time.Duration = 4 * time.Second
	// The number of probes to send when calculating RTT.
	RPMProbeCount int = 5
	// The amount of time that we give ourselves to calculate the RPM.
	RPMCalculationTime time.Duration = 10 * time.Second

	// The default amount of time that a test will take to calculate the RPM.
	DefaultTestTime int = 20
	// The default port number to which to connect on the config host.
	DefaultPortNumber int = 4043
	// The default determination of whether to run in debug mode.
	DefaultDebug bool = false
	// The default URL for the config host.
	DefaultConfigHost string = "networkquality.example.com"
)
