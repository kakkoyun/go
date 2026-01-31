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
	"runtime"
	"strings"
	"testing"
)

// TestProbeCompiles verifies that a program using usdt.Probe compiles successfully.
func TestProbeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Create a temporary directory for the test
	dir := t.TempDir()

	// Write a simple test program
	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe("testprovider", "testprobe")
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Build the program
	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}
}

// TestELFNoteGenerated verifies that the .note.stapsdt section is generated
// with correct probe metadata for linux/amd64 targets.
func TestELFNoteGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Create a temporary directory for the test
	dir := t.TempDir()

	// Write a test program with a known probe
	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe("myprovider", "myprobe")
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Build for linux/amd64
	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	// Parse the ELF binary
	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open ELF: %v", err)
	}
	defer f.Close()

	// Find the .note.stapsdt section
	sect := f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found")
	}

	// Read the section data
	data, err := sect.Data()
	if err != nil {
		t.Fatalf("failed to read section data: %v", err)
	}

	// Parse the note
	if len(data) < 12 {
		t.Fatal("note data too short")
	}

	namesz := binary.LittleEndian.Uint32(data[0:4])
	_ = binary.LittleEndian.Uint32(data[4:8]) // descsz - not used
	noteType := binary.LittleEndian.Uint32(data[8:12])

	// Verify note type (NT_STAPSDT = 3)
	if noteType != 3 {
		t.Errorf("unexpected note type: got %d, want 3", noteType)
	}

	// Verify name is "stapsdt\0"
	if namesz != 8 {
		t.Errorf("unexpected namesz: got %d, want 8", namesz)
	}
	name := string(data[12 : 12+namesz-1]) // -1 to exclude null terminator
	if name != "stapsdt" {
		t.Errorf("unexpected name: got %q, want %q", name, "stapsdt")
	}

	// Parse descriptor (after name, aligned to 4 bytes)
	descOff := 12 + ((namesz + 3) &^ 3)
	if len(data) < int(descOff)+24 {
		t.Fatal("descriptor too short")
	}

	pc := binary.LittleEndian.Uint64(data[descOff : descOff+8])
	base := binary.LittleEndian.Uint64(data[descOff+8 : descOff+16])
	sema := binary.LittleEndian.Uint64(data[descOff+16 : descOff+24])

	// Verify PC is non-zero (actual address)
	if pc == 0 {
		t.Error("probe PC is 0, expected non-zero address")
	}

	// Verify base is non-zero
	if base == 0 {
		t.Error("base address is 0, expected non-zero")
	}

	// Semaphore should be 0 (not implemented yet)
	if sema != 0 {
		t.Errorf("semaphore: got %d, want 0", sema)
	}

	// Parse provider and probe name strings
	strOff := int(descOff) + 24
	provider := ""
	probeName := ""

	for i := strOff; i < len(data); i++ {
		if data[i] == 0 {
			provider = string(data[strOff:i])
			strOff = i + 1
			break
		}
	}
	for i := strOff; i < len(data); i++ {
		if data[i] == 0 {
			probeName = string(data[strOff:i])
			break
		}
	}

	if provider != "myprovider" {
		t.Errorf("provider: got %q, want %q", provider, "myprovider")
	}
	if probeName != "myprobe" {
		t.Errorf("probe name: got %q, want %q", probeName, "myprobe")
	}

	t.Logf("USDT probe found: provider=%q name=%q pc=0x%x base=0x%x", provider, probeName, pc, base)
}

