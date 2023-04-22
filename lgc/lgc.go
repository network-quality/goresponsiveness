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

package lgc

import (
	"context"
	"net/http"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
	"github.com/network-quality/goresponsiveness/stats"
)

type LoadGeneratingConnection interface {
	Start(context.Context, debug.DebugLevel) bool
	TransferredInInterval() (uint64, time.Duration)
	Client() *http.Client
	Status() LgcStatus
	ClientId() uint64
	Stats() *stats.TraceStats
	WaitUntilStarted(context.Context) bool
}

type LgcStatus int

const (
	LGC_STATUS_NOT_STARTED LgcStatus = iota
	LGC_STATUS_RUNNING
	LGC_STATUS_DONE
	LGC_STATUS_ERROR
)
