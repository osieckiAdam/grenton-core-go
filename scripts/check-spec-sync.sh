#!/bin/sh
# Gate: the port tests the spec it claims, and the vendored spec is current.
#
#   - vendor/grenton-core-spec is checked out and has no local edits;
#   - conformance/port.json declares the suite version the vendored
#     manifest carries;
#   - the vendored commit is a spec release (a tag). Before a spec version
#     is merged and tagged, a local run only warns; CI fails;
#   - when a sibling ../grenton-core-spec checkout exists, the submodule
#     points at the same commit (--warn: only report it).
set -e
cd "$(dirname "$0")/.."
WARN_ONLY=0; [ "$1" = "--warn" ] && WARN_ONLY=1
unset GIT_DIR GIT_INDEX_FILE GIT_WORK_TREE GIT_PREFIX
SPEC=vendor/grenton-core-spec
fail() { echo "spec sync: $1" >&2; exit 1; }
warn() { if [ "$WARN_ONLY" = 1 ] || { [ -z "$CI" ] && [ "$2" = soft ]; }; then echo "spec sync (warning): $1" >&2; else fail "$1"; fi; }

[ -f "$SPEC/conformance/manifest.json" ] || fail "submodule missing — run: git submodule update --init"
[ -z "$(git -C "$SPEC" status --porcelain)" ] || fail "$SPEC has local edits — change the spec in its own repository"

vendored=$(python3 -c "import json;print(json.load(open('$SPEC/conformance/manifest.json'))['suiteVersion'])")
declared=$(python3 -c "import json;print(json.load(open('conformance/port.json'))['suiteVersion'])")
[ "$vendored" = "$declared" ] || fail "port.json declares suite $declared, the vendored suite is $vendored"

head=$(git -C "$SPEC" rev-parse HEAD)
tag=$(git -C "$SPEC" tag --points-at "$head" | head -1)
[ -n "$tag" ] || warn "the vendored spec ${head%"${head#???????}"} is not a tagged release (unreleased spec $vendored)" soft

if [ -d ../grenton-core-spec/.git ]; then
    sibling=$(git -C ../grenton-core-spec rev-parse HEAD)
    [ "$sibling" = "$head" ] || warn "vendored spec is at ${head%"${head#???????}"}, ../grenton-core-spec at ${sibling%"${sibling#???????}"}"
fi
echo "spec sync: suite $declared${tag:+, tag $tag}"
