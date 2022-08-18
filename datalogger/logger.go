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

	"github.com/network-quality/goresponsiveness/utilities"
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

func CreateCSVDataLogger[T any](filename string) (DataLogger[T], error) {
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

func doCustomFormatting(value reflect.Value, tag reflect.StructTag) (string, error) {
	if utilities.IsInterfaceNil(value) {
		return "", fmt.Errorf("Cannot format an empty interface value")
	}
	formatMethodName, success := tag.Lookup("Formatter")
	if !success {
		return "", fmt.Errorf("Could not find the formatter name")
	}
	formatMethodArgument, success := tag.Lookup("FormatterArgument")
	if !success {
		formatMethodArgument = ""
	}

	formatMethod := value.MethodByName(formatMethodName)
	if formatMethod == reflect.ValueOf(0) {
		return "", fmt.Errorf(
			"Type %v does not support a method named %v",
			value.Type(),
			formatMethodName,
		)
	}
	var formatMethodArgumentUsable []reflect.Value = make([]reflect.Value, 0)
	if formatMethodArgument != "" {
		formatMethodArgumentUsable = append(
			formatMethodArgumentUsable,
			reflect.ValueOf(formatMethodArgument),
		)
	}
	result := formatMethod.Call(formatMethodArgumentUsable)
	if len(result) == 1 {
		return fmt.Sprintf("%v", result[0]), nil
	}
	return "", fmt.Errorf("Too many results returned by the format method's invocation.")
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
		columnName := fmt.Sprintf("%s", v.Name)
		if success {
			columnName = fmt.Sprintf("%s", description)
		}
		logger.destination.Write([]byte(fmt.Sprintf("%s, ", columnName)))
	}
	logger.destination.Write([]byte("\n"))

	for _, d := range logger.data {
		for _, v := range visibleFields {
			data := reflect.ValueOf(d)
			toWrite := data.FieldByIndex(v.Index)
			if formattedToWrite, err := doCustomFormatting(toWrite, v.Tag); err == nil {
				logger.destination.Write([]byte(fmt.Sprintf("%s,", formattedToWrite)))
			} else {
				logger.destination.Write([]byte(fmt.Sprintf("%v, ", toWrite)))
			}
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
