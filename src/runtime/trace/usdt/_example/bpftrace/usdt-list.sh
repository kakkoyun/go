#!/bin/bash
# Copyright 2025 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# usdt-list.sh - List USDT probes in a Go binary and generate bpftrace commands
#
# Usage:
#   ./usdt-list.sh ./binary              # List all probes
#   ./usdt-list.sh ./binary net_http     # List only net_http probes
#   ./usdt-list.sh ./binary myapp -gen   # Generate bpftrace script

set -e

BINARY="$1"
PROVIDER="${2:-}"
GEN_SCRIPT=""

if [[ "$3" == "-gen" ]] || [[ "$2" == "-gen" ]]; then
    GEN_SCRIPT=1
    if [[ "$2" == "-gen" ]]; then
        PROVIDER=""
    fi
fi

if [[ -z "$BINARY" ]]; then
    echo "Usage: $0 <binary> [provider] [-gen]"
    echo ""
    echo "Options:"
    echo "  binary    Path to Go ELF binary"
    echo "  provider  Filter by provider name (optional)"
    echo "  -gen      Generate bpftrace script template"
    echo ""
    echo "Examples:"
    echo "  $0 ./myapp                  # List all probes"
    echo "  $0 ./myapp net_http         # List net_http probes only"
    echo "  $0 ./myapp myapp -gen       # Generate bpftrace script"
    exit 1
fi

if [[ ! -f "$BINARY" ]]; then
    echo "Error: Binary not found: $BINARY"
    exit 1
fi

# Check if it's an ELF file
if ! file "$BINARY" | grep -q ELF; then
    echo "Error: Not an ELF binary (USDT probes only work on Linux ELF binaries)"
    exit 1
fi

echo "=== USDT Probes in $BINARY ==="
echo ""

if [[ -n "$GEN_SCRIPT" ]]; then
    echo "#!/usr/bin/env bpftrace"
    echo "// Auto-generated bpftrace script for $BINARY"
    echo ""
    echo "BEGIN {"
    echo "    printf(\"Tracing USDT probes...\\n\\n\");"
    echo "}"
    echo ""
fi

# Parse readelf output
readelf -n "$BINARY" 2>/dev/null | awk -v binary="$BINARY" -v prov="$PROVIDER" -v gen="$GEN_SCRIPT" '
/Provider:/ { provider = $2 }
/Name:/ { name = $2 }
/Location:/ {
    addr = $2
    gsub(/,/, "", addr)
    next_is_args = 1
}
/Arguments:/ {
    args = ""
    for (i = 2; i <= NF; i++) {
        args = args (args ? " " : "") $i
    }
    if (prov == "" || provider == prov) {
        if (gen) {
            printf "// %s:%s\n", provider, name
            if (args) {
                printf "// Arguments: %s\n", args
            }
            printf "uprobe:%s:%s {\n", binary, addr
            if (args) {
                printf "    printf(\"%s:%s arg0=%%d\\n\", arg0);\n", provider, name
            } else {
                printf "    printf(\"%s:%s hit\\n\");\n", provider, name
            }
            printf "}\n\n"
        } else {
            printf "Provider: %s\n", provider
            printf "Name:     %s\n", name
            printf "Address:  %s\n", addr
            if (args) {
                printf "Args:     %s\n", args
            }
            printf "\n"
        }
    }
}
'

if [[ -n "$GEN_SCRIPT" ]]; then
    echo "END {"
    echo "    printf(\"\\nTracing complete.\\n\");"
    echo "}"
fi