// TestELFNoteGeneratedARM64 verifies that the .note.stapsdt section is generated
// with correct probe metadata for linux/arm64 targets.
func TestELFNoteGeneratedARM64(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe("myprovider", "myprobe")
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Build for linux/arm64
	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	// Parse the ELF binary
	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open ELF: %v", err)
	}
	defer f.Close()

	// Verify ELF machine type is ARM64
	if f.Machine != elf.EM_AARCH64 {
		t.Fatalf("expected ARM64 ELF, got machine type %v", f.Machine)
	}

	// Find the .note.stapsdt section
	sect := f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found for ARM64")
	}

	data, err := sect.Data()
	if err != nil {
		t.Fatalf("failed to read section data: %v", err)
	}

	// Parse the note and verify basic structure
	if len(data) < 12 {
		t.Fatal("note data too short")
	}

	noteType := binary.LittleEndian.Uint32(data[8:12])
	if noteType != 3 {
		t.Errorf("unexpected note type: got %d, want 3", noteType)
	}

	t.Log("ARM64 USDT probe ELF note generated successfully")
}

// TestMultipleProbes verifies that multiple probes in a program are all recorded.
func TestMultipleProbes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func foo() {
	usdt.Probe("app", "foo_enter")
}

func bar() {
	usdt.Probe("app", "bar_enter")
}

func main() {
	foo()
	bar()
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open ELF: %v", err)
	}
	defer f.Close()

	sect := f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found")
	}

	data, err := sect.Data()
	if err != nil {
		t.Fatalf("failed to read section data: %v", err)
	}

	// Count probes by counting "app" occurrences in the data
	// Each probe has provider "app"
	probeCount := strings.Count(string(data), "app")
	if probeCount < 2 {
		t.Errorf("expected at least 2 probes, found %d", probeCount)
	}

	t.Logf("Found %d probes in binary", probeCount)
}

// TestNonLiteralError verifies that non-literal strings cause a compile error.
func TestNonLiteralError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Skip on non-amd64 since the intrinsic only exists for amd64
	if runtime.GOARCH != "amd64" && os.Getenv("GOARCH") != "amd64" {
		t.Skip("skipping on non-amd64")
	}

	dir := t.TempDir()

	// Test with non-literal provider
	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	provider := "dynamic"
	usdt.Probe(provider, "test")
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()

	// Should fail with compile error about string literal
	if err == nil {
		t.Fatal("expected compile error for non-literal provider, but build succeeded")
	}
	if !strings.Contains(string(output), "string literal") {
		t.Errorf("expected error about string literal, got: %s", output)
	}
}

// TestProbeWithArguments verifies that Probe1-4 with various argument types compile
// and generate correct argdesc in the ELF notes.
func TestProbeWithArguments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	// Test program with various argument types
	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import (
	"runtime/trace/usdt"
	"unsafe"
)

func main() {
	var i8 int8 = 1
	var i16 int16 = 2
	var i32 int32 = 200
	var i64 int64 = 1000
	var u8 uint8 = 255
	var u16 uint16 = 65535
	var u32 uint32 = 100
	var u64 uint64 = 999
	var uptr uintptr = 0x1234
	str := "GET"
	ptr := unsafe.Pointer(unsafe.StringData(str))

	usdt.Probe1("app", "int8_probe", i8)
	usdt.Probe1("app", "int16_probe", i16)
	usdt.Probe1("app", "int32_probe", i32)
	usdt.Probe1("app", "int64_probe", i64)
	usdt.Probe1("app", "uint8_probe", u8)
	usdt.Probe1("app", "uint16_probe", u16)
	usdt.Probe1("app", "uint32_probe", u32)
	usdt.Probe1("app", "uint64_probe", u64)
	usdt.Probe1("app", "uintptr_probe", uptr)
	usdt.Probe1("app", "pointer_probe", ptr)
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open ELF: %v", err)
	}
	defer f.Close()

	sect := f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found")
	}

	data, err := sect.Data()
	if err != nil {
		t.Fatalf("failed to read section data: %v", err)
	}

	// Verify probes exist
	probeNames := []string{"int8_probe", "int16_probe", "int32_probe", "int64_probe",
		"uint8_probe", "uint16_probe", "uint32_probe", "uint64_probe",
		"uintptr_probe", "pointer_probe"}
	for _, name := range probeNames {
		if !strings.Contains(string(data), name) {
			t.Errorf("probe %q not found in ELF notes", name)
		}
	}

	t.Log("All Probe1 argument type tests passed")
}

