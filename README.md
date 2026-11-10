# grenton-core-go

The Go port of [Grenton Core](https://github.com/osieckiAdam/grenton-core-spec):
a library for the Grenton smart-home "mobile interface" protocol. It gets
the interface from Object Manager (or a saved archive, or an `.omp` project
file), keeps an encrypted UDP session with every CLU, and maps the widgets
to canonical devices with normalized state and intents.

Standard library only. Go 1.26 or newer; Linux, macOS and Windows.

```sh
go get github.com/osieckiAdam/grenton-core-go
```

## Quick start

```go
sys, err := grenton.FromObjectManager(ctx, "192.0.2.10", "149 853", devices.SystemConfig{})
if err != nil {
	log.Fatal(err)
}
sys.Start()
defer sys.Stop()

for _, d := range sys.Devices() {
	fmt.Println(d.Name, d.Kind)
}
lamp := sys.Entity("w-onoff_0").(*devices.Switch)
on, known := lamp.IsOn()
updates, cancel := lamp.Updates() // a flag: read the getters when it fires
defer cancel()
```

The PIN is the token Object Manager shows in its "Send to mobile devices"
window; Object Manager serves the interface only while that window is
open. Without Object Manager, import the project file:
`grenton.FromProject(data, "", cfg)`. The interface archive holds the
installation's key: keep a saved copy private.

`examples/watch` lists every device and prints state changes (read-only):

```sh
go run ./examples/watch -host 192.0.2.10 -pin "149 853"
go run ./examples/watch -project house.omp
```

## Packages

| Package | Spec module | |
|---|---|---|
| `grenton` | helper | get an interface and make its `System` |
| `model` | Model | values, references, interface DTOs |
| `provisioning` | Provisioning | Object Manager client, archives, `.omp` import |
| `clu` | Runtime | `Session` (one CLU), `Store`, cipher, UDP transport |
| `devices` | Devices | descriptors, bound entities, `System` (all CLUs) |

`Session.Stop` and `System.Stop` end every socket, timer and goroutine; a
session dropped without `Stop` is closed by the garbage collector, but
call `Stop`.

## Conformance

Suite 0.15.0, all six profiles (`stateless`, `mapping`, `project`,
`session`, `provisioning`, `entities`), 391 cases, no skips
(`conformance/port.json`). The session scenarios run with an exact idle
barrier, 20 times under the race detector in the gate.

## Local network

- **macOS**: local-network privacy silently denies UDP to the LAN for
  binaries started from an IDE or an agent's shell (`no route to host`,
  `network is down`, while HTTP to the same host works). Run from
  Terminal.app, or grant the IDE access in System Settings → Privacy &
  Security → Local Network.
- **Linux, Windows**: no prompt; the firewall must allow inbound UDP on
  the ephemeral port the session binds (Windows asks on first run).
- A CLU with several network interfaces may report from another address:
  use `clu.UDP{AnySource: true}` as the transport.

## Development

`scripts/check.sh` is the gate (also the pre-push hook); enable the hooks
once per clone with `git config core.hooksPath scripts/hooks`. See
`CLAUDE.md` and `docs/grenton-core-go-port.md`.

MIT licence.
