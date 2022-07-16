package datalogger

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
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

	t := new(T)
	visibleFields := reflect.VisibleFields(reflect.TypeOf(t).Elem())
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
