package traceable

import (
	"crypto/tls"
	"net/http/httptrace"
	"time"

	"github.com/network-quality/goresponsiveness/debug"
)

type Traceable interface {
	SetDnsStartTimeInfo(time.Time, httptrace.DNSStartInfo)
	SetDnsDoneTimeInfo(time.Time, httptrace.DNSDoneInfo)
	SetConnectStartTime(time.Time)
	SetConnectDoneTimeError(time.Time, error)
	SetGetConnTime(time.Time)
	SetGotConnTimeInfo(time.Time, httptrace.GotConnInfo)
	SetTLSHandshakeStartTime(time.Time)
	SetTLSHandshakeDoneTimeState(time.Time, tls.ConnectionState)
	SetHttpWroteRequestTimeInfo(time.Time, httptrace.WroteRequestInfo)
	SetHttpResponseReadyTime(time.Time)
}

func GenerateHttpTimingTracer(
	traceable Traceable,
	debug debug.DebugLevel,
) *httptrace.ClientTrace {
	tracer := httptrace.ClientTrace{
		DNSStart: func(dnsStartInfo httptrace.DNSStartInfo) {
			traceable.SetDnsStartTimeInfo(time.Now(), dnsStartInfo)
		},
		DNSDone: func(dnsDoneInfo httptrace.DNSDoneInfo) {
			traceable.SetDnsDoneTimeInfo(time.Now(), dnsDoneInfo)
		},
		ConnectStart: func(network, address string) {
			traceable.SetConnectStartTime(time.Now())
		},
		ConnectDone: func(network, address string, err error) {
			traceable.SetConnectDoneTimeError(time.Now(), err)
		},
		GetConn: func(hostPort string) {
			traceable.SetGetConnTime(time.Now())
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			traceable.SetGotConnTimeInfo(time.Now(), connInfo)
		},
		TLSHandshakeStart: func() {
			traceable.SetTLSHandshakeStartTime(time.Now())
		},
		TLSHandshakeDone: func(tlsConnState tls.ConnectionState, err error) {
			traceable.SetTLSHandshakeDoneTimeState(time.Now(), tlsConnState)
		},
		WroteRequest: func(wroteRequest httptrace.WroteRequestInfo) {
			traceable.SetHttpWroteRequestTimeInfo(time.Now(), wroteRequest)
		},
		GotFirstResponseByte: func() {
			traceable.SetHttpResponseReadyTime(time.Now())
		},
	}
	return &tracer
}
