// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package usdt provides USDT (Userland Statically Defined Tracing) probes.
//
// USDT probes allow tracing tools like bpftrace, SystemTap, and perf to
// attach to specific points in Go programs at runtime. When a tracer attaches,
// it patches the probe instruction from a NOP to a breakpoint, enabling
// zero-overhead tracing when no tracer is attached.
//
// # Supported Platforms
//
// USDT probes are currently supported on:
//   - linux/amd64
//   - linux/arm64
//
// On unsupported platforms, Probe calls compile to no-ops with no metadata.
//
// # Basic Usage
//
// Both provider and name arguments must be compile-time string literals.
// The compiler treats Probe calls as intrinsics and emits metadata into
// the ELF .note.stapsdt section for tracer discovery.
//
//	usdt.Probe("myapp", "request_start")
//	// ... handle request ...
//	usdt.Probe("myapp", "request_end")
//
// # Tracing with bpftrace
//
// Due to Go's static linking, the standard bpftrace USDT syntax may not work.
// Instead, extract probe addresses from the binary and use uprobe syntax:
//
//	# List probe addresses
//	readelf -n ./myapp | grep -E "Provider|Name|Location"
//
//	# Trace using uprobe with address from .note.stapsdt
//	bpftrace -e 'uprobe:./myapp:0x<address> { printf("probe hit\n"); }'
//
// For convenience, a helper script can parse .note.stapsdt and generate
// bpftrace commands automatically.
//
// # Standard Library Probes
//
// The net/http package includes built-in USDT probes for HTTP tracing:
//
//	Provider: net_http
//	Probes:
//	  - server_request_start  (before ServeHTTP)
//	  - server_request_end    (after handler completes)
//	  - client_request_start  (before sending request)
//	  - client_response_received (after receiving response)
//
// Example tracing HTTP latency:
//
//	# Get probe addresses
//	readelf -n ./myserver | grep -A2 "net_http"
//
//	# Trace with timestamps
//	bpftrace -e '
//	  uprobe:./myserver:0x<start_addr> { @start[tid] = nsecs; }
//	  uprobe:./myserver:0x<end_addr> {
//	    printf("latency: %dus\n", (nsecs - @start[tid])/1000);
//	  }
//	'
//
// # Probe Arguments
//
// Probes can pass arguments to tracers using Probe1-Probe4 functions:
//
//	usdt.Probe1("myapp", "request_end", statusCode)
//	usdt.Probe2("myapp", "transfer", bytesSent, bytesReceived)
//
// Arguments must be integer types (int, int8-64, uint, uint8-64, uintptr)
// or unsafe.Pointer. The argument values are encoded in the .note.stapsdt
// argdesc field using SystemTap SDT format (e.g., "-4@%eax" for int32 in eax).
//
// Tracers can read arguments using the arg0, arg1, etc. variables:
//
//	bpftrace -e 'uprobe:./myserver:0x<addr> { printf("status=%d\n", arg0); }'
//
// # Known Limitations
//
//   - No semaphores: Probes cannot be enabled/disabled at runtime.
//     The NOP instruction is always present.
//
//   - Static binary limitation: bpftrace's "usdt:" syntax may not discover
//     probes in statically linked Go binaries. Use "uprobe:" with addresses
//     extracted from .note.stapsdt instead.
//
//   - String literals only: Provider and name must be compile-time constants.
//     Dynamic strings cause a compile error.
//
//   - Argument count: Maximum 4 arguments per probe (Probe1-Probe4).
//
// # Implementation Details
//
// On supported platforms, the compiler replaces Probe calls with:
//   - A single NOP instruction (0x90 on amd64, 0xD503201F on arm64)
//   - Metadata in the .note.stapsdt ELF section containing:
//   - Probe location (absolute address)
//   - Base address (text segment start)
//   - Provider and probe name strings
//
// The .note.stapsdt format follows the SystemTap SDT specification,
// enabling compatibility with Linux tracing infrastructure.
package usdt

import "unsafe"

// Probe emits a USDT probe with the given provider and name.
//
// Both provider and name MUST be compile-time string literals; using
// variables will result in a compile-time error.
//
// The call compiles to a single NOP instruction. When no tracer is attached,
// there is zero runtime overhead. When a tracer attaches, it patches the NOP
// to a breakpoint instruction and can inspect the program state.
//
// On unsupported platforms, the call is a no-op.
//
// Note: On supported platforms (linux/amd64), the compiler may replace this
// call with a NOP instruction and emit USDT metadata. This function provides
// a fallback for unsupported platforms.
func Probe(provider, name string) {
	// No-op fallback for unsupported platforms.
	// On linux/amd64, the compiler treats this as an intrinsic and
	// replaces the call with a NOP instruction.
}

// USDTArg constrains the types that can be passed as USDT probe arguments.
// Only integer types and unsafe.Pointer are supported since they can be
// passed in registers and encoded in the argdesc format.
//
// To pass a string, use unsafe.Pointer with unsafe.StringData:
//
//	usdt.Probe1("myapp", "request", unsafe.Pointer(unsafe.StringData(method)))
//
// The tracer can then read the null-terminated string data:
//
//	bpftrace -e 'uprobe:./app:0x<addr> { printf("method=%s\n", str(arg0)); }'
type USDTArg interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		unsafe.Pointer
}

// Probe1 emits a USDT probe with one argument.
//
// Both provider and name MUST be compile-time string literals.
// The argument value is passed to tracers via the argdesc field.
//
// On unsupported platforms, the call is a no-op.
func Probe1[T USDTArg](provider, name string, arg1 T) {
	// No-op fallback for unsupported platforms.
}

// Probe2 emits a USDT probe with two arguments.
//
// Both provider and name MUST be compile-time string literals.
// The argument values are passed to tracers via the argdesc field.
//
// On unsupported platforms, the call is a no-op.
func Probe2[T1, T2 USDTArg](provider, name string, arg1 T1, arg2 T2) {
	// No-op fallback for unsupported platforms.
}

// Probe3 emits a USDT probe with three arguments.
//
// Both provider and name MUST be compile-time string literals.
// The argument values are passed to tracers via the argdesc field.
//
// On unsupported platforms, the call is a no-op.
func Probe3[T1, T2, T3 USDTArg](provider, name string, arg1 T1, arg2 T2, arg3 T3) {
	// No-op fallback for unsupported platforms.
}

// Probe4 emits a USDT probe with four arguments.
//
// Both provider and name MUST be compile-time string literals.
// The argument values are passed to tracers via the argdesc field.
//
// On unsupported platforms, the call is a no-op.
func Probe4[T1, T2, T3, T4 USDTArg](provider, name string, arg1 T1, arg2 T2, arg3 T3, arg4 T4) {
	// No-op fallback for unsupported platforms.
}
