//go:build ignore

// Trace scraper - demonstrates consuming streaming trace data from pprof endpoint.
// Run with: go run trace_scraper.go [url]
//
// Default URL: http://localhost:6060/debug/pprof/trace?stream=1&filter=http&interval=2
//
// The scraper reads length-prefixed trace snapshots and saves them to files.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	outputDir := flag.String("output", "traces", "Directory to save trace files")
	maxSnapshots := flag.Int("max", 10, "Maximum number of snapshots to collect (0 = unlimited)")
	flag.Parse()

	url := "http://localhost:6060/debug/pprof/trace?stream=1&filter=http&interval=2"
	if flag.NArg() > 0 {
		url = flag.Arg(0)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	log.Printf("Connecting to %s", url)
	log.Printf("Saving traces to %s/", *outputDir)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Connect to streaming endpoint
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Server returned %d: %s", resp.StatusCode, body)
	}

	log.Println("Connected, receiving trace snapshots...")

	// Read snapshots in a goroutine
	snapshotCh := make(chan []byte)
	errCh := make(chan error, 1)

	go func() {
		for {
			// Read 8-byte length prefix (little-endian)
			var lenBuf [8]byte
			if _, err := io.ReadFull(resp.Body, lenBuf[:]); err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("failed to read length: %w", err)
				}
				close(snapshotCh)
				return
			}

			length := binary.LittleEndian.Uint64(lenBuf[:])
			if length == 0 {
				continue
			}

			// Read trace data
			data := make([]byte, length)
			if _, err := io.ReadFull(resp.Body, data); err != nil {
				errCh <- fmt.Errorf("failed to read trace data: %w", err)
				close(snapshotCh)
				return
			}

			snapshotCh <- data
		}
	}()

	count := 0
	for {
		select {
		case <-sigCh:
			log.Println("Interrupted, shutting down...")
			return

		case err := <-errCh:
			log.Printf("Error: %v", err)
			return

		case data, ok := <-snapshotCh:
			if !ok {
				log.Println("Stream ended")
				return
			}

			count++
			filename := filepath.Join(*outputDir, fmt.Sprintf("trace_%s_%03d.out",
				time.Now().Format("20060102_150405"), count))

			if err := os.WriteFile(filename, data, 0644); err != nil {
				log.Printf("Failed to save snapshot: %v", err)
				continue
			}

			log.Printf("Saved snapshot %d: %s (%d bytes)", count, filename, len(data))

			if *maxSnapshots > 0 && count >= *maxSnapshots {
				log.Printf("Reached max snapshots (%d), exiting", *maxSnapshots)
				return
			}
		}
	}
}
