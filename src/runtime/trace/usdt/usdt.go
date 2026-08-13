// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package usdt provides USDT (Userland Statically Defined Tracing) probes.
//
// USDT probes allow tracing tools like bpftrace, SystemTap, and perf to
// attach to specific points in Go programs at runtime. When no tracer is
// attached, the probe is a single NOP instruction with zero overhead.
//
// # Supported platforms
//
// USDT probes are supported on linux/amd64 and linux/arm64.
// On other platforms, Probe calls compile to no-ops with no metadata.
//
// # Usage
//
// Both provider and name must be compile-time string literals:
//
//	usdt.Probe("myapp", "request_start")
//	// ... handle request ...
//	usdt.Probe1("myapp", "request_end", statusCode)
//
// Arguments must be integer types or unsafe.Pointer (see [USDTArg]).
// To pass a string, use unsafe.Pointer with unsafe.StringData:
//
//	usdt.Probe2("myapp", "transfer",
//	    unsafe.Pointer(unsafe.StringData(method)), int64(len(method)))
//
// # Tracing
//
// List probes:
//
//	bpftrace -l 'usdt:./myapp:*'
//
// Count hits:
//
//	bpftrace -e 'usdt:./myapp:myapp:request_start { @ = count() }' -c ./myapp
//
// Read arguments:
//
//	bpftrace -e 'usdt:./myapp:myapp:request_end { printf("status=%d\n", arg0) }' -c ./myapp
//
// # Argdesc format
//
// The .note.stapsdt section encodes each argument as a SystemTap SDT
// descriptor: "[size]@[reg]", space-separated. Negative size means
// signed. Examples: "-4@%eax" (int32 in eax), "8@%rsi" (uint64 in rsi),
// "-4@w0" (int32 in ARM64 w0).
//
// # Known limitations
//
//   - No semaphores: probes cannot be enabled/disabled at runtime.
//   - String literals only: provider and name must be compile-time constants.
//   - Maximum 4 arguments per probe (Probe1-Probe4).
//   - External linking (-linkmode=external) does not emit .note.stapsdt;
//     use internal linking (default) or -linkmode=internal.
package usdt

import "unsafe"

// Probe emits a USDT probe with the given provider and name.
//
// Both provider and name MUST be compile-time string literals; using
// variables will result in a compile-time error.
func Probe(provider, name string) {
	// No-op fallback for unsupported platforms.
}

// USDTArg constrains the types that can be passed as USDT probe arguments.
// Only integer types and unsafe.Pointer are supported since they can be
// passed in registers and encoded in the argdesc format.
type USDTArg interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		unsafe.Pointer
}

// Probe1 emits a USDT probe with one argument.
func Probe1[T USDTArg](provider, name string, arg1 T) {
}

// Probe2 emits a USDT probe with two arguments.
func Probe2[T1, T2 USDTArg](provider, name string, arg1 T1, arg2 T2) {
}

// Probe3 emits a USDT probe with three arguments.
func Probe3[T1, T2, T3 USDTArg](provider, name string, arg1 T1, arg2 T2, arg3 T3) {
}

// Probe4 emits a USDT probe with four arguments.
func Probe4[T1, T2, T3, T4 USDTArg](provider, name string, arg1 T1, arg2 T2, arg3 T3, arg4 T4) {
}
