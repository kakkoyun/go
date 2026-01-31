// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"os"
)

func cmdValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose output")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: go tool usdt validate [flags] <binary>

Validate USDT probe placement in a Go binary.

This command checks:
  - Probe addresses are within the text segment
  - Probe metadata is well-formed
  - NOP instructions are present at probe sites

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
	errors := validate(filename, *verbose)

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation failed with %d error(s):\n", len(errors))
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("Validation passed.")
}

func validate(filename string, verbose bool) []string {
	var errors []string

	f, err := elf.Open(filename)
	if err != nil {
		return []string{fmt.Sprintf("failed to open ELF file: %v", err)}
	}
	defer f.Close()

	probes, err := ParseProbes(filename)
	if err != nil {
		return []string{fmt.Sprintf("failed to parse probes: %v", err)}
	}

	if len(probes) == 0 {
		if verbose {
			fmt.Println("No USDT probes found in binary.")
		}
		return nil
	}

	if verbose {
		fmt.Printf("Found %d USDT probe(s)\n", len(probes))
	}

	// Find text segment bounds
	var textStart, textEnd uint64
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_LOAD && prog.Flags&elf.PF_X != 0 {
			textStart = prog.Vaddr
			textEnd = prog.Vaddr + prog.Memsz
			break
		}
	}

	if textStart == 0 && textEnd == 0 {
		return []string{"could not find executable segment"}
	}

	if verbose {
		fmt.Printf("Text segment: 0x%x - 0x%x\n", textStart, textEnd)
	}

	// Validate each probe
	for i, p := range probes {
		if verbose {
			fmt.Printf("Validating probe %d: %s:%s at 0x%x\n", i+1, p.Provider, p.Name, p.Address)
		}

		// Check probe address is in text segment
		if p.Address < textStart || p.Address >= textEnd {
			errors = append(errors, fmt.Sprintf("probe %s:%s address 0x%x is outside text segment [0x%x, 0x%x)",
				p.Provider, p.Name, p.Address, textStart, textEnd))
			continue
		}

		// Check provider and name are not empty
		if p.Provider == "" {
			errors = append(errors, fmt.Sprintf("probe at 0x%x has empty provider name", p.Address))
		}
		if p.Name == "" {
			errors = append(errors, fmt.Sprintf("probe %s at 0x%x has empty probe name", p.Provider, p.Address))
		}

		// Validate NOP instruction at probe site
		if err := validateNOP(f, p); err != nil {
			errors = append(errors, fmt.Sprintf("probe %s:%s at 0x%x: %v", p.Provider, p.Name, p.Address, err))
		} else if verbose {
			fmt.Printf("  NOP instruction verified at probe site\n")
		}
	}

	return errors
}

func validateNOP(f *elf.File, p Probe) error {
	// Find the section containing the probe address
	for _, sect := range f.Sections {
		if sect.Addr <= p.Address && p.Address < sect.Addr+sect.Size {
			if sect.Type != elf.SHT_PROGBITS {
				continue
			}

			data, err := sect.Data()
			if err != nil {
				return fmt.Errorf("failed to read section data: %v", err)
			}

			offset := p.Address - sect.Addr
			if offset >= uint64(len(data)) {
				return fmt.Errorf("probe offset beyond section data")
			}

			// Check for NOP instruction based on architecture
			switch f.Machine {
			case elf.EM_X86_64:
				// x86-64 NOP is 0x90
				if data[offset] != 0x90 {
					return fmt.Errorf("expected NOP (0x90) at probe site, found 0x%02x", data[offset])
				}
			case elf.EM_AARCH64:
				// ARM64 NOP is 0xd503201f (little-endian: 1f 20 03 d5)
				if offset+3 >= uint64(len(data)) {
					return fmt.Errorf("not enough data for ARM64 instruction")
				}
				instr := uint32(data[offset]) | uint32(data[offset+1])<<8 |
					uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
				if instr != 0xd503201f {
					return fmt.Errorf("expected NOP (0xd503201f) at probe site, found 0x%08x", instr)
				}
			default:
				// Skip NOP check for unknown architectures
				return nil
			}
			return nil
		}
	}

	return fmt.Errorf("probe address not found in any section")
}
