package mc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

var chunkSize int = 5000

type MeasurableConnection interface {
	Start(context.Context, bool) bool
	Transferred() uint64
}

type LoadBearingDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
}

func (lbd *LoadBearingDownload) Transferred() uint64 {
	return lbd.downloaded
}

func (lbd *LoadBearingDownload) Start(ctx context.Context, debug bool) bool {
	lbd.downloaded = 0
	lbd.client = &http.Client{}

	if debug {
		fmt.Printf("Started a load bearing download.\n")
	}
	go doDownload(ctx, lbd.client, lbd.Path, &lbd.downloaded, debug)
	return true
}

func doDownload(ctx context.Context, client *http.Client, path string, count *uint64, debug bool) {
	get, err := client.Get(path)
	if err != nil {
		return
	}
	for ctx.Err() == nil {
		n, err := io.CopyN(ioutil.Discard, get.Body, int64(chunkSize))
		if err != nil {
			break
		}
		*count += uint64(n)
	}
	get.Body.Close()
	if debug {
		fmt.Printf("Ending a load-bearing download.\n")
	}
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
	n = chunkSize
	*s.n += uint64(n)
	return
}

func doUpload(ctx context.Context, client *http.Client, path string, count *uint64, debug bool) bool {
	*count = 0
	s := &syntheticCountingReader{n: count, ctx: ctx}
	resp, _ := client.Post(path, "application/octet-stream", s)
	resp.Body.Close()
	if debug {
		fmt.Printf("Ending a load-bearing upload.\n")
	}
	return true
}

func (lbu *LoadBearingUpload) Start(ctx context.Context, debug bool) bool {
	lbu.uploaded = 0
	lbu.client = &http.Client{}
	fmt.Printf("Started a load bearing upload.\n")
	go doUpload(ctx, lbu.client, lbu.Path, &lbu.uploaded, debug)
	return true
}
