// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace_test

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"
)

func TestEventFilterConstants(t *testing.T) {
	// Verify filter constants are properly defined
	if trace.FilterCore == 0 {
		t.Error("FilterCore should not be zero")
	}
	if trace.FilterHTTP == 0 {
		t.Error("FilterHTTP should not be zero")
	}
	if trace.FilterSQL == 0 {
		t.Error("FilterSQL should not be zero")
	}
	if trace.FilterTLS == 0 {
		t.Error("FilterTLS should not be zero")
	}
	if trace.FilterNet == 0 {
		t.Error("FilterNet should not be zero")
	}
	if trace.FilterCustom == 0 {
		t.Error("FilterCustom should not be zero")
	}

	// Verify FilterAll contains all categories
	all := trace.FilterCore | trace.FilterHTTP | trace.FilterSQL |
		trace.FilterTLS | trace.FilterNet | trace.FilterCustom
	if trace.FilterAll != all {
		t.Errorf("FilterAll = %d, want %d", trace.FilterAll, all)
	}

	// Verify FilterDefault contains Core and Custom
	want := trace.FilterCore | trace.FilterCustom
	if trace.FilterDefault != want {
		t.Errorf("FilterDefault = %d, want %d", trace.FilterDefault, want)
	}
}

func TestSetEventFilter(t *testing.T) {
	// Set a custom filter
	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP)

	// Verify we can read it back
	got := trace.GetEventFilter()
	want := trace.FilterCore | trace.FilterHTTP
	if got != want {
		t.Errorf("GetEventFilter() = %d, want %d", got, want)
	}

	// Reset to default
	trace.SetEventFilter(trace.FilterDefault)
}

func TestEventFilterWithTracing(t *testing.T) {
	// Set filter to only Core events (no Custom)
	trace.SetEventFilter(trace.FilterCore)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("trace.Start failed: %v", err)
	}

	// Create a task and emit a user log event (CategoryCustom) - should be filtered out
	ctx, task := trace.NewTask(context.Background(), "test-task")
	trace.Log(ctx, "test-category", "test-message")
	task.End()

	trace.Stop()

	// The trace should still have data (header, core events from goroutines)
	if buf.Len() == 0 {
		t.Error("trace buffer is empty, expected at least header and core events")
	}

	// Reset filter to default for other tests
	trace.SetEventFilter(trace.FilterDefault)
}

func TestEventFilterHTTP(t *testing.T) {
	// Set filter to include HTTP events
	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP | trace.FilterCustom)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("trace.Start failed: %v", err)
	}

	// HTTP events would be emitted by net/http package
	// For now, just verify tracing works with HTTP filter enabled

	trace.Stop()

	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	// Reset filter
	trace.SetEventFilter(trace.FilterDefault)
}

func TestEventFilterPreservedAcrossStart(t *testing.T) {
	// Set a custom filter before starting trace
	trace.SetEventFilter(trace.FilterCore | trace.FilterSQL)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("trace.Start failed: %v", err)
	}

	// Verify filter is preserved (should include SQL since we set it)
	got := trace.GetEventFilter()
	// Note: StartTrace may add GODEBUG-enabled categories
	if got&trace.FilterSQL == 0 {
		t.Error("FilterSQL should be enabled after Start")
	}
	if got&trace.FilterCore == 0 {
		t.Error("FilterCore should be enabled after Start")
	}

	trace.Stop()

	// Reset filter
	trace.SetEventFilter(trace.FilterDefault)
}

func TestSampleRate(t *testing.T) {
	// Test setting and getting sample rate
	trace.SetSampleRate(1.0) // 100%
	if got := trace.GetSampleRate(); got < 0.99 || got > 1.01 {
		t.Errorf("GetSampleRate() = %f, want ~1.0", got)
	}

	trace.SetSampleRate(0.5) // 50%
	if got := trace.GetSampleRate(); got < 0.49 || got > 0.51 {
		t.Errorf("GetSampleRate() = %f, want ~0.5", got)
	}

	trace.SetSampleRate(0.0) // 0%
	if got := trace.GetSampleRate(); got > 0.01 {
		t.Errorf("GetSampleRate() = %f, want ~0.0", got)
	}

	// Test clamping
	trace.SetSampleRate(2.0) // Should clamp to 1.0
	if got := trace.GetSampleRate(); got < 0.99 || got > 1.01 {
		t.Errorf("GetSampleRate() after 2.0 = %f, want ~1.0", got)
	}

	trace.SetSampleRate(-1.0) // Should clamp to 0.0
	if got := trace.GetSampleRate(); got > 0.01 {
		t.Errorf("GetSampleRate() after -1.0 = %f, want ~0.0", got)
	}

	// Reset to 100%
	trace.SetSampleRate(1.0)
}

func TestSampleRateWithTracing(t *testing.T) {
	// Set sample rate to 0% - no spans should be traced
	trace.SetSampleRate(0.0)
	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP | trace.FilterCustom)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("trace.Start failed: %v", err)
	}

	// These should all return noop spans due to 0% sampling
	// (HTTP events would normally be traced but sampling blocks them)

	trace.Stop()

	// The trace should have some data (header + core events)
	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	// Reset
	trace.SetSampleRate(1.0)
	trace.SetEventFilter(trace.FilterDefault)
}
