// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Basic demonstrates simple USDT probe usage.
//
// Build and run:
//
//	go build -o basic .
//	./basic
//
// Find probe addresses:
//
//	readelf -n ./basic | grep -E "(Provider|Name|Location)"
//
// Trace with bpftrace (requires Linux and root):
//
//	sudo bpftrace -e 'uprobe:./basic:0x<ADDR> { printf("work_start\n"); }'
package main

import "runtime/trace/usdt"

func doWork() {
	usdt.Probe("myapp", "work_start")
	// ... do some work ...
	usdt.Probe("myapp", "work_end")
}

func main() {
	for i := 0; i < 10; i++ {
		doWork()
	}
}
