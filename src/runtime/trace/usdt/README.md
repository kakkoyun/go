# USDT Probes for Go

This package provides USDT (Userland Statically Defined Tracing) probes for Go programs.

## Quick Start

```go
import "runtime/trace/usdt"

func handleRequest(w http.ResponseWriter, r *http.Request) {
    usdt.Probe("myapp", "request_start")
    defer usdt.Probe1("myapp", "request_end", int32(w.StatusCode))
    // ... handle request
}
```

## The `go tool usdt` Command

The `go tool usdt` command helps you work with USDT probes in Go binaries.

### List Probes

```bash
$ go tool usdt list ./myserver
PROVIDER             NAME                           ADDRESS            ARGUMENTS
------------------------------------------------------------------------------------------
net_http             server_request_start           0x63296c           8@%rsi -8@%r8 8@%rdx -8@%r9
net_http             server_request_end             0x631c5c           -4@%ecx
sql                  query                          0x4a1234           8@%rsi -8@%rdx

# JSON output for scripting
$ go tool usdt list -json ./myserver
```

### Generate bpftrace Scripts

```bash
# Generate bpftrace script (recommended approach)
$ go tool usdt bpftrace ./myserver > trace.bt
$ sudo bpftrace trace.bt

# Filter by provider or probe name
$ go tool usdt bpftrace -provider net_http ./myserver
$ go tool usdt bpftrace -probe server_request_end ./myserver
```

### Validate Probe Placement

```bash
$ go tool usdt validate -v ./myserver
Found 6 USDT probe(s)
Text segment: 0x400000 - 0x6a0000
Validating probe 1: net_http:server_request_start at 0x63296c
  NOP instruction verified at probe site
Validation passed.
```

## Tracing with bpftrace

Due to Go's static linking, the standard `usdt:` syntax may not work. Use `uprobe:` with addresses from the ELF notes.

### Step 1: Find Probe Addresses

```bash
# List all USDT probes in a binary
readelf -n ./myapp | grep -E "(Provider|Name|Location|Arguments)"

# Example output:
#     Provider: myapp
#     Name: request_start
#     Location: 0x000000000047ae00, Base: 0x0000000000400040, Semaphore: 0x0
#     Arguments:
#     Provider: myapp
#     Name: request_end
#     Location: 0x000000000047ae20, Base: 0x0000000000400040, Semaphore: 0x0
#     Arguments: -4@%eax
```

### Step 2: Trace Using uprobe

```bash
# Simple probe hit counter
bpftrace -e 'uprobe:./myapp:0x47ae00 { @hits++; }'

# Print on each hit
bpftrace -e 'uprobe:./myapp:0x47ae00 { printf("request started\n"); }'
```

### Step 3: Reading Probe Arguments

Arguments are passed in registers. Use `arg0`, `arg1`, etc. to read them:

```bash
# Read status code from request_end probe
bpftrace -e 'uprobe:./myapp:0x47ae20 { printf("status=%d\n", arg0); }'

# The argdesc "-4@%eax" means:
#   -4 = signed 4-byte value (int32)
#   %eax = x86-64 register containing the value
# bpftrace's arg0 automatically reads from the correct register
```

### Step 4: Passing String Pointers

To pass string data to tracers, use `unsafe.Pointer` with `unsafe.StringData`:

```go
import (
    "runtime/trace/usdt"
    "unsafe"
)

func handleRequest(method string, status int32) {
    // Pass string data pointer and status code
    usdt.Probe2("myapp", "request",
        unsafe.Pointer(unsafe.StringData(method)),
        status)
}
```

**Important**: Go strings are not null-terminated. When reading with bpftrace's `str()`, you must also pass the length:

```go
// Pass pointer and length
usdt.Probe2("myapp", "request",
    unsafe.Pointer(unsafe.StringData(method)),
    int64(len(method)))
```

```bash
# Read fixed-length string (requires passing length as arg1)
bpftrace -e 'uprobe:./myapp:0x<addr> {
    printf("method=%.*s\n", arg1, str(arg0, arg1));
}'
```

Alternatively, for short fixed strings, read only the expected bytes:

```bash
# Read up to 8 bytes (suitable for HTTP methods like GET, POST, etc.)
bpftrace -e 'uprobe:./myapp:0x<addr> { printf("method=%s\n", str(arg0, 8)); }'
```

## Example bpftrace Scripts

### HTTP Request Latency

```bash
#!/usr/bin/env bpftrace
// Measure HTTP request latency
// Usage: sudo bpftrace http_latency.bt -c ./myserver

BEGIN {
    printf("Tracing HTTP requests... Ctrl-C to stop\n\n");
}

// Get addresses with: readelf -n ./myserver | grep -B2 "request_start\|request_end"
uprobe:./myserver:0x<START_ADDR> {
    @start[tid] = nsecs;
}

uprobe:./myserver:0x<END_ADDR> {
    $latency = (nsecs - @start[tid]) / 1000;
    @latency_us = hist($latency);
    printf("request completed: latency=%dus status=%d\n", $latency, arg0);
    delete(@start[tid]);
}

END {
    clear(@start);
}
```

