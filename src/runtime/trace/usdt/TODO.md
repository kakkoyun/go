# USDT Implementation TODO

Open items only. Completed work is recorded in the e2e harness FINDINGS.md.
See ~/Workspace/Sandbox/go-usdt-e2e/FINDINGS.md for the evidence record.

## Semaphores

- [ ] Implement semaphore support for probe enable/disable
- [ ] Add `usdt.Enabled(provider, name)` API to check if probe is active
- [ ] Optimize probe sites to skip work when no tracer attached

## Trampoline-Friendly Probes

- [ ] Add support for trampoline-friendly probes
      <https://github.com/linux-usdt/libstapsdt/blob/0d53f987b0787362fd9c16a93cdad2c273d809fc/src/asm/libstapsdt-x86_64.s#L8-L11>

## External Linking

- [ ] Emit .note.stapsdt under -linkmode=external (currently drops all probes;
      external linking hands section emission to the host linker and needs its
      own design)

## Testing

- [ ] Integration tests with actual tracing (requires Linux CI / Lima VM)
- [ ] Benchmark probe overhead (should be ~0 when not traced)
- [ ] Test with SystemTap and perf probe

## Standard Library Probes

- [ ] Probes that capture W3C trace headers

## Design Notes

- **Thread ID tracking**: Go's M:N scheduler means probe events on different
  OS threads may belong to the same goroutine. Consider adding goroutine ID
  to probe arguments.
- **String arguments**: Pass string data using
  `unsafe.Pointer(unsafe.StringData(s))` with length as a separate argument:
  `usdt.Probe2("app", "req", unsafe.Pointer(unsafe.StringData(s)), int64(len(s)))`.
