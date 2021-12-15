package lbc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

var chunkSize int = 5000

type LoadBearingConnection interface {
	Start(context.Context, bool) bool
	Transferred() uint64
	Client() *http.Client
}

type LoadBearingConnectionDownload struct {
	Path       string
	downloaded uint64
	client     *http.Client
}

func (lbd *LoadBearingConnectionDownload) Transferred() uint64 {
	return lbd.downloaded
}

func (lbd *LoadBearingConnectionDownload) Client() *http.Client {
	return lbd.client
}

func (lbd *LoadBearingConnectionDownload) Start(ctx context.Context, debug bool) bool {
	lbd.downloaded = 0
	lbd.client = &http.Client{}

	// At some point this might be useful: It is a snippet of code that will enable
	// logging of per-session TLS key material in order to make debugging easier in
	// Wireshark.
	/*
		lbd.client = &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					KeyLogWriter: w,

					Rand:               utilities.RandZeroSource{}, // for reproducible output; don't do this.
					InsecureSkipVerify: true,                       // test server certificate is not trusted.
				},
			},
		}
	*/

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

type LoadBearingConnectionUpload struct {
	Path     string
	uploaded uint64
	client   *http.Client
}

func (lbu *LoadBearingConnectionUpload) Transferred() uint64 {
	return lbu.uploaded
}

func (lbd *LoadBearingConnectionUpload) Client() *http.Client {
	return lbd.client
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

func (lbu *LoadBearingConnectionUpload) Start(ctx context.Context, debug bool) bool {
	lbu.uploaded = 0
	lbu.client = &http.Client{}
	fmt.Printf("Started a load bearing upload.\n")
	go doUpload(ctx, lbu.client, lbu.Path, &lbu.uploaded, debug)
	return true
}
