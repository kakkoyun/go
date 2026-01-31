// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pprof

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"internal/profile"
	"internal/testenv"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDescriptions checks that the profile names under runtime/pprof package
// have a key in the description map.
func TestDescriptions(t *testing.T) {
	for _, p := range pprof.Profiles() {
		_, ok := profileDescriptions[p.Name()]
		if ok != true {
			t.Errorf("%s does not exist in profileDescriptions map\n", p.Name())
		}
	}
}

func TestHandlers(t *testing.T) {
	testCases := []struct {
		path               string
		handler            http.HandlerFunc
		statusCode         int
		contentType        string
		contentDisposition string
		resp               []byte
	}{
		{"/debug/pprof/<script>scripty<script>", Index, http.StatusNotFound, "text/plain; charset=utf-8", "", []byte("Unknown profile\n")},
		{"/debug/pprof/heap", Index, http.StatusOK, "application/octet-stream", `attachment; filename="heap"`, nil},
		{"/debug/pprof/heap?debug=1", Index, http.StatusOK, "text/plain; charset=utf-8", "", nil},
		{"/debug/pprof/cmdline", Cmdline, http.StatusOK, "text/plain; charset=utf-8", "", nil},
		{"/debug/pprof/profile?seconds=1", Profile, http.StatusOK, "application/octet-stream", `attachment; filename="profile"`, nil},
		{"/debug/pprof/symbol", Symbol, http.StatusOK, "text/plain; charset=utf-8", "", nil},
		{"/debug/pprof/trace", Trace, http.StatusOK, "application/octet-stream", `attachment; filename="trace"`, nil},
		{"/debug/pprof/mutex", Index, http.StatusOK, "application/octet-stream", `attachment; filename="mutex"`, nil},
		{"/debug/pprof/block?seconds=1", Index, http.StatusOK, "application/octet-stream", `attachment; filename="block-delta"`, nil},
		{"/debug/pprof/goroutine?seconds=1", Index, http.StatusOK, "application/octet-stream", `attachment; filename="goroutine-delta"`, nil},
		{"/debug/pprof/", Index, http.StatusOK, "text/html; charset=utf-8", "", []byte("Types of profiles available:")},
	}
	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com"+tc.path, nil)
			w := httptest.NewRecorder()
			tc.handler(w, req)

			resp := w.Result()
			if got, want := resp.StatusCode, tc.statusCode; got != want {
				t.Errorf("status code: got %d; want %d", got, want)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("when reading response body, expected non-nil err; got %v", err)
			}
			if got, want := resp.Header.Get("X-Content-Type-Options"), "nosniff"; got != want {
				t.Errorf("X-Content-Type-Options: got %q; want %q", got, want)
			}
			if got, want := resp.Header.Get("Content-Type"), tc.contentType; got != want {
				t.Errorf("Content-Type: got %q; want %q", got, want)
			}
			if got, want := resp.Header.Get("Content-Disposition"), tc.contentDisposition; got != want {
				t.Errorf("Content-Disposition: got %q; want %q", got, want)
			}

			if resp.StatusCode == http.StatusOK {
				return
			}
			if got, want := resp.Header.Get("X-Go-Pprof"), "1"; got != want {
				t.Errorf("X-Go-Pprof: got %q; want %q", got, want)
			}
			if !bytes.Equal(body, tc.resp) {
				t.Errorf("response: got %q; want %q", body, tc.resp)
			}
		})
	}
}

var Sink uint32

func mutexHog1(mu1, mu2 *sync.Mutex, start time.Time, dt time.Duration) {
	atomic.AddUint32(&Sink, 1)
	for time.Since(start) < dt {
		// When using gccgo the loop of mutex operations is
		// not preemptible. This can cause the loop to block a GC,
		// causing the time limits in TestDeltaContentionz to fail.
		// Since this loop is not very realistic, when using
		// gccgo add preemption points 100 times a second.
		t1 := time.Now()
		for time.Since(start) < dt && time.Since(t1) < 10*time.Millisecond {
			mu1.Lock()
			mu2.Lock()
			mu1.Unlock()
			mu2.Unlock()
		}
		if runtime.Compiler == "gccgo" {
			runtime.Gosched()
		}
	}
}

// mutexHog2 is almost identical to mutexHog but we keep them separate
// in order to distinguish them with function names in the stack trace.
// We make them slightly different, using Sink, because otherwise
// gccgo -c opt will merge them.
func mutexHog2(mu1, mu2 *sync.Mutex, start time.Time, dt time.Duration) {
	atomic.AddUint32(&Sink, 2)
	for time.Since(start) < dt {
		// See comment in mutexHog.
		t1 := time.Now()
		for time.Since(start) < dt && time.Since(t1) < 10*time.Millisecond {
			mu1.Lock()
			mu2.Lock()
			mu1.Unlock()
			mu2.Unlock()
		}
		if runtime.Compiler == "gccgo" {
			runtime.Gosched()
		}
	}
}

