// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Usdt lists and analyzes USDT (User Statically-Defined Tracing) probes in Go binaries.
//
// Usage:
//
//	go tool usdt <command> [arguments]
//
// Commands:
//
//	list      list USDT probes in a binary
//	bpftrace  generate bpftrace script for probes
//	validate  validate probe placement in a binary
//
// Use "go tool usdt <command> -help" for more information about a command.
package main

import (
	"flag"
	"fmt"
	"os"

	"cmd/internal/telemetry/counter"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: go tool usdt <command> [arguments]

Commands:
    list      list USDT probes in a binary
    bpftrace  generate bpftrace script for probes
    validate  validate probe placement in a binary

Use "go tool usdt <command> -help" for more information about a command.
`)
	os.Exit(2)
}

func main() {
	counter.Open()
	flag.Usage = usage

	if len(os.Args) < 2 {
		usage()
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	counter.Inc("usdt/invocations")

	switch cmd {
	case "list":
		cmdList(args)
	case "bpftrace":
		cmdBpftrace(args)
	case "validate":
		cmdValidate(args)
	case "-h", "-help", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "usdt: unknown command %q\n", cmd)
		fmt.Fprintf(os.Stderr, "Run 'go tool usdt' for usage.\n")
		os.Exit(2)
	}
}
