// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"sync/atomic"
	_ "unsafe"
)

// HTTPVersion represents the HTTP protocol version.
type HTTPVersion uint8

const (
	HTTPVersionUnknown HTTPVersion = iota
	HTTPVersion10                  // HTTP/1.0
	HTTPVersion11                  // HTTP/1.1
	HTTPVersion2                   // HTTP/2
	HTTPVersion3                   // HTTP/3
)

// String returns a human-readable representation of the HTTP version.
func (v HTTPVersion) String() string {
	switch v {
	case HTTPVersion10:
		return "HTTP/1.0"
	case HTTPVersion11:
		return "HTTP/1.1"
	case HTTPVersion2:
		return "HTTP/2"
	case HTTPVersion3:
		return "HTTP/3"
	default:
		return "Unknown"
	}
}

// HTTPErrorKind represents the type of HTTP error that occurred.
type HTTPErrorKind uint8

const (
	HTTPErrorNone     HTTPErrorKind = iota
	HTTPErrorDNS                    // DNS resolution failed
	HTTPErrorConnect                // TCP connection failed
	HTTPErrorTLS                    // TLS handshake failed
	HTTPErrorTimeout                // Request timed out
	HTTPErrorCanceled               // Request was canceled
	HTTPErrorProtocol               // Protocol error (e.g., HTTP/2 stream error)
	HTTPErrorOther                  // Other error
)

// String returns a human-readable representation of the error kind.
func (k HTTPErrorKind) String() string {
	switch k {
	case HTTPErrorNone:
		return "none"
	case HTTPErrorDNS:
		return "dns"
	case HTTPErrorConnect:
		return "connect"
	case HTTPErrorTLS:
		return "tls"
	case HTTPErrorTimeout:
		return "timeout"
	case HTTPErrorCanceled:
		return "canceled"
	case HTTPErrorProtocol:
		return "protocol"
	case HTTPErrorOther:
		return "other"
	default:
		return "unknown"
	}
}

// httpTraceContextKey is the context key for HTTP trace context.
// This is separate from the traceContextKey used for Task in annotation.go.
type httpTraceContextKey struct{}

// TraceContext holds W3C trace context identifiers for distributed tracing.
// It contains a 128-bit trace ID (stored as hi/lo), a 64-bit span ID,
// and optional tracestate header value.
//
// See https://www.w3.org/TR/trace-context/ for the W3C Trace Context specification.
//
// Note: The trace events store a 64-bit trace ID derived from the 128-bit ID
// (XOR of high and low parts) due to trace format limitations.
type TraceContext struct {
	TraceIDHi uint64 // High 64 bits of the 128-bit trace ID
	TraceIDLo uint64 // Low 64 bits of the 128-bit trace ID
	SpanID    uint64 // 64-bit span ID
	State     string // W3C tracestate header value (vendor-specific key=value pairs)
}

// HTTPSpan represents an in-progress HTTP request span.
// Use End to mark the completion of the span.
type HTTPSpan struct {
	spanID    uint64
	isNoop    bool
	isClient  bool
	errorKind HTTPErrorKind
}

var noopHTTPSpan = &HTTPSpan{isNoop: true}

// URL sanitization configuration
var (
	urlSanitizer atomic.Pointer[func(string) string]
)

// shouldSample returns true if the current span should be sampled based on
// the configured sample rate.
//
//go:linkname shouldSample runtime.traceShouldSample
func shouldSample() bool

// SetContentLength is a no-op kept for API compatibility.
// Content length is not currently included in trace events due to format constraints.
func (s *HTTPSpan) SetContentLength(length int64) {
	// No-op - content length not supported in current trace format
}

// SetError sets the error kind for a failed request.
// This should be called before End() when the request fails.
// Only applies to client spans.
func (s *HTTPSpan) SetError(kind HTTPErrorKind) {
	if !s.isNoop && s.isClient {
		s.errorKind = kind
	}
}

// End completes the HTTP span with the given status code.
// Status code should be the HTTP status code (e.g., 200, 404, 500).
// Use 0 if the request failed before receiving a status code.
func (s *HTTPSpan) End(statusCode int) {
	if s.isNoop {
		return
	}
	if s.isClient {
		httpClientRequestEnd(s.spanID, statusCode, s.errorKind)
	} else {
		httpServerRequestEnd(s.spanID, statusCode)
	}
}

// EndWithError completes the HTTP span with an error.
// This emits an HTTPClientError event with details about the failure.
func (s *HTTPSpan) EndWithError(kind HTTPErrorKind, errMsg string) {
	if s.isNoop {
		return
	}
	if s.isClient {
		httpClientError(s.spanID, kind, errMsg)
	}
	// Server errors are handled through status codes
}

