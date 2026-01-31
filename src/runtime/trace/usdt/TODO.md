# USDT Implementation TODO

## Milestone B: Additional Architectures

- [x] linux/amd64 support
- [x] linux/arm64 support

## Milestone C: Probe Arguments

- [x] Add Probe1-Probe4 functions with generic type constraints
- [x] Define USDTArg interface for allowed argument types
- [x] Implement SSA operations (OpUSDTProbe1-4, LoweredUSDTProbe1-4)
- [x] Add intrinsic handlers for generic functions
- [x] Generate argdesc in SystemTap SDT format (e.g., "-4@%eax", "8@x0")
- [x] Support integer types: int8-64, uint8-64, uintptr
- [x] Support unsafe.Pointer for passing string/data pointers
- [x] Serialize ArgDesc through goobj to linker
- [x] Write ArgDesc to .note.stapsdt ELF section
- [x] Update net/http probes to pass status codes
- [x] Document argument format for bpftrace compatibility
- [x] Document string pointer usage in README.md
- [x] Verify with bpftrace on ARM64 (Lima VM)

## Milestone D: Semaphores

- [ ] Add `.stapsdt.base` section for proper relocation
- [ ] Implement semaphore support for probe enable/disable
- [ ] Add `usdt.Enabled(provider, name)` API to check if probe is active
- [ ] Optimize probe sites to skip work when no tracer attached

## Milestone E: Trampoline Friendly Probes

- [ ] Add support for trampoline friendly probes <https://github.com/linux-usdt/libstapsdt/blob/0d53f987b0787362fd9c16a93cdad2c273d809fc/src/asm/libstapsdt-x86_64.s#L8-L11>

## Tooling & Compatibility

- [ ] Fix bpftrace USDT discovery for static Go binaries
  - Investigate adding `.stapsdt.base` section
  - Consider dynamic symbol table entries
  - [x] Document workaround using uprobe syntax (see README.md)
  - **Challenges documented below**
- [x] Create `go tool usdt` helper (cmd/usdt) for:
  - [x] Listing probes in a binary (`go tool usdt list`)
  - [x] Generating bpftrace scripts from probe metadata (`go tool usdt bpftrace`)
  - [x] Validating probe placement (`go tool usdt validate`)
- [ ] Add perf probe integration tests
- [ ] Test with SystemTap

### Challenges: bpftrace USDT Discovery for Static Binaries

The `usdt:/path:provider:name` syntax in bpftrace doesn't work for Go binaries because:

1. **No Dynamic Symbol Table**: Go produces statically linked binaries with no `.dynsym` section.
   bpftrace's USDT discovery queries the dynamic linker and symbol tables, which don't exist
   in static binaries.

2. **Missing `.stapsdt.base` Section**: Some tracing tools expect a dedicated `.stapsdt.base`
   PROGBITS section as a relocation anchor. Go currently stores the base address inline in
   each probe descriptor within `.note.stapsdt`, but lacks the separate section.

3. **No Dynamic Linker**: Static binaries have no `ld.so` involvement at runtime, breaking
   traditional USDT probe attachment mechanisms that rely on dynamic loading.

**Potential Solutions to Investigate:**

1. **Add `.stapsdt.base` Section**: Create a PROGBITS section containing a single pointer
   to the text segment start. Location: `src/cmd/link/internal/ld/elf.go`

   ```go
   // Pseudocode for adding .stapsdt.base
   sh := elfshname(".stapsdt.base")
   sh.Type = uint32(elf.SHT_PROGBITS)
   sh.Flags = uint64(elf.SHF_ALLOC)
   // Write single pointer: Segtext.Vaddr
   ```

2. **Add Minimal Dynamic Symbols**: Add probe-related symbols to a minimal `.dynsym` section.
   This is complex and may conflict with Go's static linking model.

3. **Upstream bpftrace Fix**: The issue may be better solved in bpftrace itself to support
   static binaries by reading `.note.stapsdt` without requiring dynamic symbols.

**Current Workaround**: Use `go tool usdt bpftrace` to generate scripts with `uprobe:` syntax,
which works reliably with static binaries.

**References:**

- [bpftrace discussion #2184](https://github.com/bpftrace/bpftrace/discussions/2184)
- [libstapsdt internals](https://libstapsdt.readthedocs.io/en/latest/how-it-works/internals.html)
- [USDT deep dive](https://www.polarsignals.com/blog/posts/2025/12/10/usdt-deep-dive)

## Standard Library Probes

- [x] net/http server request start/end
- [x] net/http client request start/end
- [x] net/http: pass status code as argument (Probe1)
- [x] net/http: pass method, path/URL as arguments (Probe4 with unsafe.Pointer)
- [x] database/sql: query start/end with query text (prepare, exec, query, stmt_exec, stmt_query, tx_begin)
- [x] crypto/tls: handshake start/end/error with isClient flag
- [x] net: connection accept/close (conn_accept, conn_close)
- [ ] Probes captures w3c trace headers

## Documentation

- [x] Package documentation with examples
- [x] Document known limitations
- [x] Write README.md with bpftrace recipes
- [x] Add example programs in `_example/` directory
- [x] Add bpftrace script examples and helper tools
- [ ] Document probe naming conventions

## Testing

- [x] Unit tests for ELF note generation (amd64)
- [x] Unit tests for ELF note generation (arm64)
- [x] Test multiple probes in single binary
- [x] Test non-literal string error
- [x] Test unsupported platform behavior
- [x] Test Probe1-4 with all supported argument types
- [x] Test unsafe.Pointer arguments
- [x] Test argdesc format verification (AMD64 and ARM64)
- [x] Verify argument capture with bpftrace (ARM64)
- [ ] Integration tests with actual tracing (requires Linux CI)
- [ ] Benchmark probe overhead (should be ~0 when not traced)
- [ ] Test probe behavior across goroutines
- [ ] Test inlined probe sites

## Known Issues

1. **bpftrace USDT discovery**: The `usdt:/path:provider:name` syntax doesn't
   work for statically linked Go binaries. Workaround: use `uprobe:/path:0xADDR`
   with addresses from `readelf -n`.

2. **Missing .stapsdt.base section**: Some tools expect this section for
   address relocation. Currently we use text segment start as base address.

3. **Thread ID tracking**: Go's M:N scheduler means probe events on different
   OS threads may belong to the same goroutine. Consider adding goroutine ID
   to probe arguments.

4. **String arguments**: Pass string data using `unsafe.Pointer(unsafe.StringData(s))`.
   Go strings are not null-terminated, so the tracer must also receive the length
   to read correctly. Pass length as a separate argument:
   `usdt.Probe2("app", "req", unsafe.Pointer(unsafe.StringData(s)), int64(len(s)))`
