// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace

import (
	"context"
	_ "unsafe"
)

// TLSVersion represents the TLS protocol version.
type TLSVersion uint16

// TLS version constants matching crypto/tls.
const (
	TLSVersionUnknown TLSVersion = 0
	TLSVersion10      TLSVersion = 0x0301
	TLSVersion11      TLSVersion = 0x0302
	TLSVersion12      TLSVersion = 0x0303
	TLSVersion13      TLSVersion = 0x0304
)

// String returns a human-readable representation of the TLS version.
func (v TLSVersion) String() string {
	switch v {
	case TLSVersion10:
		return "TLS 1.0"
	case TLSVersion11:
		return "TLS 1.1"
	case TLSVersion12:
		return "TLS 1.2"
	case TLSVersion13:
		return "TLS 1.3"
	default:
		return "Unknown"
	}
}

// TLSSpan represents an in-progress TLS handshake span.
// Use End to mark the completion of the span.
type TLSSpan struct {
	spanID uint64
	isNoop bool
}

var noopTLSSpan = &TLSSpan{isNoop: true}

// End completes the TLS span with the negotiated parameters.
// version is the TLS version (e.g., TLSVersion13).
// cipherSuite is the negotiated cipher suite ID.
func (s *TLSSpan) End(version TLSVersion, cipherSuite uint16) {
	if s.isNoop {
		return
	}
	tlsHandshakeEnd(s.spanID, uint16(version), cipherSuite)
}

// TLSHandshake traces a TLS handshake.
// It emits a TLSHandshakeStart event with the given trace context and server name.
//
// Returns a TLSSpan that must be ended with End(version, cipherSuite) when the
// handshake completes.
//
//	span := trace.TLSHandshake(ctx, tc, serverName)
//	conn, err := tls.Dial("tcp", addr, config)
//	if err != nil {
//	    span.End(0, 0) // Failed handshake
//	} else {
//	    state := conn.ConnectionState()
//	    span.End(trace.TLSVersion(state.Version), state.CipherSuite)
//	}
func TLSHandshake(ctx context.Context, tc TraceContext, serverName string) *TLSSpan {
	if !IsEnabled() {
		return noopTLSSpan
	}

	// Apply sampling
	if !shouldSample() {
		return noopTLSSpan
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
	tlsHandshakeStart(traceID, tc.SpanID, serverName)
	return &TLSSpan{spanID: tc.SpanID}
}

//
// Function declarations - implementations provided by runtime via linkname
//

// emits TLSHandshakeStart event.
func tlsHandshakeStart(traceID, spanID uint64, serverName string)

// emits TLSHandshakeEnd event.
func tlsHandshakeEnd(spanID uint64, version, cipherSuite uint16)
