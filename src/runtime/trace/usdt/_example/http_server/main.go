// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP_server demonstrates USDT probes with arguments in an HTTP server.
//
// Build and run:
//
//	GOOS=linux GOARCH=amd64 go build -o http_server .
//
// Find probe addresses:
//
//	readelf -n ./http_server | grep -E "(Provider|Name|Location|Arguments)"
//
// Example output:
//
//	Provider: myserver
//	Name: request_end
//	Location: 0x00000000004a1234, ...
//	Arguments: -4@%eax 8@%rcx
//
// Trace with bpftrace:
//
//	sudo bpftrace -e '
//	  uprobe:./http_server:0x<request_end_addr> {
//	    printf("status=%d latency=%dus\n", arg0, arg1);
//	  }
//	' -c ./http_server
package main

import (
	"fmt"
	"io"
	"net/http"
	"runtime/trace/usdt"
	"time"
	"unsafe"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Probe at request start with method pointer and path length
		methodPtr := unsafe.Pointer(unsafe.StringData(r.Method))
		usdt.Probe2("myserver", "request_start", methodPtr, int64(len(r.Method)))

		// Handle the request
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Hello, World!")

		// Probe at request end with status code and latency
		latencyUs := time.Since(start).Microseconds()
		usdt.Probe2("myserver", "request_end", int32(http.StatusOK), latencyUs)
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		usdt.Probe("myserver", "slow_request_start")

		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Slow response")

		latencyUs := time.Since(start).Microseconds()
		usdt.Probe2("myserver", "request_end", int32(http.StatusOK), latencyUs)
	})

	server := &http.Server{Addr: ":8080", Handler: mux}

	// Start server in background
	go func() {
		fmt.Println("Server starting on :8080")
		server.ListenAndServe()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Make some test requests
	for i := 0; i < 5; i++ {
		resp, err := http.Get("http://localhost:8080/")
		if err != nil {
			fmt.Printf("Request failed: %v\n", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("Request %d: status=%d\n", i+1, resp.StatusCode)
	}

	// Make a slow request
	resp, _ := http.Get("http://localhost:8080/slow")
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("Slow request: status=%d\n", resp.StatusCode)
	}

	server.Close()
}
