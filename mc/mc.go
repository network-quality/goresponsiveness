package mc

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type MeasurableConnection interface {
	Start(context.Context) bool
	Stop() bool
	Downloaded() uint64
}

type LoadBearingConnection struct {
	Path       string
	ctx        context.Context
	cancel     context.CancelFunc
	downloaded uint64
	client     *http.Client
}

func (lbc *LoadBearingConnection) Downloaded() uint64 {
	return lbc.downloaded
}

func (lbc *LoadBearingConnection) Start(ctx context.Context) bool {
	fmt.Printf("Starting a LBC ...")
	lbc.ctx, lbc.cancel = context.WithCancel(ctx)
	lbc.downloaded = 0
	lbc.client = &http.Client{}
	get, err := lbc.client.Get(lbc.Path)

	if err != nil {
		return false
	}
	go doDownload(get, &lbc.downloaded, lbc.ctx)
	return true
}

func (lbc *LoadBearingConnection) Stop() bool {
	lbc.cancel()
	return true
}

func doDownload(get *http.Response, count *uint64, ctx context.Context) {
	for ctx.Err() == nil {
		n, err := io.CopyN(ioutil.Discard, get.Body, 1024*1024)
		if err != nil {
			fmt.Printf("Done reading!\n")
			break
		}
		fmt.Printf("Read some bytes: %d\n", n)
		*count += uint64(n)
	}
	fmt.Printf("Cancelling my download.\n")
	get.Body.Close()
}
