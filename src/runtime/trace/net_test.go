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

func TestNetErrorCodeString(t *testing.T) {
	tests := []struct {
		code     trace.NetErrorCode
		expected string
	}{
		{trace.NetErrorNone, "none"},
		{trace.NetErrorTimeout, "timeout"},
		{trace.NetErrorCanceled, "canceled"},
		{trace.NetErrorRefused, "refused"},
		{trace.NetErrorReset, "reset"},
		{trace.NetErrorUnreach, "unreachable"},
		{trace.NetErrorNoHost, "no_host"},
		{trace.NetErrorOther, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.code.String(); got != tt.expected {
				t.Errorf("NetErrorCode(%d).String() = %q, want %q", tt.code, got, tt.expected)
			}
		})
	}
}

func TestDNSLookup(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	// When tracing is disabled, should return noop span
	span := trace.DNSLookup(ctx, tc, "example.com")
	if span == nil {
		t.Fatal("DNSLookup() returned nil")
	}

	// End should not panic
	span.End(2)
}

func TestDNSLookupWithTracing(t *testing.T) {
	trace.SetEventFilter(trace.FilterCore | trace.FilterNet | trace.FilterCustom)
	defer trace.SetEventFilter(trace.FilterDefault)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.DNSLookup(ctx, tc, "example.com")
	span.End(2)

	trace.Stop()

	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	t.Logf("Trace buffer size: %d bytes", buf.Len())
}

func TestDNSLookupWithError(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.DNSLookup(ctx, tc, "nonexistent.example.com")
	span.SetError(trace.NetErrorNoHost)
	span.End(0)
}

func TestConnect(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	// When tracing is disabled, should return noop span
	span := trace.Connect(ctx, tc, "example.com:443")
	if span == nil {
		t.Fatal("Connect() returned nil")
	}

	// End should not panic
	span.End()
}

func TestConnectWithTracing(t *testing.T) {
	trace.SetEventFilter(trace.FilterCore | trace.FilterNet | trace.FilterCustom)
	defer trace.SetEventFilter(trace.FilterDefault)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.Connect(ctx, tc, "example.com:443")
	span.End()

	trace.Stop()

	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	t.Logf("Trace buffer size: %d bytes", buf.Len())
}

func TestConnectWithError(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.Connect(ctx, tc, "192.168.0.1:12345")
	span.SetError(trace.NetErrorRefused)
	span.End()
}
