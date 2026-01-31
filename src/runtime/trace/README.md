# Event Filtering and Distributed Tracing POC

This branch adds event filtering and distributed tracing support to the runtime/trace package.

## Features

- **Event Filtering**: Selective tracing by category (Core, HTTP, SQL, TLS, Net, Custom)
- **W3C Trace Context**: TraceContext struct for distributed tracing correlation
- **HTTP Tracing**: HTTPSpan for client/server requests with version and error tracking
- **SQL Tracing**: SQLSpan for database queries with error codes
- **TLS Tracing**: TLSSpan for handshakes with version and cipher suite info
- **Network Tracing**: DNSSpan and ConnectSpan for DNS lookups and TCP connections
- **Sanitizers**: URL and SQL sanitizers for sensitive data protection

## Usage

### Environment Variables

```bash
GODEBUG=tracehttp=1 ./myapp              # Enable HTTP events
GODEBUG=tracehttp=1,tracesql=1 ./myapp   # Enable HTTP and SQL events
```

### API

```go
trace.SetEventFilter(trace.FilterCore | trace.FilterHTTP)
trace.Start(w)
```

## Files

- `config.go` - Event filtering API and GODEBUG integration
- `http.go` - HTTP tracing (TraceContext, HTTPSpan)
- `sql.go` - SQL query tracing
- `tls.go` - TLS handshake tracing
- `net.go` - Network-level tracing (DNS, TCP)

## Status

POC implementation. See test files for usage examples.
