# USDT Examples

This directory contains example programs demonstrating USDT probe usage.

## Examples

### basic/

Simple probe usage without arguments.

```bash
cd basic
go build -o basic .
readelf -n ./basic | grep -E "(Provider|Name|Location)"
```

### http_server/

HTTP server with probe arguments (status codes, latency).

```bash
cd http_server
GOOS=linux GOARCH=amd64 go build -o http_server .
readelf -n ./http_server | grep -E "(Provider|Name|Location|Arguments)"
```

## bpftrace Scripts

### usdt-list.sh

Helper script to list probes and generate bpftrace commands.

```bash
# List all probes
./bpftrace/usdt-list.sh ./http_server

# Filter by provider
./bpftrace/usdt-list.sh ./http_server myserver

# Generate bpftrace script
./bpftrace/usdt-list.sh ./http_server myserver -gen > trace.bt
sudo bpftrace trace.bt -c ./http_server
```

### trace_basic.bt

Template for basic probe tracing.

### http_latency.bt

Template for HTTP request latency tracing using net/http probes.

## Quick Start

1. Build your Go binary for Linux:
   ```bash
   GOOS=linux GOARCH=amd64 go build -o myapp .
   ```

2. Find probe addresses:
   ```bash
   readelf -n ./myapp | grep -E "(Provider|Name|Location|Arguments)"
   ```

3. Trace with bpftrace:
   ```bash
   sudo bpftrace -e 'uprobe:./myapp:0x<ADDR> { printf("probe hit\n"); }'
   ```

## Reading Probe Arguments

For probes with arguments, the `Arguments` field shows the register/format:

| Format | Meaning |
|--------|---------|
| `-4@%eax` | Signed 32-bit in eax (AMD64) |
| `8@%rax` | Unsigned 64-bit in rax (AMD64) |
| `-4@w0` | Signed 32-bit in w0 (ARM64) |
| `8@x0` | Unsigned 64-bit in x0 (ARM64) |

Example reading status code:
```bash
# Arguments: -4@%eax
bpftrace -e 'uprobe:./app:0x<ADDR> { printf("status=%d\n", arg0); }'
```

## Platform Requirements

- **Build target**: linux/amd64 or linux/arm64
- **Tracing**: Requires Linux with bpftrace and root privileges
- **Cross-compilation**: Works from macOS/Windows, trace on Linux