// HTTPServerRequest traces an incoming HTTP server request.
// It emits an HTTPServerRequestStart event with the given trace context and path.
//
// The trace context should be extracted from the incoming request's
// traceparent header using ParseTraceContext. If no traceparent header
// exists, pass a zero TraceContext and a new one will be generated.
//
// Returns an HTTPSpan that must be ended with End(statusCode) when the
// request completes.
//
//	tc, _ := trace.ParseTraceContext(r.Header.Get("Traceparent"))
//	tc.State = r.Header.Get("Tracestate")
//	span := trace.HTTPServerRequest(ctx, tc, r.URL.Path)
//	defer span.End(statusCode)
func HTTPServerRequest(ctx context.Context, tc TraceContext, path string) *HTTPSpan {
	if !IsEnabled() {
		return noopHTTPSpan
	}

	// Apply sampling - check if this request should be traced
	if !shouldSample() {
		return noopHTTPSpan
	}

	// If no trace context provided, generate new one
	if tc.TraceIDHi == 0 && tc.TraceIDLo == 0 {
		tc = NewTraceContext()
	}

	// If no span ID, generate one
	if tc.SpanID == 0 {
		tc.SpanID = NewSpanID()
	}

	// Sanitize the path
	path = SanitizeURL(path)

	// Derive 64-bit trace ID from 128-bit ID (XOR high and low parts)
	traceID := tc.TraceIDHi ^ tc.TraceIDLo
	httpServerRequestStart(traceID, tc.SpanID, path)
	return &HTTPSpan{spanID: tc.SpanID, isClient: false}
}

// HTTPClientRequest traces an outgoing HTTP client request.
// It emits an HTTPClientRequestStart event with the given trace context and URL.
//
// The trace context can be extracted from the request context, or a new
// one will be generated. The returned TraceContext should be injected
// into the outgoing request using FormatTraceContext().
//
// Returns an HTTPSpan that must be ended with End(statusCode) when the
// request completes.
//
//	tc := trace.NewTraceContext()
//	span := trace.HTTPClientRequest(ctx, tc, r.URL.String())
//	r.Header.Set("Traceparent", tc.FormatTraceContext())
//	if tc.State != "" {
//	    r.Header.Set("Tracestate", tc.State)
//	}
//	defer span.End(resp.StatusCode)
func HTTPClientRequest(ctx context.Context, tc TraceContext, url string) *HTTPSpan {
	if !IsEnabled() {
		return noopHTTPSpan
	}

	// Apply sampling - check if this request should be traced
	if !shouldSample() {
		return noopHTTPSpan
	}

	// If no trace context provided, generate new one
	if tc.TraceIDHi == 0 && tc.TraceIDLo == 0 {
		tc = NewTraceContext()
	}

	// If no span ID, generate one
	if tc.SpanID == 0 {
		tc.SpanID = NewSpanID()
	}

	// Sanitize the URL
	url = SanitizeURL(url)

	// Derive 64-bit trace ID from 128-bit ID (XOR high and low parts)
	traceID := tc.TraceIDHi ^ tc.TraceIDLo
	httpClientRequestStart(traceID, tc.SpanID, url)
	return &HTTPSpan{spanID: tc.SpanID, isClient: true}
}

// WithSpan returns a context that carries the given trace context.
// Use SpanFromContext to retrieve it.
func WithSpan(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, httpTraceContextKey{}, tc)
}

// SpanFromContext extracts the trace context from the context.
// Returns the TraceContext and true if found, or zero TraceContext and false if not.
func SpanFromContext(ctx context.Context) (TraceContext, bool) {
	if tc, ok := ctx.Value(httpTraceContextKey{}).(TraceContext); ok {
		return tc, true
	}
	return TraceContext{}, false
}

// ParseTraceContext extracts W3C trace context from a traceparent header value.
// The traceparent format is: version-traceId-spanId-flags
// Example: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
//
// Returns the parsed TraceContext and true on success.
// Returns zero TraceContext and false if the header is invalid.
//
// Note: This does not parse the tracestate header. Use ParseTraceState for that,
// and assign the result to TraceContext.State.
//
// See https://www.w3.org/TR/trace-context/#traceparent-header for details.
func ParseTraceContext(header string) (TraceContext, bool) {
	if header == "" {
		return TraceContext{}, false
	}

	// Expected format: version-traceId-spanId-flags
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return TraceContext{}, false
	}

	// Check version (currently only "00" is supported)
	if parts[0] != "00" {
		return TraceContext{}, false
	}

	// Parse 128-bit trace ID (32 hex chars)
	if len(parts[1]) != 32 {
		return TraceContext{}, false
	}
	traceIDBytes, err := hex.DecodeString(parts[1])
	if err != nil || len(traceIDBytes) != 16 {
		return TraceContext{}, false
	}
	traceIDHi := binary.BigEndian.Uint64(traceIDBytes[0:8])
	traceIDLo := binary.BigEndian.Uint64(traceIDBytes[8:16])

	// Validate parent span ID format (16 hex chars) but don't use it
	if len(parts[2]) != 16 {
		return TraceContext{}, false
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return TraceContext{}, false
	}

	// Generate new span ID for this operation
	spanID := NewSpanID()

	return TraceContext{
		TraceIDHi: traceIDHi,
		TraceIDLo: traceIDLo,
		SpanID:    spanID,
	}, true
}

