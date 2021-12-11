package saturating

type SaturatingInt struct {
	max       int
	value     int
	saturated bool
}

func NewSaturatingInt(max int) *SaturatingInt {
	return &SaturatingInt{max: max, value: 0, saturated: false}
}

func (s *SaturatingInt) Value() int {
	if s.saturated {
		return s.max
	}
	return s.value
}

func (s *SaturatingInt) Add(operand int) {
	if !s.saturated {
		s.value += operand
		if s.value >= s.max {
			s.saturated = true
		}
	}
}
