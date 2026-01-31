// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace_test

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"
)

func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		version  trace.TLSVersion
		expected string
	}{
		{trace.TLSVersionUnknown, "Unknown"},
		{trace.TLSVersion10, "TLS 1.0"},
		{trace.TLSVersion11, "TLS 1.1"},
		{trace.TLSVersion12, "TLS 1.2"},
		{trace.TLSVersion13, "TLS 1.3"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.version.String(); got != tt.expected {
				t.Errorf("TLSVersion(%d).String() = %q, want %q", tt.version, got, tt.expected)
			}
		})
	}
}

func TestTLSHandshake(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	// When tracing is disabled, should return noop span
	span := trace.TLSHandshake(ctx, tc, "example.com")
	if span == nil {
		t.Fatal("TLSHandshake() returned nil")
	}

	// End should not panic
	span.End(trace.TLSVersion13, 0x1301)
}

func TestTLSHandshakeWithTracing(t *testing.T) {
	trace.SetEventFilter(trace.FilterCore | trace.FilterTLS | trace.FilterCustom)
	defer trace.SetEventFilter(trace.FilterDefault)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.TLSHandshake(ctx, tc, "example.com")
	span.End(trace.TLSVersion13, 0x1301) // TLS_AES_128_GCM_SHA256

	trace.Stop()

	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	t.Logf("Trace buffer size: %d bytes", buf.Len())
}
