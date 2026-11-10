# grenton-core-go — port plan

Maintainers' notes. This plan turns spec 0.15.0 (`vendor/grenton-core-spec`)
into Go decisions: it says *what to use*, never *what to do*. The behaviour
is the spec's; code comments cite this file as "Go plan §n".

## 1. Toolchain, platforms, dependencies

- `go 1.26` in `go.mod` (the oldest supported release; vCLU builds with it).
- Linux (amd64, arm64), macOS, Windows. Pure Go, no cgo.
- Standard library only. `crypto/aes` + `crypto/cipher` (CBC), `net`,
  `net/http`, `archive/zip`, `compress/flate`, `encoding/xml`,
  `encoding/json` (for the *mobile* interface only, see §5), `log/slog`.

## 2. Packages and the module boundaries of spec §2

| Package | Spec module | Public for |
|---|---|---|
| `model` | Model | values, `ValueRef`/`ActionRef`, the interface DTOs and their tolerant decoder |
| `provisioning` | Provisioning | the Object Manager client, saved archives, `.omp` import |
| `clu` | Runtime | `Session`, `StateStore`, the cipher, the port interfaces (§4) |
| `devices` | Devices | descriptors, bound devices/entities, `System` (several CLUs) |
| `internal/wire` | Runtime | frames, payload builders and parsers, chunk split |
| `internal/strictjson` | Provisioning | the strict, depth-bounded JSON of §4.5.1 item 5 |
| `internal/normalize` | Devices | coercions, integer reads, `mapRange`, number format, schedule rules |
| `grenton` (module root) | helper (umbrella) | `Open`: fetch or import, then start a `System`; holds no rule |
| `simulation` | helper | devices over values a test sets, every command recorded (spec §2) |

`internal/` sits at the module root so the conformance runner (a test
package of the module) can reach the plumbing without it becoming public
API. `runtime` is not used as a package name: it would shadow the standard
library's.

The dependency direction (`provisioning → model ← clu ← devices`, helpers on
top) is enforced by `internal/archtest`: a test that reads every package's
imports with `go/build` and fails on an edge the table does not allow
(`model` imports nothing of the module; `internal/wire` only `model`, …).

## 3. Values and references

- `model.Value` is a small struct `{Kind; i int64; f float64; s string; b bool}`
  with constructors (`Int`, `Double`, `String`, `Bool`, `Null`) and accessors.
  A struct, not an interface: comparable with `==`, no allocation, and
  `Kind` makes the five cases explicit. "Unknown" (never reported) is a
  separate `(Value, bool)` at the store, never a sixth kind.
- `model.ParseWireValue(s)` is §3.1: its own ASCII grammar checks before
  `strconv.ParseInt`/`ParseFloat`, which accept `inf`, `0x`, `_` and `+`.
- `ValueRef` is a struct with a `CallType` (`Variable`/`Attribute`);
  `ActionRef` likewise (`Variable`, `Attribute`, `Method`, `Script`), with
  `Index` absent for scripts. Both are values; an intent that overrides
  `Value` works on a copy (spec §6.3.10).
- Widget DTOs: one struct per widget type behind a `Widget` interface with
  an unexported marker method, decoded by `type`. LED components the same.

## 4. Port contract (spec §7)

| Spec | Go | Production | Tests |
|---|---|---|---|
| `UdpSocketFactory`/`UdpSocket` | `clu.Transport.Open(host, port, Receiver) (Socket, error)`; `Socket.Send([]byte) error`, `Close()`; the receiver is a callback `Datagram([]byte)`, `Closed(error)` | `clu.UDP{AnySource bool}`: peer-filtered (connected `net.UDPConn`) or any-source (one *unconnected* `ListenUDP` on an ephemeral port, `WriteTo` the CLU) | scripted transport in the runner; loopback tests of both modes |
| `AesCbcCipher` | `clu.Cipher` (`Encrypt`, `Decrypt` → `([]byte, bool)`) | `crypto/cipher` CBC + own PKCS#7 | golden vectors |
| `HttpClient` + `ZipReader` | `provisioning.HTTPDoer` (`Do(*http.Request)`) and the archive reader | `http.Client` with `CheckRedirect` → `http.ErrUseLastResponse`, timeouts, no cookie jar | the runner's loopback HTTP server |
| `Clock`/`TimerScheduler` | `clu.Clock`: `Now() time.Time`, `AfterFunc(d, f) Timer` | `time` | virtual clock: time moves only on `Advance`, due callbacks run synchronously in deadline order |
| `RandomSource` | `clu.Random`: `Read([]byte)` | `crypto/rand` | fixed sequence |
| `Logger` | `*slog.Logger` | discard handler by default | capturing handler (PIN tests) |
| Asynchronous execution | `clu.Activity` (below) | no-op | counting tracker |

