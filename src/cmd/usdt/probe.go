// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Probe represents a USDT probe parsed from an ELF binary.
type Probe struct {
	Address   uint64 // Probe instruction address
	Base      uint64 // Base address for relocation
	Semaphore uint64 // Semaphore address (0 if not used)
	Provider  string // Provider name (e.g., "net_http")
	Name      string // Probe name (e.g., "server_request_start")
	ArgDesc   string // Argument descriptor (e.g., "-4@%eax 8@%rsi")
}

// ParseProbes reads USDT probes from an ELF binary.
func ParseProbes(filename string) ([]Probe, error) {
	f, err := elf.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open ELF file: %w", err)
	}
	defer f.Close()

	// Find .note.stapsdt section
	var noteSection *elf.Section
	for _, s := range f.Sections {
		if s.Name == ".note.stapsdt" {
			noteSection = s
			break
		}
	}

	if noteSection == nil {
		return nil, nil // No probes in this binary
	}

	data, err := noteSection.Data()
	if err != nil {
		return nil, fmt.Errorf("read .note.stapsdt: %w", err)
	}

	return parseNoteSection(data, f.ByteOrder, f.Class == elf.ELFCLASS64)
}

func parseNoteSection(data []byte, order binary.ByteOrder, is64bit bool) ([]Probe, error) {
	var probes []Probe
	offset := 0

	for offset < len(data) {
		if offset+12 > len(data) {
			break
		}

		// Note header
		namesz := order.Uint32(data[offset:])
		descsz := order.Uint32(data[offset+4:])
		noteType := order.Uint32(data[offset+8:])
		offset += 12

		// Verify this is a stapsdt note
		if noteType != 3 { // NT_STAPSDT
			// Skip this note
			offset += int(align4(namesz)) + int(align4(descsz))
			continue
		}

		// Read name (should be "stapsdt\0")
		if offset+int(align4(namesz)) > len(data) {
			break
		}
		name := string(data[offset : offset+int(namesz)-1]) // -1 to exclude null
		offset += int(align4(namesz))

		if name != "stapsdt" {
			// Skip this note
			offset += int(align4(descsz))
			continue
		}

		// Read descriptor
		if offset+int(descsz) > len(data) {
			break
		}
		descData := data[offset : offset+int(descsz)]
		offset += int(align4(descsz))

		probe, err := parseProbeDescriptor(descData, order, is64bit)
		if err != nil {
			return nil, fmt.Errorf("parse probe descriptor: %w", err)
		}
		probes = append(probes, probe)
	}

	return probes, nil
}

func parseProbeDescriptor(data []byte, order binary.ByteOrder, is64bit bool) (Probe, error) {
	var probe Probe

	if is64bit {
		if len(data) < 24 {
			return probe, fmt.Errorf("descriptor too short: %d bytes", len(data))
		}
		probe.Address = order.Uint64(data[0:8])
		probe.Base = order.Uint64(data[8:16])
		probe.Semaphore = order.Uint64(data[16:24])
		data = data[24:]
	} else {
		if len(data) < 12 {
			return probe, fmt.Errorf("descriptor too short: %d bytes", len(data))
		}
		probe.Address = uint64(order.Uint32(data[0:4]))
		probe.Base = uint64(order.Uint32(data[4:8]))
		probe.Semaphore = uint64(order.Uint32(data[8:12]))
		data = data[12:]
	}

	// Parse null-terminated strings
	strings := splitNullTerminated(data)
	if len(strings) >= 1 {
		probe.Provider = strings[0]
	}
	if len(strings) >= 2 {
		probe.Name = strings[1]
	}
	if len(strings) >= 3 {
		probe.ArgDesc = strings[2]
	}

	return probe, nil
}

func splitNullTerminated(data []byte) []string {
	var result []string
	start := 0
	for i, b := range data {
		if b == 0 {
			result = append(result, string(data[start:i]))
			start = i + 1
		}
	}
	return result
}

func align4(n uint32) uint32 {
	return (n + 3) &^ 3
}

// FormatProbeTable returns a formatted table of probes.
func FormatProbeTable(probes []Probe, w io.Writer) {
	if len(probes) == 0 {
		fmt.Fprintln(w, "No USDT probes found.")
		return
	}

	// Header
	fmt.Fprintf(w, "%-20s %-30s %-18s %s\n", "PROVIDER", "NAME", "ADDRESS", "ARGUMENTS")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 90))

	for _, p := range probes {
		args := p.ArgDesc
		if args == "" {
			args = "(none)"
		}
		fmt.Fprintf(w, "%-20s %-30s 0x%-16x %s\n", p.Provider, p.Name, p.Address, args)
	}
}
