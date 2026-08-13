// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package usdt_test

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// probeCase is one row in the table-driven USDT test suite.
type probeCase struct {
	name        string
	src         string // Go source to build
	goos        string
	goarch      string
	wantBuildErr string // non-empty = expect build to fail with this substring
	wantNoNotes  bool   // true = binary must have no .note.stapsdt
	wantProbes   []probeExpect
}

// probeExpect describes a probe we expect to find (or not) in .note.stapsdt.
type probeExpect struct {
	provider string
	name     string
	// argdescChecks are optional functions that validate the argdesc string.
	// If nil, only provider/name presence is checked.
	argdescCheck func(t *testing.T, argdesc string)
}

func TestUSDT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	cases := []probeCase{
		// 1. Simple probe compiles for linux/amd64.
		{
			name:   "compiles_amd64",
			src:    `package main; import "runtime/trace/usdt"; func main() { usdt.Probe("tp", "tn") }`,
			goos:   "linux",
			goarch: "amd64",
			wantProbes: []probeExpect{
				{provider: "tp", name: "tn"},
			},
		},
		// 2. Simple probe compiles for linux/arm64.
		{
			name:   "compiles_arm64",
			src:    `package main; import "runtime/trace/usdt"; func main() { usdt.Probe("tp", "tn") }`,
			goos:   "linux",
			goarch: "arm64",
			wantProbes: []probeExpect{
				{provider: "tp", name: "tn"},
			},
		},
		// 3. Multiple probes in one binary.
		{
			name: "multiple_probes",
			src: `package main
import "runtime/trace/usdt"
func main() {
	usdt.Probe("app", "start")
	usdt.Probe("app", "end")
	usdt.Probe("app", "middle")
}`,
			goos:   "linux",
			goarch: "amd64",
			wantProbes: []probeExpect{
				{provider: "app", name: "start"},
				{provider: "app", name: "end"},
				{provider: "app", name: "middle"},
			},
		},
		// 4. Non-literal provider is a compile error.
		{
			name: "non_literal_error",
			src: `package main
import "runtime/trace/usdt"
func main() {
	p := "dynamic"
	usdt.Probe(p, "test")
}`,
			goos:        "linux",
			goarch:      "amd64",
			wantBuildErr: "string literal",
		},
		// 5. Probe1 with various integer types.
		{
			name: "probe1_int_types",
			src: `package main
import ("runtime/trace/usdt"; "unsafe")
func main() {
	var i8 int8 = 1; var i16 int16 = 2; var i32 int32 = 200; var i64 int64 = 1000
	var u8 uint8 = 255; var u16 uint16 = 65535; var u32 uint32 = 100; var u64 uint64 = 999
	var uptr uintptr = 0x1234
	str := "GET"; ptr := unsafe.Pointer(unsafe.StringData(str))
	usdt.Probe1("app", "p_i8", i8)
	usdt.Probe1("app", "p_i16", i16)
	usdt.Probe1("app", "p_i32", i32)
	usdt.Probe1("app", "p_i64", i64)
	usdt.Probe1("app", "p_u8", u8)
	usdt.Probe1("app", "p_u16", u16)
	usdt.Probe1("app", "p_u32", u32)
	usdt.Probe1("app", "p_u64", u64)
	usdt.Probe1("app", "p_ptr", ptr)
	_ = uptr
}`,
			goos:   "linux",
			goarch: "amd64",
			wantProbes: []probeExpect{
				{provider: "app", name: "p_i8", argdescCheck: checkSigned(1)},
				{provider: "app", name: "p_i16", argdescCheck: checkSigned(2)},
				{provider: "app", name: "p_i32", argdescCheck: checkSigned(4)},
				{provider: "app", name: "p_i64", argdescCheck: checkSigned(8)},
				{provider: "app", name: "p_u8", argdescCheck: checkUnsigned(1)},
				{provider: "app", name: "p_u16", argdescCheck: checkUnsigned(2)},
				{provider: "app", name: "p_u32", argdescCheck: checkUnsigned(4)},
				{provider: "app", name: "p_u64", argdescCheck: checkUnsigned(8)},
				{provider: "app", name: "p_ptr", argdescCheck: checkUnsigned(8)},
			},
		},
		// 6. Probe2/3/4 with multiple arguments.
		{
			name: "probe_multi_args",
			src: `package main
import ("runtime/trace/usdt"; "unsafe")
func main() {
	s := "GET"; ptr := unsafe.Pointer(unsafe.StringData(s))
	usdt.Probe2("app", "p2", ptr, int64(len(s)))
	usdt.Probe3("app", "p3", int8(-1), int16(-2), int64(-3))
	usdt.Probe4("app", "p4", uintptr(1), uintptr(2), uintptr(3), uintptr(4))
}`,
			goos:   "linux",
			goarch: "amd64",
			wantProbes: []probeExpect{
				{provider: "app", name: "p2"},
				{provider: "app", name: "p3"},
				{provider: "app", name: "p4"},
			},
		},
		// 7. Unsupported platform (darwin) builds with no notes.
		{
			name: "unsupported_darwin",
			src: `package main
import "runtime/trace/usdt"
func main() { usdt.Probe("test", "probe"); println("ok") }`,
			goos:       "darwin",
			goarch:     "arm64",
			wantNoNotes: true,
		},
		// 8. Unsupported platform (windows) builds with no notes.
		{
			name: "unsupported_windows",
			src: `package main
import "runtime/trace/usdt"
func main() { usdt.Probe("test", "probe") }`,
			goos:       "windows",
			goarch:     "amd64",
			wantNoNotes: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "main.go")
			if err := os.WriteFile(srcPath, []byte(tc.src), 0644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "testprog")
			cmd := exec.Command("go", "build", "-o", out, srcPath)
			cmd.Env = append(os.Environ(),
				"GOOS="+tc.goos, "GOARCH="+tc.goarch, "CGO_ENABLED=0")
			output, err := cmd.CombinedOutput()

			if tc.wantBuildErr != "" {
				if err == nil {
					t.Fatalf("expected build error containing %q, but build succeeded", tc.wantBuildErr)
				}
				if !strings.Contains(string(output), tc.wantBuildErr) {
					t.Fatalf("expected error containing %q, got: %s", tc.wantBuildErr, output)
				}
				return
			}
			if err != nil {
				t.Fatalf("build failed: %v\n%s", err, output)
			}

			if tc.wantNoNotes {
				// Not ELF — just verify the binary exists and is non-empty.
				info, err := os.Stat(out)
				if err != nil {
					t.Fatalf("stat: %v", err)
				}
				if info.Size() == 0 {
					t.Fatal("binary is empty")
				}
				// Try to open as ELF; should fail for darwin/windows.
				if f, err := elf.Open(out); err == nil {
					f.Close()
					if sec := f.Section(".note.stapsdt"); sec != nil {
						t.Errorf("unsupported platform has .note.stapsdt section")
					}
				}
				return
			}

			// Parse ELF notes and verify expected probes.
			f, err := elf.Open(out)
			if err != nil {
				t.Fatalf("open elf: %v", err)
			}
			defer f.Close()

			sec := f.Section(".note.stapsdt")
			if sec == nil {
				t.Fatal(".note.stapsdt section not found")
			}
			data, err := sec.Data()
			if err != nil {
				t.Fatalf("read section: %v", err)
			}

			probes := parseStapsdtNotes(data, f.ByteOrder)
			for _, want := range tc.wantProbes {
				found := false
				for _, p := range probes {
					if p.provider == want.provider && p.name == want.name {
						found = true
						if want.argdescCheck != nil {
							want.argdescCheck(t, p.argdesc)
						}
						break
					}
				}
				if !found {
					t.Errorf("expected probe %s:%s not found", want.provider, want.name)
				}
			}
			// No _fallback probes.
			for _, p := range probes {
				if p.name == "_fallback" || p.provider == "_fallback" {
					t.Errorf("found _fallback probe: %s:%s", p.provider, p.name)
				}
			}
		})
	}
}