The receive side is a callback because it gives **one reader per socket**
by construction: the transport's read goroutine (or the runner, directly)
calls `Datagram`; the session never reads a socket. The callback only
decrypts, takes the session lock briefly, completes a pending request or
publishes a report.

## 5. Concurrency (spec §5.6) and the idle barrier

- A `Session` holds its state under one `sync.Mutex`, never held across a
  blocking call.
- **Waiters.** Every place a session goroutine blocks (a reply, a timeout,
  a loop's sleep, its turn at the gate) is a one-shot `waiter`: the first
  `fire` stores the result and wakes it; later fires are no-ops. A reply,
  its timeout and `stop()` race to fire the same waiter, so exactly one wins
  and a late reply cannot end the next request with the same `msg_id`
  (every ping reuses `keepAliveId`).
- **Gate.** `start`, `register`, `fetch`, `stop`, the recovery and the loop
  renewal take a FIFO gate (a queue of waiters), so whole operations never
  interleave. `stop()` first fails pending requests (without the gate), then
  takes the gate for the destroys and the close, so it never waits for a
  registration's remaining timeouts.
- **Sends** go through one sender goroutine per socket, in order, so a send
  that never completes (spec §5.4.1 item 3) blocks neither the session nor
  `stop()`; the request's deadline starts when it is queued. Closing the
  socket ends the stuck send.
- **Loops** are goroutines sleeping on a waiter fired by `Clock.AfterFunc`
  or by `stop()`. `stop()` ends them and waits for them, so after it returns
  no session goroutine remains.
- **Abandoned sessions.** The public `*Session` is a handle around an
  internal core; goroutines reference only the core. `runtime.AddCleanup`
  on the handle closes the socket and ends the loops when the application
  drops the session without `Stop` (spec §5.4.5; no `clientDestroy`). `Stop`
  is still documented as the normal end.
- **Idle barrier (conformance.md §4b).** `clu.Activity` is an optional
  `Add(delta int)` hook. The session calls it around every goroutine it
  starts and every waiter (−1 before blocking, +1 by the firer *before* it
  wakes the goroutine). The runner's tracker counts; it also counts its own
  call goroutines and delivers datagrams synchronously (the callback runs
  in the runner's goroutine) and advances the virtual clock synchronously.
  So "count = 0" means no session work is possible without time or a new
  datagram: an exact barrier, with no real-time quiet window for
  `expectQuiet`/`expectNoReport`. The race detector runs the session suite.

## 6. Errors (spec §8)

Each category is a type with a `Kind`; `errors.Is` matches the sentinel of
the kind (`provisioning.ErrInvalidData`, `clu.ErrTimeout`, …).

| Spec | Go | Suite code |
|---|---|---|
| `ProvisioningError.*` | `*provisioning.Error{Kind, Status, Detail}` | `provisioning.<kind>` |
| `CryptoError.invalidKeyMaterial`, `encryptFailed` | `clu.ErrInvalidKeyMaterial`, `clu.ErrEncryptFailed` | `crypto.*` |
| `PayloadError.unresolvedPlaceholder` | `model.ErrUnresolvedPlaceholder` | `payload.unresolvedPlaceholder` |
| `TimeoutError` | `clu.ErrTimeout` | `session.timeout` |
| `SessionError.notConnected`, `connectionClosed` | `clu.ErrNotConnected`, `clu.ErrConnectionClosed` | `session.*` |

`provisioning.Error` never wraps a `*url.Error` (its text holds the URL,
and with it the PIN): a transport failure is rebuilt as
`connection: <op>: <the inner error's text with every PIN form removed>`.

## 7. Name map (spec §2 reference names → Go)

| Spec | Go |
|---|---|
| `GrentonValue` | `model.Value` |
| `castString` | `model.ParseWireValue` |
| `MobileInterface` | `model.Interface` |
| `CluSession` | `clu.Session` |
| `StateStore` (`registerKey`, `updateKeys`, `get`) | `clu.Store` (`Register`, `Update`, `Get`) |
| `Slot` (`failed`/`value`) | `clu.Slot` (`Failed bool`, `Value`) |
| `UdpSocketFactory` | `clu.Transport` |
| `DeviceDescriptor` | `devices.Descriptor` |
| binding | `devices.Bind` |
| `DeviceStateChanged` | `devices.Entity.Updates()` (a channel of capacity 1) |
| system facade | `devices.System` |

## 8. Strict inputs

- **Mobile `data.json`** (OM and archives): `encoding/json` into
  `json.RawMessage` per widget, then a hand-written decoder per widget type
  that checks required fields, field kinds and enums, so one bad widget is
  skipped and its neighbours survive (spec §3.6).
- **`.omp` nodes**: `internal/strictjson`, an iterative parser with an
  explicit stack (limit 64 levels): duplicate keys, `NaN`, non-finite
  numbers, integers outside Int64 and invalid UTF-8 are errors; object key
  order is kept.
- **XML**: `encoding/xml` with `Strict`; a `xml.Directive` token that is a
  DOCTYPE is `invalidData`; a non-UTF-8 encoding has no `CharsetReader`, so
  it fails as a syntax error (`invalidData`), which is also what §4.5.1
  item 4 wants of a DTD in any encoding.
- **ZIP**: `archive/zip` lists the directory; the reader the library uses
  counts inflated bytes itself through an `io.LimitReader` of limit + 1, and
  `archive/zip`'s checksum reader adds the size and CRC check against the
  headers. Entry names are compared byte for byte; `zip.ErrInsecurePath` is
  not an error for us (names are never used as paths).

## 9. Conformance runner

`conformance/` (package `conformance_test`): manifest checks (fail closed
on a missing submodule, a hash mismatch, an unlisted file, an unknown
operation, a bad case shape, a profile or skip the manifest does not know,
a stale skip, a suite version mismatch), `port.json`, one subtest per case.
Each fail-closed rule has its own test over a temporary copy of the suite.
`provisioning.fetch` uses `net/http/httptest`-free raw TCP server code, so
the runner controls the exact response bytes and counts requests.

## 10. Docs and examples

`go doc` comments on every exported identifier, checked by a test that
walks the AST of the public packages. `Example*` functions are compiled and
(where they need no network) run by `go test`. `examples/` holds programs
built by `go vet ./...` against the public API only, mirroring the Swift
port's: `fetch-interface`, `import-project`, `list-devices`, `watch-state`,
`switch-light`, `run-scene`, `set-thermostat`. User documentation follows
the Swift port too: README.md with the quick start, and `site/` (landing
page, Getting Started) on GitHub Pages; the reference is pkg.go.dev, not
hosted. `scripts/check-site-links.py` and `scripts/check-doc-snippets.py`
keep both honest in the gate.

## 11. Go traps (porting-prompt §7)

| Trap | Avoided by |
|---|---|
| lenient number parsing/formatting | own grammar before `strconv`; format reproduces `str(round(v, p))` with `strconv.FormatFloat(…, 'f', -1, 64)` after an exact half-to-even round on the binary value (`math/big`) |
| JSON duplicate keys, big ints as floats, NaN, abort on first bad element | strict parser for nodes; per-widget raw decoding for interfaces |
| XML entities / directives | DOCTYPE directive rejected; no `Entity` map |
| ZIP trusting sizes | own counting reader + `archive/zip` checksum reader |
| redirects, URL in errors | `CheckRedirect`; errors rebuilt without the URL |
| unconnected UDP | `net.ListenUDP` + `WriteTo` for any-source |
| AES-CBC padding | own PKCS#7, strict check, `Decrypt` returns `false` |
| stray real sleeps | the session imports no `time.Sleep`/`time.After`; the clock is injected |
| goroutine leaks, timer vs stop races | waiters + `stop()` joining its goroutines; leak test; `-race` in the gate |
| `int` width | `int64` for values and subscription numbers' arithmetic; ports as `int` checked to 1–65535 |
