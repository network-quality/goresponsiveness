/*
 * This file is part of Go Responsiveness.
 *
 * Go Responsiveness is free software: you can redistribute it and/or modify it under
 * the terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 * Go Responsiveness is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with Go Responsiveness. If not, see <https://www.gnu.org/licenses/>.
 */

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