// mutexHog starts multiple goroutines that runs the given hogger function for the specified duration.
// The hogger function will be given two mutexes to lock & unlock.
func mutexHog(duration time.Duration, hogger func(mu1, mu2 *sync.Mutex, start time.Time, dt time.Duration)) {
	start := time.Now()
	mu1 := new(sync.Mutex)
	mu2 := new(sync.Mutex)
	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			hogger(mu1, mu2, start, duration)
		}()
	}
	wg.Wait()
}

func TestDeltaProfile(t *testing.T) {
	if strings.HasPrefix(runtime.GOARCH, "arm") {
		testenv.SkipFlaky(t, 50218)
	}

	rate := runtime.SetMutexProfileFraction(1)
	defer func() {
		runtime.SetMutexProfileFraction(rate)
	}()

	// mutexHog1 will appear in non-delta mutex profile
	// if the mutex profile works.
	mutexHog(20*time.Millisecond, mutexHog1)

	// If mutexHog1 does not appear in the mutex profile,
	// skip this test. Mutex profile is likely not working,
	// so is the delta profile.

	p, err := query("/debug/pprof/mutex")
	if err != nil {
		t.Skipf("mutex profile is unsupported: %v", err)
	}

	if !seen(p, "mutexHog1") {
		t.Skipf("mutex profile is not working: %v", p)
	}

	// causes mutexHog2 call stacks to appear in the mutex profile.
	done := make(chan bool)
	go func() {
		for {
			mutexHog(20*time.Millisecond, mutexHog2)
			select {
			case <-done:
				done <- true
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	defer func() { // cleanup the above goroutine.
		done <- true
		<-done // wait for the goroutine to exit.
	}()

	for _, d := range []int{1, 4, 16, 32} {
		endpoint := fmt.Sprintf("/debug/pprof/mutex?seconds=%d", d)
		p, err := query(endpoint)
		if err != nil {
			t.Fatalf("failed to query %q: %v", endpoint, err)
		}
		if !seen(p, "mutexHog1") && seen(p, "mutexHog2") && p.DurationNanos > 0 {
			break // pass
		}
		if d == 32 {
			t.Errorf("want mutexHog2 but no mutexHog1 in the profile, and non-zero p.DurationNanos, got %v", p)
		}
	}
	p, err = query("/debug/pprof/mutex")
	if err != nil {
		t.Fatalf("failed to query mutex profile: %v", err)
	}
	if !seen(p, "mutexHog1") || !seen(p, "mutexHog2") {
		t.Errorf("want both mutexHog1 and mutexHog2 in the profile, got %v", p)
	}
}

var srv = httptest.NewServer(nil)

func query(endpoint string) (*profile.Profile, error) {
	url := srv.URL + endpoint
	r, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %q: %v", url, err)
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %q: %v", url, r.Status)
	}

	b, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read and parse the result from %q: %v", url, err)
	}
	return profile.Parse(bytes.NewBuffer(b))
}

// seen returns true if the profile includes samples whose stacks include
// the specified function name (fname).
func seen(p *profile.Profile, fname string) bool {
	locIDs := map[*profile.Location]bool{}
	for _, loc := range p.Location {
		for _, l := range loc.Line {
			if strings.Contains(l.Function.Name, fname) {
				locIDs[loc] = true
				break
			}
		}
	}
	for _, sample := range p.Sample {
		for _, loc := range sample.Location {
			if locIDs[loc] {
				return true
			}
		}
	}
	return false
}

// TestDeltaProfileEmptyBase validates that we still receive a valid delta
// profile even if the base contains no samples.
//
// Regression test for https://go.dev/issue/64566.
func TestDeltaProfileEmptyBase(t *testing.T) {
	if testing.Short() {
		// Delta profile collection has a 1s minimum.
		t.Skip("skipping in -short mode")
	}

	testenv.MustHaveGoRun(t)

	gotool, err := testenv.GoTool()
	if err != nil {
		t.Fatalf("error finding go tool: %v", err)
	}

	out, err := testenv.Command(t, gotool, "run", filepath.Join("testdata", "delta_mutex.go")).CombinedOutput()
	if err != nil {
		t.Fatalf("error running profile collection: %v\noutput: %s", err, out)
	}

	// Log the binary output for debugging failures.
	b64 := make([]byte, base64.StdEncoding.EncodedLen(len(out)))
	base64.StdEncoding.Encode(b64, out)
	t.Logf("Output in base64.StdEncoding: %s", b64)

	p, err := profile.Parse(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("Parse got err %v want nil", err)
	}

	t.Logf("Output as parsed Profile: %s", p)

	if len(p.SampleType) != 2 {
		t.Errorf("len(p.SampleType) got %d want 2", len(p.SampleType))
	}
	if p.SampleType[0].Type != "contentions" {
		t.Errorf(`p.SampleType[0].Type got %q want "contentions"`, p.SampleType[0].Type)
	}
	if p.SampleType[0].Unit != "count" {
		t.Errorf(`p.SampleType[0].Unit got %q want "count"`, p.SampleType[0].Unit)
	}
	if p.SampleType[1].Type != "delay" {
		t.Errorf(`p.SampleType[1].Type got %q want "delay"`, p.SampleType[1].Type)
	}
	if p.SampleType[1].Unit != "nanoseconds" {
		t.Errorf(`p.SampleType[1].Unit got %q want "nanoseconds"`, p.SampleType[1].Unit)
	}

	if p.PeriodType == nil {
		t.Fatal("p.PeriodType got nil want not nil")
	}
	if p.PeriodType.Type != "contentions" {
		t.Errorf(`p.PeriodType.Type got %q want "contentions"`, p.PeriodType.Type)
	}
	if p.PeriodType.Unit != "count" {
		t.Errorf(`p.PeriodType.Unit got %q want "count"`, p.PeriodType.Unit)
	}
}

// TestTraceStream tests the trace streaming endpoint.
func TestTraceStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(Trace))
	defer ts.Close()

	// Create a context with timeout to limit streaming duration
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Request streaming trace with short interval
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"?stream=1&interval=0.5", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check headers
	if got, want := resp.Header.Get("Content-Type"), "application/octet-stream"; got != want {
		t.Errorf("Content-Type: got %q; want %q", got, want)
	}
	if got, want := resp.Header.Get("X-Content-Type-Options"), "nosniff"; got != want {
		t.Errorf("X-Content-Type-Options: got %q; want %q", got, want)
	}

	// Read at least one snapshot
	snapshots := 0
	for snapshots < 2 {
		// Read length prefix (8 bytes, little-endian)
		var lenBuf [8]byte
		_, err := io.ReadFull(resp.Body, lenBuf[:])
		if err != nil {
			if ctx.Err() != nil {
				// Context canceled, expected
				break
			}
			t.Fatalf("failed to read length prefix: %v", err)
		}

		length := binary.LittleEndian.Uint64(lenBuf[:])
		if length == 0 {
			t.Fatal("got zero-length snapshot")
		}

		// Read trace data
		data := make([]byte, length)
		_, err = io.ReadFull(resp.Body, data)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			t.Fatalf("failed to read trace data: %v", err)
		}

		// Verify trace header magic bytes "go 1." prefix
		if !bytes.HasPrefix(data, []byte("go 1.")) {
			t.Errorf("snapshot %d: invalid trace header, got prefix %q", snapshots, data[:min(10, len(data))])
		}

		snapshots++
		t.Logf("received snapshot %d: %d bytes", snapshots, length)
	}

	if snapshots < 1 {
		t.Error("expected at least 1 snapshot")
	}
}

