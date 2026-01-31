// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	. "runtime/trace"
	"strings"
	"testing"
)

func TestParseTraceContext(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantOK     bool
		checkTrace func(t *testing.T, tc TraceContext)
	}{
		{
			name:   "valid traceparent",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantOK: true,
			checkTrace: func(t *testing.T, tc TraceContext) {
				wantTraceIDHi := uint64(0x0af7651916cd43dd)
				wantTraceIDLo := uint64(0x8448eb211c80319c)

				if tc.TraceIDHi != wantTraceIDHi {
					t.Errorf("TraceIDHi = %x, want %x", tc.TraceIDHi, wantTraceIDHi)
				}
				if tc.TraceIDLo != wantTraceIDLo {
					t.Errorf("TraceIDLo = %x, want %x", tc.TraceIDLo, wantTraceIDLo)
				}
				if tc.SpanID == 0 {
					t.Error("SpanID should be generated, got 0")
				}
			},
		},
		{
			name:   "empty header",
			header: "",
			wantOK: false,
		},
		{
			name:   "invalid format - missing parts",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331",
			wantOK: false,
		},
		{
			name:   "invalid format - too many parts",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01-extra",
			wantOK: false,
		},
		{
			name:   "invalid version",
			header: "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantOK: false,
		},
		{
			name:   "invalid trace ID - too short",
			header: "00-0af7651916cd43dd-b7ad6b7169203331-01",
			wantOK: false,
		},
		{
			name:   "invalid trace ID - non-hex",
			header: "00-0af7651916cd43ddXX48eb211c80319c-b7ad6b7169203331-01",
			wantOK: false,
		},
		{
			name:   "invalid span ID - too short",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b71-01",
			wantOK: false,
		},
		{
			name:   "invalid span ID - non-hex",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203XXX-01",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, ok := ParseTraceContext(tt.header)
			if ok != tt.wantOK {
				t.Errorf("ParseTraceContext() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if ok && tt.checkTrace != nil {
				tt.checkTrace(t, tc)
			}
		})
	}
}

func TestFormatTraceContext(t *testing.T) {
	tc := TraceContext{
		TraceIDHi: 0x0af7651916cd43dd,
		TraceIDLo: 0x8448eb211c80319c,
		SpanID:    0xb7ad6b7169203331,
	}

	header := tc.FormatTraceContext()

	// Should be 55 characters: "00-" (3) + traceID (32) + "-" (1) + spanID (16) + "-01" (3)
	if len(header) != 55 {
		t.Errorf("FormatTraceContext() length = %d, want 55", len(header))
	}

	// Should start with version "00-"
	if !strings.HasPrefix(header, "00-") {
		t.Errorf("FormatTraceContext() = %q, want prefix '00-'", header)
	}

	// Should end with flags "-01"
	if !strings.HasSuffix(header, "-01") {
		t.Errorf("FormatTraceContext() = %q, want suffix '-01'", header)
	}

	// Verify we can parse it back
	parsed, ok := ParseTraceContext(header)
	if !ok {
		t.Errorf("ParseTraceContext(%q) failed", header)
	}

	if parsed.TraceIDHi != tc.TraceIDHi || parsed.TraceIDLo != tc.TraceIDLo {
		t.Errorf("Round-trip trace ID mismatch: got %x%x, want %x%x",
			parsed.TraceIDHi, parsed.TraceIDLo, tc.TraceIDHi, tc.TraceIDLo)
	}

	// New span ID should be generated
	if parsed.SpanID == 0 {
		t.Error("ParseTraceContext should generate new span ID")
	}
}

func TestNewTraceContext(t *testing.T) {
	tc1 := NewTraceContext()
	tc2 := NewTraceContext()

	// Should generate unique trace IDs
	if tc1.TraceIDHi == 0 && tc1.TraceIDLo == 0 {
		t.Error("NewTraceContext() generated zero trace ID")
	}

	if tc1.TraceIDHi == tc2.TraceIDHi && tc1.TraceIDLo == tc2.TraceIDLo {
		t.Error("NewTraceContext() generated duplicate trace IDs")
	}

	// Should generate unique span IDs
	if tc1.SpanID == 0 {
		t.Error("NewTraceContext() generated zero span ID")
	}

	if tc1.SpanID == tc2.SpanID {
		t.Error("NewTraceContext() generated duplicate span IDs")
	}

	// Trace IDs should be different from span IDs
	if tc1.TraceIDHi == 0 && tc1.TraceIDLo == 0 {
		t.Error("NewTraceContext() generated zero trace ID")
	}
}

