#!/bin/sh
# The gate every change passes before it is pushed.
#
#   scripts/check.sh          everything (the pre-push hook runs this)
#   scripts/check.sh quick    the fast checks only (the pre-commit hook)
#
# Spec compliance is the conformance suite inside `go test`; the spec-sync
# check makes sure it is the current spec. The public API must be
# documented (internal/archtest), the examples must build, the site's
# links must resolve and every Go block of README.md and site/ must
# compile against the public API.
set -e
cd "$(dirname "$0")/.."
step() { printf '\n== %s\n' "$1"; }

step "Git hooks";         scripts/check-hooks.sh
step "Installation data"; scripts/check-no-installation-data.py --all
step "Spec sync";         scripts/check-spec-sync.sh
step "Formatting"
unformatted=$(gofmt -l $(git ls-files --cached --others --exclude-standard '*.go'))
[ -z "$unformatted" ] || { echo "not gofmt-formatted:" >&2; echo "$unformatted" >&2; exit 1; }
echo "gofmt: clean"
step "Vet";               go vet ./...
[ "$1" = "quick" ] && { echo; echo "Quick checks passed."; exit 0; }

step "Build (also the examples, against the public API)"; go build ./...
step "Tests, the conformance suite and the API checks, under the race detector"
log=$(mktemp)
if go test -race -count=1 ./... > "$log" 2>&1; then grep -v "no test files" "$log"; rm -f "$log"
else cat "$log"; rm -f "$log"; echo "tests failed" >&2; exit 1; fi
step "Session scenarios, repeated"; go test -race -count=20 -run 'TestConformance/session' ./conformance/
step "Documentation site"; scripts/docs.sh build
step "Site links";        scripts/check-site-links.py
step "Doc snippets";      scripts/check-doc-snippets.py
step "Cross-compile";     GOOS=windows GOARCH=amd64 go build ./... && GOOS=linux GOARCH=arm64 go build ./...
echo
echo "All checks passed."
