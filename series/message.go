package series

import (
	"github.com/network-quality/goresponsiveness/utilities"
	"golang.org/x/exp/constraints"
)

type SeriesMessageType int

const (
	SeriesMessageReserve SeriesMessageType = iota
	SeriesMessageMeasure SeriesMessageType = iota
)

type SeriesMessage[Data any, BucketType constraints.Ordered] struct {
	Type    SeriesMessageType
	Bucket  BucketType
	Measure utilities.Optional[Data]
}