### HTTP Status Code Distribution

```bash
#!/usr/bin/env bpftrace
// Count HTTP responses by status code
// Usage: sudo bpftrace http_status.bt -c ./myserver

uprobe:./myserver:0x<END_ADDR> {
    @status[arg0] = count();
}

END {
    printf("\nHTTP Status Code Distribution:\n");
    print(@status);
}
```

## Standard Library Probes

The Go standard library includes USDT probes in several packages.

### net/http Probes

| Provider | Name | Arguments | Description |
|----------|------|-----------|-------------|
| net_http | server_request_start | method_ptr, method_len, path_ptr, path_len | HTTP handler entry |
| net_http | server_request_end | status:int32 | HTTP handler exit |
| net_http | client_request_start | method_ptr, method_len, url_ptr, url_len | Client request sent |
| net_http | client_response_received | status:int32 | Client response received |

### database/sql Probes

| Provider | Name | Arguments | Description |
|----------|------|-----------|-------------|
| sql | prepare | query_ptr, query_len | Prepared statement creation |
| sql | exec | query_ptr, query_len | Exec operation |
| sql | query | query_ptr, query_len | Query operation |
| sql | stmt_exec | (none) | Prepared statement exec |
| sql | stmt_query | (none) | Prepared statement query |
| sql | tx_begin | (none) | Transaction start |

### crypto/tls Probes

| Provider | Name | Arguments | Description |
|----------|------|-----------|-------------|
| tls | handshake_start | isClient:int8 | TLS handshake started (1=client, 0=server) |
| tls | handshake_end | isClient:int8 | TLS handshake completed |
| tls | handshake_error | isClient:int8 | TLS handshake failed |

### net Probes

| Provider | Name | Arguments | Description |
|----------|------|-----------|-------------|
| net | conn_accept | (none) | TCP connection accepted |
| net | conn_close | (none) | Connection closed |

### Example: Tracing net/http

```bash
# Use go tool usdt to generate a script
go tool usdt bpftrace -provider net_http ./myserver > http_trace.bt
sudo bpftrace http_trace.bt

# Or manually trace server requests with status codes
bpftrace -e '
uprobe:./myserver:0x<server_end_addr> {
    printf("HTTP %d\n", arg0);
    @status[arg0] = count();
}
' -c ./myserver
```

## Argument Format Reference

The `Arguments` field in `readelf -n` output uses SystemTap SDT format:

| Format | Meaning | Example |
|--------|---------|---------|
| `N@%reg` | N-byte unsigned value in register | `8@%rax` |
| `-N@%reg` | N-byte signed value in register | `-4@%eax` |
| `N@-offset(%rbp)` | N-byte value at stack offset | `8@-16(%rbp)` |

### Register Names by Architecture

**AMD64 (x86-64):**

- 64-bit: `%rax`, `%rbx`, `%rcx`, `%rdx`, `%rsi`, `%rdi`, `%r8`-`%r15`
- 32-bit: `%eax`, `%ebx`, `%ecx`, `%edx`, `%esi`, `%edi`
- 16-bit: `%ax`, `%bx`, `%cx`, `%dx`
- 8-bit: `%al`, `%bl`, `%cl`, `%dl`

**ARM64:**

- 64-bit: `x0`-`x30`
- 32-bit: `w0`-`w30`

## Helper Script

Create a script to generate bpftrace commands from a binary:

```bash
#!/bin/bash
# usdt-trace.sh - Generate bpftrace commands for Go USDT probes
# Usage: ./usdt-trace.sh ./myapp [provider]

BINARY="$1"
PROVIDER="${2:-}"

readelf -n "$BINARY" 2>/dev/null | awk -v binary="$BINARY" -v prov="$PROVIDER" '
/Provider:/ { provider = $2 }
/Name:/ { name = $2 }
/Location:/ {
    addr = $2
    gsub(/,/, "", addr)
    if (prov == "" || provider == prov) {
        printf "# %s:%s\n", provider, name
        printf "uprobe:%s:%s { printf(\"%s:%s hit\\n\"); }\n\n", binary, addr, provider, name
    }
}
'
```

## Troubleshooting

### Probes not visible to bpftrace

Go binaries are statically linked, so `bpftrace -l 'usdt:./myapp:*'` won't find probes. Use `readelf -n` instead.

### Wrong argument values

Ensure you're using the correct register size. For `int32` (`-4@%eax`), cast the value:

```bash
printf("status=%d\n", (int32)arg0);
```

### Probe not firing

1. Verify the address is correct: `readelf -n ./myapp | grep Location`
2. Check the binary matches: rebuild and re-extract addresses
3. Ensure bpftrace has permissions: `sudo bpftrace ...`

## Platform Support

| Platform | Status |
|----------|--------|
| linux/amd64 | Supported |
| linux/arm64 | Supported |
| darwin/* | No-op (compiles but no tracing) |
| windows/* | No-op (compiles but no tracing) |