// stapsdtProbe is a parsed .note.stapsdt probe.
type stapsdtProbe struct {
	addr     uint64
	base     uint64
	provider string
	name     string
	argdesc  string
}

// parseStapsdtNotes parses raw .note.stapsdt bytes.
func parseStapsdtNotes(data []byte, bo binary.ByteOrder) []stapsdtProbe {
	var probes []stapsdtProbe
	off := 0
	for off+12 <= len(data) {
		namesz := bo.Uint32(data[off:])
		descsz := bo.Uint32(data[off+4:])
		ntype := bo.Uint32(data[off+8:])
		namePad := int((namesz + 3) &^ 3)
		descStart := off + 12 + namePad
		descEnd := descStart + int(descsz)
		if ntype != 3 || descEnd > len(data) {
			break
		}
		desc := data[descStart:descEnd]
		p := stapsdtProbe{
			addr: bo.Uint64(desc[0:8]),
			base: bo.Uint64(desc[8:16]),
		}
		rest := desc[24:]
		strs := readNulStrings(rest, 3)
		p.provider, p.name, p.argdesc = strs[0], strs[1], strs[2]
		probes = append(probes, p)
		descPad := int((descsz + 3) &^ 3)
		off = descStart + descPad
	}
	return probes
}

func readNulStrings(b []byte, n int) []string {
	strs := make([]string, n)
	for i := 0; i < n; i++ {
		idx := -1
		for j, c := range b {
			if c == 0 {
				idx = j
				break
			}
		}
		if idx < 0 {
			strs[i] = string(b)
			break
		}
		strs[i] = string(b[:idx])
		b = b[idx+1:]
	}
	return strs
}

// checkSigned returns an argdesc check that verifies the first argument
// is signed with the given byte width.
func checkSigned(width int) func(t *testing.T, argdesc string) {
	return func(t *testing.T, argdesc string) {
		want := "-" + itoa(width) + "@"
		if !strings.HasPrefix(argdesc, want) {
			t.Errorf("expected argdesc to start with %q, got %q", want, argdesc)
		}
	}
}

// checkUnsigned returns an argdesc check that verifies the first argument
// is unsigned with the given byte width.
func checkUnsigned(width int) func(t *testing.T, argdesc string) {
	return func(t *testing.T, argdesc string) {
		want := itoa(width) + "@"
		if !strings.HasPrefix(argdesc, want) {
			t.Errorf("expected argdesc to start with %q, got %q", want, argdesc)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