// ParseTraceState parses a W3C tracestate header value.
// Returns a map of vendor keys to their values.
//
// The tracestate format is: key1=value1,key2=value2,...
// Example: "congo=t61rcWkgMzE,rojo=00f067aa0ba902b7"
//
// See https://www.w3.org/TR/trace-context/#tracestate-header for details.
func ParseTraceState(header string) map[string]string {
	if header == "" {
		return nil
	}

	result := make(map[string]string)
	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

// FormatTraceState formats a tracestate map as a W3C tracestate header value.
func FormatTraceState(state map[string]string) string {
	if len(state) == 0 {
		return ""
	}

	var parts []string
	for k, v := range state {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// FormatTraceContext formats the trace context as a W3C traceparent header value.
// The format is: version-traceId-spanId-flags
// Example: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
//
// The flags field is set to "01" (sampled).
func (tc TraceContext) FormatTraceContext() string {
	var buf [55]byte // "00-" + 32 + "-" + 16 + "-01" = 55 bytes

	buf[0] = '0'
	buf[1] = '0'
	buf[2] = '-'

	// Encode 128-bit trace ID
	var traceIDBytes [16]byte
	binary.BigEndian.PutUint64(traceIDBytes[0:8], tc.TraceIDHi)
	binary.BigEndian.PutUint64(traceIDBytes[8:16], tc.TraceIDLo)
	hex.Encode(buf[3:35], traceIDBytes[:])

	buf[35] = '-'

	// Encode 64-bit span ID
	var spanIDBytes [8]byte
	binary.BigEndian.PutUint64(spanIDBytes[:], tc.SpanID)
	hex.Encode(buf[36:52], spanIDBytes[:])

	buf[52] = '-'
	buf[53] = '0'
	buf[54] = '1' // sampled flag

	return string(buf[:])
}

// NewTraceContext generates a new trace context with random trace ID and span ID.
// Use this when starting a new distributed trace.
func NewTraceContext() TraceContext {
	var buf [16]byte
	rand.Read(buf[:])

	return TraceContext{
		TraceIDHi: binary.BigEndian.Uint64(buf[0:8]),
		TraceIDLo: binary.BigEndian.Uint64(buf[8:16]),
		SpanID:    NewSpanID(),
	}
}

// NewSpanID generates a new random 64-bit span ID.
func NewSpanID() uint64 {
	var buf [8]byte
	rand.Read(buf[:])
	return binary.BigEndian.Uint64(buf[:])
}

// SanitizeURL removes sensitive information from URLs.
// By default, it strips query parameters to prevent leaking sensitive data.
// The path is preserved.
//
// Custom sanitization can be configured using SetURLSanitizer.
//
// Example:
//
//	SanitizeURL("https://example.com/api/users?token=secret&id=123")
//	// Returns: "https://example.com/api/users"
func SanitizeURL(url string) string {
	if fn := urlSanitizer.Load(); fn != nil {
		return (*fn)(url)
	}
	return defaultURLSanitizer(url)
}

// defaultURLSanitizer removes query parameters from URLs.
func defaultURLSanitizer(url string) string {
	// Find the query string start
	idx := strings.Index(url, "?")
	if idx == -1 {
		return url
	}
	return url[:idx]
}

// SetURLSanitizer configures a custom URL sanitization function.
// The function receives the raw URL and should return the sanitized version.
//
// Set to nil to restore the default sanitizer (which removes query parameters).
//
// Example - keep only the scheme and host:
//
//	trace.SetURLSanitizer(func(url string) string {
//	    u, err := url.Parse(url)
//	    if err != nil {
//	        return url
//	    }
//	    return u.Scheme + "://" + u.Host
//	})
func SetURLSanitizer(fn func(string) string) {
	if fn == nil {
		urlSanitizer.Store(nil)
	} else {
		urlSanitizer.Store(&fn)
	}
}

//
// Function declarations - implementations provided by runtime via linkname
//

// emits HTTPServerRequestStart event.
func httpServerRequestStart(traceID, spanID uint64, path string)

// emits HTTPServerRequestEnd event.
func httpServerRequestEnd(spanID uint64, statusCode int)

// emits HTTPClientRequestStart event.
func httpClientRequestStart(traceID, spanID uint64, url string)

// emits HTTPClientRequestEnd event.
func httpClientRequestEnd(spanID uint64, statusCode int, errorKind HTTPErrorKind)

// emits HTTPClientError event.
func httpClientError(spanID uint64, errorKind HTTPErrorKind, errMsg string)