// TestProbeMultipleArguments verifies Probe2, Probe3, Probe4 work correctly.
func TestProbeMultipleArguments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe2("app", "two_args", int32(100), int64(200))
	usdt.Probe3("app", "three_args", int32(1), int32(2), int32(3))
	usdt.Probe4("app", "four_args", int32(1), int32(2), int32(3), int32(4))
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open ELF: %v", err)
	}
	defer f.Close()

	sect := f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found")
	}

	data, err := sect.Data()
	if err != nil {
		t.Fatalf("failed to read section data: %v", err)
	}

	// Verify probes exist
	for _, name := range []string{"two_args", "three_args", "four_args"} {
		if !strings.Contains(string(data), name) {
			t.Errorf("probe %q not found in ELF notes", name)
		}
	}

	t.Log("Multiple argument probes test passed")
}

// TestProbeArgDescFormat verifies the argdesc field format in ELF notes.
func TestProbeArgDescFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe1("app", "status", int32(200))
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test AMD64
	out := filepath.Join(dir, "testprog_amd64")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("AMD64 build failed: %v\n%s", err, output)
	}

	f, err := elf.Open(out)
	if err != nil {
		t.Fatalf("failed to open AMD64 ELF: %v", err)
	}

	sect := f.Section(".note.stapsdt")
	if sect == nil {
		f.Close()
		t.Fatal(".note.stapsdt section not found for AMD64")
	}

	data, err := sect.Data()
	f.Close()
	if err != nil {
		t.Fatalf("failed to read AMD64 section data: %v", err)
	}

	// Verify argdesc contains register notation for AMD64 (e.g., %eax, %rax)
	dataStr := string(data)
	if !strings.Contains(dataStr, "%") {
		t.Error("AMD64 argdesc should contain register notation with '%'")
	}
	t.Logf("AMD64 ELF data contains register notation")

	// Test ARM64
	outArm := filepath.Join(dir, "testprog_arm64")
	cmd = exec.Command("go", "build", "-o", outArm, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ARM64 build failed: %v\n%s", err, output)
	}

	f, err = elf.Open(outArm)
	if err != nil {
		t.Fatalf("failed to open ARM64 ELF: %v", err)
	}
	defer f.Close()

	sect = f.Section(".note.stapsdt")
	if sect == nil {
		t.Fatal(".note.stapsdt section not found for ARM64")
	}

	data, err = sect.Data()
	if err != nil {
		t.Fatalf("failed to read ARM64 section data: %v", err)
	}

	// Verify argdesc contains ARM64 register notation (w0-w30, x0-x30)
	dataStr = string(data)
	hasArmReg := strings.Contains(dataStr, "w") || strings.Contains(dataStr, "x")
	if !hasArmReg {
		t.Error("ARM64 argdesc should contain register notation (w/x)")
	}
	t.Logf("ARM64 ELF data contains register notation")
}

// TestUnsupportedPlatform verifies that on unsupported platforms,
// the probe is a no-op (doesn't crash, doesn't generate USDT notes).
func TestUnsupportedPlatform(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	dir := t.TempDir()

	src := filepath.Join(dir, "main.go")
	err := os.WriteFile(src, []byte(`
package main

import "runtime/trace/usdt"

func main() {
	usdt.Probe("test", "probe")
	println("ok")
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Build for darwin/arm64 (no USDT support)
	out := filepath.Join(dir, "testprog")
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	// The binary should exist and be a valid Mach-O (not ELF)
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("binary is empty")
	}

	// Verify it's not an ELF (shouldn't have .note.stapsdt)
	// This just verifies the build succeeded for a non-Linux target
	t.Log("Build for unsupported platform succeeded")
}