func TestNewSpanID(t *testing.T) {
	id1 := NewSpanID()
	id2 := NewSpanID()

	if id1 == 0 {
		t.Error("NewSpanID() returned 0")
	}

	if id1 == id2 {
		t.Error("NewSpanID() generated duplicate IDs")
	}
}

func TestHTTPServerRequest(t *testing.T) {
	if IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := NewTraceContext()

	// When tracing is disabled, should return noop span
	span := HTTPServerRequest(ctx, tc, "/test")
	if span == nil {
		t.Fatal("HTTPServerRequest() returned nil")
	}

	// End should not panic
	span.End(200)
}

func TestHTTPClientRequest(t *testing.T) {
	if IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := NewTraceContext()

	// When tracing is disabled, should return noop span
	span := HTTPClientRequest(ctx, tc, "http://example.com")
	if span == nil {
		t.Fatal("HTTPClientRequest() returned nil")
	}

	// End should not panic
	span.End(200)
}

func TestHTTPServerTraceIntegration(t *testing.T) {
	if IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	var buf bytes.Buffer
	if err := Start(&buf); err != nil {
		t.Fatalf("failed to start tracing: %v", err)
	}
	defer Stop()

	// Create test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Make request with traceparent header
	req, err := http.NewRequest("GET", server.URL+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stop tracing
	Stop()

	// Verify trace contains data
	if buf.Len() == 0 {
		t.Fatal("trace buffer is empty")
	}

	// Note: Full event parsing would require internal/trace package
	// For now, we verify that tracing produced output
	t.Logf("Trace buffer size: %d bytes", buf.Len())
}

func TestHTTPClientTraceIntegration(t *testing.T) {
	if IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	var buf bytes.Buffer
	if err := Start(&buf); err != nil {
		t.Fatalf("failed to start tracing: %v", err)
	}
	defer Stop()

	// Create test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify traceparent header was set
		if traceparent := r.Header.Get("Traceparent"); traceparent == "" {
			t.Error("traceparent header not set by client")
		} else {
			t.Logf("Received traceparent: %s", traceparent)
		}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Make request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stop tracing
	Stop()

	// Verify trace contains data
	if buf.Len() == 0 {
		t.Fatal("trace buffer is empty")
	}

	t.Logf("Trace buffer size: %d bytes", buf.Len())
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no query params",
			input:    "https://example.com/api/users",
			expected: "https://example.com/api/users",
		},
		{
			name:     "with query params",
			input:    "https://example.com/api/users?token=secret&id=123",
			expected: "https://example.com/api/users",
		},
		{
			name:     "path only",
			input:    "/api/users?page=1",
			expected: "/api/users",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "question mark only",
			input:    "https://example.com?",
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeURL(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSetURLSanitizer(t *testing.T) {
	// Set custom sanitizer
	SetURLSanitizer(func(url string) string {
		return "sanitized"
	})

	got := SanitizeURL("https://example.com/path?secret=value")
	if got != "sanitized" {
		t.Errorf("SanitizeURL() = %q, want %q", got, "sanitized")
	}

	// Reset to nil (default behavior)
	SetURLSanitizer(nil)

	got = SanitizeURL("https://example.com/path?secret=value")
	if got != "https://example.com/path" {
		t.Errorf("SanitizeURL() after reset = %q, want %q", got, "https://example.com/path")
	}
}

func TestParseTraceState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:  "single pair",
			input: "congo=t61rcWkgMzE",
			expected: map[string]string{
				"congo": "t61rcWkgMzE",
			},
		},
		{
			name:  "multiple pairs",
			input: "congo=t61rcWkgMzE,rojo=00f067aa0ba902b7",
			expected: map[string]string{
				"congo": "t61rcWkgMzE",
				"rojo":  "00f067aa0ba902b7",
			},
		},
		{
			name:  "with spaces",
			input: " congo = t61rcWkgMzE , rojo = 00f067aa0ba902b7 ",
			expected: map[string]string{
				"congo": "t61rcWkgMzE",
				"rojo":  "00f067aa0ba902b7",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTraceState(tt.input)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("ParseTraceState(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("ParseTraceState(%q) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("ParseTraceState(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestFormatTraceState(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]string
	}{
		{
			name:  "empty",
			input: nil,
		},
		{
			name: "single pair",
			input: map[string]string{
				"congo": "t61rcWkgMzE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTraceState(tt.input)
			if tt.input == nil {
				if got != "" {
					t.Errorf("FormatTraceState(nil) = %q, want empty", got)
				}
				return
			}
			// Verify round-trip
			parsed := ParseTraceState(got)
			for k, v := range tt.input {
				if parsed[k] != v {
					t.Errorf("Round-trip failed for %q: got %q, want %q", k, parsed[k], v)
				}
			}
		})
	}
}

func TestWithSpanAndSpanFromContext(t *testing.T) {
	tc := TraceContext{
		TraceIDHi: 0x1234,
		TraceIDLo: 0x5678,
		SpanID:    0xabcd,
		State:     "vendor=value",
	}

	ctx := context.Background()

	// Should not find span in empty context
	_, ok := SpanFromContext(ctx)
	if ok {
		t.Error("SpanFromContext should return false for empty context")
	}

	// Add span to context
	ctx = WithSpan(ctx, tc)

	// Should find span
	got, ok := SpanFromContext(ctx)
	if !ok {
		t.Error("SpanFromContext should return true after WithSpan")
	}

	if got.TraceIDHi != tc.TraceIDHi || got.TraceIDLo != tc.TraceIDLo {
		t.Errorf("TraceID mismatch: got %x%x, want %x%x",
			got.TraceIDHi, got.TraceIDLo, tc.TraceIDHi, tc.TraceIDLo)
	}
	if got.SpanID != tc.SpanID {
		t.Errorf("SpanID = %x, want %x", got.SpanID, tc.SpanID)
	}
	if got.State != tc.State {
		t.Errorf("State = %q, want %q", got.State, tc.State)
	}
}

func TestHTTPVersionString(t *testing.T) {
	tests := []struct {
		version  HTTPVersion
		expected string
	}{
		{HTTPVersionUnknown, "Unknown"},
		{HTTPVersion10, "HTTP/1.0"},
		{HTTPVersion11, "HTTP/1.1"},
		{HTTPVersion2, "HTTP/2"},
		{HTTPVersion3, "HTTP/3"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.version.String(); got != tt.expected {
				t.Errorf("HTTPVersion(%d).String() = %q, want %q", tt.version, got, tt.expected)
			}
		})
	}
}

func TestHTTPErrorKindString(t *testing.T) {
	tests := []struct {
		kind     HTTPErrorKind
		expected string
	}{
		{HTTPErrorNone, "none"},
		{HTTPErrorDNS, "dns"},
		{HTTPErrorConnect, "connect"},
		{HTTPErrorTLS, "tls"},
		{HTTPErrorTimeout, "timeout"},
		{HTTPErrorCanceled, "canceled"},
		{HTTPErrorProtocol, "protocol"},
		{HTTPErrorOther, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.expected {
				t.Errorf("HTTPErrorKind(%d).String() = %q, want %q", tt.kind, got, tt.expected)
			}
		})
	}
}

func TestHTTPSpanSetters(t *testing.T) {
	// Test that setters don't panic on spans returned when tracing is disabled
	if IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := NewTraceContext()

	// When tracing is disabled, we get noop spans
	span := HTTPClientRequest(ctx, tc, "http://example.com")

	// These should not panic on noop spans
	span.SetContentLength(1024) // No-op but shouldn't panic
	span.SetError(HTTPErrorTimeout)
	span.End(0)

	// Test EndWithError doesn't panic
	span2 := HTTPClientRequest(ctx, tc, "http://example.com")
	span2.EndWithError(HTTPErrorDNS, "dns lookup failed")
}
