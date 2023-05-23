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

package rpm

import (
	"strings"
	"testing"
)

func TestSpecParametersFromArgumentsBadTimeout(t *testing.T) {
	_, err := SpecParametersFromArguments(0, 0, 0, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("0 timeout improperly allowed.")
	}
	_, err = SpecParametersFromArguments(-1, 0, 0, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("negative timeout improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadMad(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 0, 0, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "moving-average") {
		t.Fatalf("0 mad improperly allowed.")
	}
	_, err = SpecParametersFromArguments(1, 0, 0, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "moving-average") {
		t.Fatalf("negative mad improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadId(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 1, 0, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "reevaluation") {
		t.Fatalf("0 id improperly allowed.")
	}
	_, err = SpecParametersFromArguments(1, 1, -1, 0, 0, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "reevaluation") {
		t.Fatalf("negative id improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadSdt(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 1, 1, 1, -1, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "deviation") {
		t.Fatalf("0 sdt improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadMnp(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 1, 1, 1, 1, 0, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "parallel") {
		t.Fatalf("0 mnp improperly allowed.")
	}
	_, err = SpecParametersFromArguments(1, 1, 1, 1, 1, -1, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "parallel") {
		t.Fatalf("negative mnp improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadMps(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 1, 1, 1, 1, 1, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "probing interval") {
		t.Fatalf("0 mps improperly allowed.")
	}
	_, err = SpecParametersFromArguments(1, 1, 1, 1, 1, 1, -1, 0)
	if err == nil || !strings.Contains(err.Error(), "probing interval") {
		t.Fatalf("negative mps improperly allowed.")
	}
}

func TestSpecParametersFromArgumentsBadPtc(t *testing.T) {
	_, err := SpecParametersFromArguments(1, 1, 1, 1, 1, 1, 1, 0)
	if err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("0 ptc improperly allowed.")
	}
	_, err = SpecParametersFromArguments(1, 1, 1, 1, 1, 1, 1, -1)
	if err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("negative ptc improperly allowed.")
	}
}
