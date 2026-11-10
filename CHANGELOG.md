# Changelog

The port's version is the spec version it implements.

## 0.15.0 — 2026-11-10

First release: spec 0.15.0, suite 0.15.0, all six profiles (391 cases, no
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
- The spec took in part of that feedback before release: a rejected send
  fails the socket, concurrent pings share one attempt, the project XML is
  UTF-8 only and an unpaired surrogate escape is invalid. The port follows:
  UTF-16 project XML and lone surrogates are now `invalidData` (they were
  transcoded and replaced with U+FFFD), and the element and value caps are
  gone. The suite now covers the timeout-streak reset, so the port's own
  test for it is gone too.
- Live, read-only (2026-11-10, the watch example, now `examples/watch-state`,
  from a saved archive): every CLU ready in well under a second, the keys
  registered in several chunks, every entity with a known state, and
  pushes at a steady rate for 75 s, past the ~60 s lease, so the renewal
  works on real CLUs. No warning. No command was sent.
- User documentation on the Swift port's model: a README with a
  step-by-step quick start, where to find the PIN and IP address in Object
  Manager, and a table of examples; a GitHub Pages site (`site/`: a
  landing page and Getting Started, published from `main` by
  `.github/workflows/pages.yml`) that links to pkg.go.dev for the API
  reference.
- Examples, each one `main.go` on the public API: `fetch-interface`
  (saves the archive with mode 0600), `import-project`, `list-devices`
  (offline), `watch-state` (read-only; replaces `examples/watch`) and,
  changing the house, `switch-light`, `run-scene` and `set-thermostat`.
- The gate builds the site (`scripts/docs.sh build`), checks its links and
  README's (`scripts/check-site-links.py`), and compiles and vets every Go
  block of README.md and site/ against the public API
  (`scripts/check-doc-snippets.py`). A merge that touches only site/,
  examples/ or scripts/ is, like the documentation, not a release.
