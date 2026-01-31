// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Event filtering allows selective tracing of event categories.

By default, only core runtime events (goroutines, GC, scheduling) and user
annotations (tasks, regions, logs) are traced. Additional categories like
HTTP, SQL, TLS, and network events can be enabled for distributed tracing
scenarios.

# Enabling Categories via Environment

Use GODEBUG environment variables:

	GODEBUG=tracehttp=1 ./myapp              # Enable HTTP events
	GODEBUG=tracehttp=1,tracesql=1 ./myapp   # Enable HTTP and SQL events

Available variables: tracehttp, tracesql, tracetls, tracenet

# Enabling Categories via API

Use SetEventFilter before starting a trace:

	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP)
	trace.Start(w)

# Example: HTTP Server Tracing

	func main() {
	    trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP | trace.FilterCustom)

	    f, _ := os.Create("trace.out")
	    trace.Start(f)
	    defer trace.Stop()

	    http.HandleFunc("/", handler)
	    http.ListenAndServe(":8080", nil)
	}

# Example: FlightRecorder with Distributed Tracing

	func main() {
	    trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP)

	    fr := trace.NewFlightRecorder()
	    fr.Start()

	    // HTTP requests are automatically traced with W3C Trace Context
	    http.HandleFunc("/api", apiHandler)
	    http.ListenAndServe(":8080", nil)
	}
*/
package trace

import (
	"internal/trace/tracev2"
	_ "unsafe" // for go:linkname
)

// EventFilter is a bitmask indicating which event categories should be traced.
// Multiple categories can be combined using bitwise OR.
type EventFilter uint32

const (
	// FilterCore enables core runtime events (goroutine, GC, P scheduling).
	// These events are always enabled when tracing is active.
	FilterCore EventFilter = EventFilter(tracev2.CategoryCore)

	// FilterHTTP enables HTTP client and server request tracing events.
	FilterHTTP EventFilter = EventFilter(tracev2.CategoryHTTP)

	// FilterSQL enables SQL/database query tracing events.
	FilterSQL EventFilter = EventFilter(tracev2.CategorySQL)

	// FilterTLS enables TLS handshake tracing events.
	FilterTLS EventFilter = EventFilter(tracev2.CategoryTLS)

	// FilterNet enables network-level events (DNS, TCP connect).
	FilterNet EventFilter = EventFilter(tracev2.CategoryNet)

	// FilterCustom enables user annotation events (tasks, regions, logs).
	FilterCustom EventFilter = EventFilter(tracev2.CategoryCustom)

	// FilterAll enables all event categories.
	FilterAll EventFilter = EventFilter(tracev2.CategoryAll)

	// FilterDefault is the default set of enabled categories.
	// Includes core runtime events and user annotations.
	FilterDefault EventFilter = EventFilter(tracev2.CategoryDefault)
)

// SetEventFilter configures which event categories are traced.
// This must be called before Start() or FlightRecorder.Start() to take effect.
// If called while tracing is active, it will affect only newly started traces.
//
// The filter is a bitmask of EventFilter constants. For example:
//
//	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP | trace.FilterSQL)
//
// By default, FilterDefault (FilterCore | FilterCustom) is used.
//
// This function is safe to call concurrently.
func SetEventFilter(filter EventFilter) {
	setEventFilter(uint32(filter))
}

// GetEventFilter returns the current event filter configuration.
func GetEventFilter() EventFilter {
	return EventFilter(getEventFilter())
}

// setEventFilter is implemented in the runtime package.
//
//go:linkname setEventFilter runtime.traceSetEventFilter
func setEventFilter(filter uint32)

// getEventFilter is implemented in the runtime package.
//
//go:linkname getEventFilter runtime.traceGetEventFilter
func getEventFilter() uint32

// SetSampleRate sets the sampling rate for distributed tracing events.
// The rate is a value between 0.0 and 1.0:
//   - 1.0 means trace all requests (default)
//   - 0.5 means trace 50% of requests
//   - 0.1 means trace 10% of requests
//   - 0.0 means trace no requests (effectively disables HTTP/SQL/etc events)
//
// Sampling only affects "span" events (HTTP requests, SQL queries, etc.),
// not core runtime events which are always traced when tracing is enabled.
//
// This function is safe to call concurrently and can be changed while
// tracing is active.
//
// Example:
//
//	// Trace 10% of requests to reduce overhead
//	trace.SetSampleRate(0.1)
//	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP)
//	trace.Start(w)
func SetSampleRate(rate float64) {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	// Convert to uint32 for atomic storage (0 = 0%, 0xFFFFFFFF = 100%)
	setSampleRate(uint32(rate * 0xFFFFFFFF))
}

// GetSampleRate returns the current sampling rate (0.0 to 1.0).
func GetSampleRate() float64 {
	return float64(getSampleRate()) / 0xFFFFFFFF
}

// setSampleRate is implemented in the runtime package.
//
//go:linkname setSampleRate runtime.traceSetSampleRate
func setSampleRate(rate uint32)

// getSampleRate is implemented in the runtime package.
//
//go:linkname getSampleRate runtime.traceGetSampleRate
func getSampleRate() uint32
