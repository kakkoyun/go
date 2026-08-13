// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "output in JSON format")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: go tool usdt list [flags] <binary>

List USDT probes embedded in a Go binary.

Flags:
`)
		fs.PrintDefaults()
	}

	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	filename := fs.Arg(0)
	probes, err := ParseProbes(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "usdt list: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(probes); err != nil {
			fmt.Fprintf(os.Stderr, "usdt list: %v\n", err)
			os.Exit(1)
		}
	} else {
		FormatProbeTable(probes, os.Stdout)
	}
}
