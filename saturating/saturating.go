/*
 * This file is part of Go Responsiveness.
 *
 * Go Responsiveness is free software: you can redistribute it and/or modify it under
 * the terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 2 of the License, or (at your option) any later version.
 * Go Responsiveness is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with Go Responsiveness. If not, see <https://www.gnu.org/licenses/>.
 */

package saturating

type Saturatable interface {
	~uint | ~uint32 | ~uint64
}

type Saturating[T Saturatable] struct {
	max       T
	value     T
	saturated bool
}

func NewSaturating[T Saturatable](max T) *Saturating[T] {
	return &Saturating[T]{max: max, value: 0, saturated: false}
}

func (s *Saturating[T]) Value() T {
	if s.saturated {
		return s.max
	}
	return s.value
}

func (s *Saturating[T]) Add(operand T) {
	if !s.saturated {
		s.value += operand
		if s.value >= s.max {
			s.saturated = true
		}
	}
}
