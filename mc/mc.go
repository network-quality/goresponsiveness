package mc

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
)

type MeasurableConnection interface {
	Start(context.Context) bool
	Transferred() uint64
}

type LoadBearingDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
}

func (lbc *LoadBearingDownload) Transferred() uint64 {
	return lbc.downloaded
}

func (lbc *LoadBearingDownload) Start(ctx context.Context) bool {
	lbc.downloaded = 0
	lbc.client = &http.Client{}
	get, err := lbc.client.Get(lbc.Path)

	if err != nil {
		return false
	}
	go doDownload(get, &lbc.downloaded, ctx)
	return true
}

func doDownload(get *http.Response, count *uint64, ctx context.Context) {
	for ctx.Err() == nil {
		n, err := io.CopyN(ioutil.Discard, get.Body, 100)
		if err != nil {
			break
		}
		*count += uint64(n)
	}
	get.Body.Close()
}

type LoadBearingUpload struct {
	Path     string
	uploaded uint64
	client   *http.Client
}

func (lbu *LoadBearingUpload) Transferred() uint64 {
	return lbu.uploaded
}

type syntheticCountingReader struct {
	n   *uint64
	ctx context.Context
}

func (s *syntheticCountingReader) Read(p []byte) (n int, err error) {
	if s.ctx.Err() != nil {
		return 0, io.EOF
	}
	err = nil
	n = len(p)
	*s.n += uint64(n)
	return
}
func (lbu *LoadBearingUpload) Start(ctx context.Context) bool {
	lbu.uploaded = 0
	lbu.client = &http.Client{}
	s := &syntheticCountingReader{n: &lbu.uploaded, ctx: ctx}
	go func() {
		resp, _ := lbu.client.Post(lbu.Path, "application/octet-stream", s)
		resp.Body.Close()
	}()
	return true
}
