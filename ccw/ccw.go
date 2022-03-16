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
