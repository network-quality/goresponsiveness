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

package ccw

import (
	"os"
	"sync"
)

type ConcurrentWriter struct {
	lock sync.Mutex
	file *os.File
}

func NewConcurrentFileWriter(file *os.File) *ConcurrentWriter {
	return &ConcurrentWriter{sync.Mutex{}, file}
}

func (ccw *ConcurrentWriter) Write(p []byte) (n int, err error) {
	ccw.lock.Lock()
	defer func() {
		ccw.file.Sync()
		ccw.lock.Unlock()
	}()
	return ccw.file.Write(p)
}
