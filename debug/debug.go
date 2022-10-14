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

package debug

type DebugLevel int8

const (
	NoDebug DebugLevel = iota
	Debug
	Warn
	Error
)

type DebugWithPrefix struct {
	Level  DebugLevel
	Prefix string
}

func NewDebugWithPrefix(level DebugLevel, prefix string) *DebugWithPrefix {
	return &DebugWithPrefix{Level: level, Prefix: prefix}
}

func (d *DebugWithPrefix) String() string {
	return d.Prefix
}

func IsDebug(level DebugLevel) bool {
	return level <= Debug
}

func IsWarn(level DebugLevel) bool {
	return level <= Warn
}

func IsError(level DebugLevel) bool {
	return level <= Error
}
