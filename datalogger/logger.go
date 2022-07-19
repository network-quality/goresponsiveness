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

package datalogger

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
)

var (
	JSONSpaces int = 4
)

type DataLogger[T any] interface {
	LogRecord(record T)
	Export() bool
	Close() bool
}

type CSVDataLogger[T any] struct {
	mut         *sync.Mutex
	recordCount int
	data        []T
	isOpen      bool
	destination io.WriteCloser
}

type JSONDataLogger[T any] struct {
	mut         *sync.Mutex
	recordCount int
	data        []T
	isOpen      bool
	destination io.WriteCloser
}

func CreateCSVDataLogger[T any](filename string) (DataLogger[T], error) {
	fmt.Printf("Creating a CSV data logger: %v!\n", filename)
	data := make([]T, 0)
	destination, err := os.Create(filename)
	if err != nil {
		return &CSVDataLogger[T]{&sync.Mutex{}, 0, data, true, destination}, err
	}

	result := CSVDataLogger[T]{&sync.Mutex{}, 0, data, true, destination}
	return &result, nil
}

func (logger *CSVDataLogger[T]) LogRecord(record T) {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	logger.recordCount += 1
	logger.data = append(logger.data, record)
}

func (logger *CSVDataLogger[T]) Export() bool {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	if !logger.isOpen {
		return false
	}

	visibleFields := reflect.VisibleFields(reflect.TypeOf((*T)(nil)).Elem())
	for _, v := range visibleFields {
		description, success := v.Tag.Lookup("Description")
		// If no Description tag default to field name.
		columnName := fmt.Sprintf("%s", v.Name)
		if success {
			columnName = fmt.Sprintf("%s", description)
		}
		logger.destination.Write([]byte(fmt.Sprintf("%s, ", columnName)))
	}
	logger.destination.Write([]byte("\n"))

	for _, d := range logger.data {
		data := reflect.ValueOf(d)
		for _, v := range visibleFields {
			toWrite := data.FieldByIndex(v.Index)
			logger.destination.Write([]byte(fmt.Sprintf("%v, ", toWrite)))
		}
		logger.destination.Write([]byte("\n"))
	}
	return true
}

func (logger *CSVDataLogger[T]) Close() bool {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	if !logger.isOpen {
		return false
	}
	logger.destination.Close()
	logger.isOpen = false
	return true
}

func CreateJSONDataLogger[T any](filename string) (DataLogger[T], error) {
	fmt.Printf("Creating a JSON data logger: %v!\n", filename)
	data := make([]T, 0)
	destination, err := os.Create(filename)
	if err != nil {
		return &JSONDataLogger[T]{&sync.Mutex{}, 0, data, true, destination}, err
	}

	result := JSONDataLogger[T]{&sync.Mutex{}, 0, data, true, destination}
	return &result, nil
}

func (logger *JSONDataLogger[T]) LogRecord(record T) {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	logger.recordCount += 1
	logger.data = append(logger.data, record)
}

func PrettyPrint(out string, spaces int) string {
	return fmt.Sprintf("%*s%s\n", spaces, "", out)
}

// Key Value Pair Print
func KVPPrint(key string, value any) string {
	return fmt.Sprintf("\"%s\": \"%v\"", key, value)
}

func (logger *JSONDataLogger[T]) Export() bool {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	if !logger.isOpen {
		return false
	}

	// The rest of the code is written with the assumption that the indexing stays the same
	visibleFields := reflect.VisibleFields(reflect.TypeOf((*T)(nil)).Elem())

	// Meta Data
	keys := make([]string, 0)
	descriptions := make([]string, 0)
	for _, v := range visibleFields {
		// Keys
		key, keysuccess := v.Tag.Lookup("json")
		if !keysuccess {
			key = fmt.Sprintf("None for %s", v.Name)
		}
		keys = append(keys, key)

		// Descriptions
		description, success := v.Tag.Lookup("Description")
		if !success {
			description = fmt.Sprintf("None for %s", v.Name)
		}
		descriptions = append(descriptions, description)
	}

	spaces := int(0)

	// Open
	logger.destination.Write([]byte(PrettyPrint("{", spaces)))
	spaces += JSONSpaces

	// Write Meta Data
	logger.destination.Write([]byte(PrettyPrint("\"[METADATA]\": {", spaces)))
	spaces += JSONSpaces
	for i, key := range keys {
		if i == len(keys)-1 {
			logger.destination.Write([]byte(PrettyPrint(KVPPrint(key, descriptions[i]), spaces)))
		} else {
			logger.destination.Write([]byte(PrettyPrint(KVPPrint(key, descriptions[i])+",", spaces)))
		}
	}
	spaces -= JSONSpaces
	logger.destination.Write([]byte(PrettyPrint("},", spaces)))

	// Write Data
	logger.destination.Write([]byte(PrettyPrint("\"Data\": [", spaces)))
	spaces += JSONSpaces
	for ix, d := range logger.data {
		logger.destination.Write([]byte(PrettyPrint("{", spaces)))
		spaces += JSONSpaces
		data := reflect.ValueOf(d)
		for i, v := range visibleFields {
			toWrite := data.FieldByIndex(v.Index)
			if i == len(keys)-1 {
				logger.destination.Write([]byte(PrettyPrint(KVPPrint(keys[i], toWrite), spaces)))
			} else {
				logger.destination.Write([]byte(PrettyPrint(KVPPrint(keys[i], toWrite)+",", spaces)))
			}
		}
		spaces -= JSONSpaces
		if ix == len(logger.data)-1 {
			logger.destination.Write([]byte(PrettyPrint("}", spaces)))
		} else {
			logger.destination.Write([]byte(PrettyPrint("},", spaces)))
		}
	}
	spaces -= JSONSpaces
	logger.destination.Write([]byte(PrettyPrint("]", spaces)))

	// Close
	spaces -= JSONSpaces
	logger.destination.Write([]byte(PrettyPrint("}", spaces)))

	return true
}

func (logger *JSONDataLogger[T]) Close() bool {
	logger.mut.Lock()
	defer logger.mut.Unlock()
	if !logger.isOpen {
		return false
	}
	logger.destination.Close()
	logger.isOpen = false
	return true
}