// TestTraceStreamClientDisconnect tests that the streaming endpoint handles client disconnect.
func TestTraceStreamClientDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(Trace))
	defer ts.Close()

	// Create a context that we'll cancel immediately after connecting
	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"?stream=1&interval=1", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}

	// Cancel the context to simulate client disconnect
	cancel()
	resp.Body.Close()

	// Give the server time to clean up
	time.Sleep(100 * time.Millisecond)

	// If we get here without hanging, the test passes
}

// TestTraceStreamParameters tests that streaming parameters are parsed correctly.
func TestTraceStreamParameters(t *testing.T) {
	testCases := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{"default params", "stream=1", false},
		{"custom interval", "stream=1&interval=2", false},
		{"custom maxbytes", "stream=1&maxbytes=5242880", false},
		{"custom minage", "stream=1&minage=5", false},
		{"all custom", "stream=1&interval=0.5&maxbytes=1048576&minage=2", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(Trace))
			defer ts.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"?"+tc.params, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if tc.wantErr {
				if resp.StatusCode == http.StatusOK {
					t.Error("expected error status code, got OK")
				}
				return
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
			}
		})
	}
}

// TestTraceStreamFilter tests the filter parameter for trace streaming.
func TestTraceStreamFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	testCases := []struct {
		name   string
		filter string
	}{
		{"numeric filter", "stream=1&filter=3"},
		{"http filter", "stream=1&filter=http"},
		{"http,sql filter", "stream=1&filter=http,sql"},
		{"all filter", "stream=1&filter=all"},
		{"core,custom filter", "stream=1&filter=core,custom"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(Trace))
			defer ts.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"?"+tc.filter, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
				return
			}

			// Read at least one snapshot to verify streaming works with filter
			var lenBuf [8]byte
			_, err = io.ReadFull(resp.Body, lenBuf[:])
			if err != nil && ctx.Err() == nil {
				t.Fatalf("failed to read length prefix: %v", err)
			}

			length := binary.LittleEndian.Uint64(lenBuf[:])
			if length == 0 {
				t.Error("got zero-length snapshot")
			}

			t.Logf("filter %q: received snapshot of %d bytes", tc.filter, length)
		})
	}
}
