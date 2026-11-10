# grenton-core-go

Go port of the Grenton Core spec. The spec and conformance suite live in
`grenton-core-spec`, vendored as the `vendor/grenton-core-spec` submodule.
Spec first: a behaviour change starts in the spec repository, then every
port follows.

## Commands

- `scripts/check.sh`: the full gate (also the pre-push hook);
  `scripts/check.sh quick` the fast part.
- `go test ./...`: unit tests, the conformance suite (`conformance/`), the
  API-docs and dependency-direction checks (`internal/archtest`).
- Hooks: `git config core.hooksPath scripts/hooks`, once per clone.
- `scripts/docs.sh`: build the site into `_site/` (git-ignored) and serve
  it on http://localhost:8000/ to preview; `scripts/docs.sh build` only
  builds. `scripts/check-site-links.py` (after a build) checks that every
  site link is relative and resolves, and README's repository links;
  `scripts/check-doc-snippets.py` compiles and vets every `go` fenced block of
  README.md and `language-go` block of `site/` in a scratch module.
  Both run in `scripts/check.sh`.
- The installation-data gate compares against your interface archive:
  `$GRENTON_INTERFACE`, `git config grenton.interface <path>`, or the Swift
  port's `dist/interface.zip`. Without one it fails; a clone with no
  installation declares `git config grenton.interface none`.

## Layout

- `model`, `provisioning`, `clu`, `devices`: the four Core modules of spec
  §2. `internal/wire`, `internal/normalize`, `internal/strictjson` belong to
  Runtime, Devices and Provisioning; `internal/archtest` enforces the
  direction. The root package `grenton` is the umbrella and holds no rule.
- `internal/clutest`: virtual clock, scripted transport, activity counter.
- `conformance/`: the runner; `port.json` declares suite version, profiles
  and skips.
- `examples/<name>/main.go`: one program per use, public API only;
  `switch-light`, `run-scene` and `set-thermostat` change the house.
- `site/`: the GitHub Pages site (landing page, Getting Started), published
  from `main` by `.github/workflows/pages.yml`; the API reference is
  pkg.go.dev. `docs/images/`: the README's screenshot, copied into the site.
- `docs/grenton-core-go-port.md`: the port plan. `docs/spec-feedback.md`:
  spec gaps found by this port.
- `dist/`: git-ignored; saved interfaces (they hold the CLU key) and logs.

## Rules

- Expected values never come from the port; never edit the vendored suite
  or add a skip to turn the run green.
- No identifiers of a real installation in files or commit messages.
- `main` takes changes only through a pull request; every merge into main
  is a release, tagged `v` + the suite version (a port-only fix after a
  tag needs a spec bump).
- Writes that change the house need the user's go-ahead.
- The session never blocks while holding its mutex, and never closes a
  socket under it.
