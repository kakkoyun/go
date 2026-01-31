// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace

import (
	"context"
	_ "unsafe"
)

// NetErrorCode represents the type of network error that occurred.
type NetErrorCode uint8

const (
	NetErrorNone     NetErrorCode = iota
	NetErrorTimeout               // Operation timed out
	NetErrorCanceled              // Operation was canceled
	NetErrorRefused               // Connection refused
	NetErrorReset                 // Connection reset
	NetErrorUnreach               // Network/host unreachable
	NetErrorNoHost                // DNS: no such host
	NetErrorOther                 // Other error
)

// String returns a human-readable representation of the error code.
func (c NetErrorCode) String() string {
	switch c {
	case NetErrorNone:
		return "none"
	case NetErrorTimeout:
		return "timeout"
	case NetErrorCanceled:
		return "canceled"
	case NetErrorRefused:
		return "refused"
	case NetErrorReset:
		return "reset"
	case NetErrorUnreach:
		return "unreachable"
	case NetErrorNoHost:
		return "no_host"
	case NetErrorOther:
		return "other"
	default:
		return "unknown"
	}
}

// DNSSpan represents an in-progress DNS lookup span.
// Use End to mark the completion of the span.
type DNSSpan struct {
	spanID    uint64
	isNoop    bool
	errorCode NetErrorCode
}

var noopDNSSpan = &DNSSpan{isNoop: true}

// SetError sets the error code for a failed lookup.
// This should be called before End() when the lookup fails.
func (s *DNSSpan) SetError(code NetErrorCode) {
	if !s.isNoop {
		s.errorCode = code
	}
}

// End completes the DNS span with the number of addresses resolved.
// Use addrCount=0 if the lookup failed.
func (s *DNSSpan) End(addrCount int) {
	if s.isNoop {
		return
	}
	dnsLookupEnd(s.spanID, addrCount)
}

// DNSLookup traces a DNS lookup operation.
// It emits a DNSLookupStart event with the given trace context and host.
//
// Returns a DNSSpan that must be ended with End(addrCount) when the
// lookup completes.
//
//	span := trace.DNSLookup(ctx, tc, host)
//	addrs, err := net.LookupHost(host)
//	if err != nil {
//	    span.SetError(trace.NetErrorNoHost)
//	    span.End(0)
//	} else {
//	    span.End(len(addrs))
//	}
func DNSLookup(ctx context.Context, tc TraceContext, host string) *DNSSpan {
	if !IsEnabled() {
		return noopDNSSpan
	}

	// Apply sampling
	if !shouldSample() {
		return noopDNSSpan
	}

	// If no trace context provided, generate new one
	if tc.TraceIDHi == 0 && tc.TraceIDLo == 0 {
		tc = NewTraceContext()
	}

	// If no span ID, generate one
	if tc.SpanID == 0 {
		tc.SpanID = NewSpanID()
	}

	// Derive 64-bit trace ID from 128-bit ID (XOR high and low parts)
	traceID := tc.TraceIDHi ^ tc.TraceIDLo
	dnsLookupStart(traceID, tc.SpanID, host)
	return &DNSSpan{spanID: tc.SpanID}
}

// ConnectSpan represents an in-progress TCP connect span.
// Use End to mark the completion of the span.
type ConnectSpan struct {
	spanID    uint64
	isNoop    bool
	errorCode NetErrorCode
}

var noopConnectSpan = &ConnectSpan{isNoop: true}

// SetError sets the error code for a failed connection.
// This should be called before End() when the connection fails.
func (s *ConnectSpan) SetError(code NetErrorCode) {
	if !s.isNoop {
		s.errorCode = code
	}
}

// End completes the connect span.
func (s *ConnectSpan) End() {
	if s.isNoop {
		return
	}
	connectEnd(s.spanID, s.errorCode)
}

// Connect traces a TCP connection attempt.
// It emits a ConnectStart event with the given trace context and address.
//
// The address should be in "host:port" format.
//
// Returns a ConnectSpan that must be ended with End() when the
// connection attempt completes.
//
//	span := trace.Connect(ctx, tc, addr)
//	conn, err := net.DialContext(ctx, "tcp", addr)
//	if err != nil {
//	    span.SetError(trace.NetErrorRefused)
//	}
//	span.End()
func Connect(ctx context.Context, tc TraceContext, addr string) *ConnectSpan {
	if !IsEnabled() {
		return noopConnectSpan
	}

	// Apply sampling
	if !shouldSample() {
		return noopConnectSpan
	}

	// If no trace context provided, generate new one
	if tc.TraceIDHi == 0 && tc.TraceIDLo == 0 {
		tc = NewTraceContext()
	}

	// If no span ID, generate one
	if tc.SpanID == 0 {
		tc.SpanID = NewSpanID()
	}

	// Derive 64-bit trace ID from 128-bit ID (XOR high and low parts)
	traceID := tc.TraceIDHi ^ tc.TraceIDLo
	connectStart(traceID, tc.SpanID, addr)
	return &ConnectSpan{spanID: tc.SpanID}
}

//
// Function declarations - implementations provided by runtime via linkname
//

// emits DNSLookupStart event.
func dnsLookupStart(traceID, spanID uint64, host string)

// emits DNSLookupEnd event.
func dnsLookupEnd(spanID uint64, addrCount int)

// emits ConnectStart event.
func connectStart(traceID, spanID uint64, addr string)

// emits ConnectEnd event.
func connectEnd(spanID uint64, errorCode NetErrorCode)
