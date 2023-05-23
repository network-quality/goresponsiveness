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
	"fmt"
	"math/rand"
	"sync"
)

type LoadGeneratingConnectionCollection struct {
	Lock sync.Mutex
	LGCs *[]LoadGeneratingConnection
}

func NewLoadGeneratingConnectionCollection() LoadGeneratingConnectionCollection {
	return LoadGeneratingConnectionCollection{LGCs: new([]LoadGeneratingConnection)}
}

func (collection *LoadGeneratingConnectionCollection) Get(idx int) (*LoadGeneratingConnection, error) {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return nil, fmt.Errorf("collection is unlocked")
	}
	return collection.lockedGet(idx)
}

func (collection *LoadGeneratingConnectionCollection) lockedGet(idx int) (*LoadGeneratingConnection, error) {
	return &(*collection.LGCs)[idx], nil
}

func (collection *LoadGeneratingConnectionCollection) Append(conn LoadGeneratingConnection) error {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return fmt.Errorf("collection is unlocked")
	}
	*collection.LGCs = append(*collection.LGCs, conn)
	return nil
}

func (collection *LoadGeneratingConnectionCollection) Len() (int, error) {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return -1, fmt.Errorf("collection is unlocked")
	}
	return len(*collection.LGCs), nil
}

func (collection *LoadGeneratingConnectionCollection) GetRandom() (*LoadGeneratingConnection, error) {
	if collection.Lock.TryLock() {
		collection.Lock.Unlock()
		return nil, fmt.Errorf("collection is unlocked")
	}

	idx := int(rand.Uint32())
	idx = idx % len(*collection.LGCs)

	return collection.lockedGet(idx)
}
