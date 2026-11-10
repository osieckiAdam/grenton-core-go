# Changelog

The port's version is the spec version it implements.

## 0.15.0 — unreleased

First release: spec 0.15.0, suite 0.15.0, all six profiles (383 cases, no
skips).

- `model`, `provisioning`, `clu`, `devices` and the `grenton` umbrella,
  standard library only, Go 1.26.
- The session reports its runnable work to `clu.Activity`, which gives the
  conformance runner an exact idle barrier instead of a real-time quiet
  window.
- Own tests for what the suite does not cover (spec §9.5 item 8): both UDP
  modes on loopback, no goroutine after `Stop` (also over real UDP), an
  abandoned session closes its socket, the update signals and command
  routing of `System` over two CLUs, no PIN in log lines.
- Two independent reviews (provisioning and model; the session) found 10
  faults the suite does not catch; each is fixed with a test, and the spec
  gaps behind them are in `docs/spec-feedback.md`.
- Live, read-only (2026-11-10, `examples/watch` from a saved archive): 5
  CLUs ready in 137 ms, 290 keys in several chunks, every entity with a
  known state, and pushes at a steady rate for 75 s, past the ~60 s lease,
  so the renewal works on real CLUs. No warning. No command was sent.
