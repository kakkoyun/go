//go:build ignore

// Demo application for trace streaming endpoint.
// Run with: go run trace_stream_demo.go
//
// Then in another terminal:
//   # Stream all events:
//   curl -N "http://localhost:6060/debug/pprof/trace?stream=1&interval=2" | xxd | head -50
//
//   # Stream only HTTP events:
//   curl -N "http://localhost:6060/debug/pprof/trace?stream=1&filter=http&interval=2"
//
//   # Stream HTTP and SQL events:
//   curl -N "http://localhost:6060/debug/pprof/trace?stream=1&filter=http,sql&interval=2"

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime/trace"
	"time"
)

func main() {
	// Enable HTTP tracing
	trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP | trace.FilterCustom)

	// Simple handler that does some work
	http.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		fmt.Fprintf(w, "Hello, World!")
	})

	http.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		// Simulate slower work
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(w, "Slow response")
	})

	// Background goroutine that generates HTTP traffic
	go func() {
		time.Sleep(time.Second) // Wait for server to start
		client := &http.Client{Timeout: 5 * time.Second}
		for {
			// Make requests to generate trace events
			resp, err := client.Get("http://localhost:6060/api/hello")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			time.Sleep(500 * time.Millisecond)

			resp, err = client.Get("http://localhost:6060/api/slow")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	log.Println("Starting server on :6060")
	log.Println("Trace streaming endpoint: http://localhost:6060/debug/pprof/trace?stream=1")
	log.Println("With HTTP filter: http://localhost:6060/debug/pprof/trace?stream=1&filter=http")
	log.Fatal(http.ListenAndServe(":6060", nil))
}
