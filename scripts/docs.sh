#!/bin/sh
# Builds the documentation site and serves it locally.
#
#   scripts/docs.sh          build, then serve on http://localhost:8000/
#   scripts/docs.sh build    build only, into _site
#
# The site is the landing page and Getting Started from site/, plus the
# screenshot from docs/images. There is no generated reference: the API
# reference is pkg.go.dev, built from the doc comments. Every link inside
# the site is relative (scripts/check-site-links.py enforces it), so the
# same build works at the root and under GitHub Pages' /grenton-core-go/.
set -e
cd "$(dirname "$0")/.."

SITE=_site
rm -rf "$SITE"
mkdir -p "$SITE"
cp -R site/. "$SITE/"
cp docs/images/om-send-to-mobile.png "$SITE/assets/"

for page in index.html getting-started/index.html assets/style.css assets/site.js; do
    if [ ! -f "$SITE/$page" ]; then
        echo "error: $page is missing from the site" >&2
        exit 1
    fi
done
echo "site: built into $SITE"

[ "$1" = "build" ] && exit 0

# Take the first free port from $PORT (default 8000) on; an earlier run may
# still be serving in another terminal.
PORT=$(python3 - "${PORT:-8000}" <<'PY'
import socket, sys
start = int(sys.argv[1])
for port in range(start, start + 20):
    with socket.socket() as s:
        try:
            s.bind(("127.0.0.1", port))
        except OSError:
            continue
        print(port)
        break
else:
    sys.exit(f"no free port in {start}-{start + 19}")
PY
)
echo "Documentation: http://localhost:$PORT/"
echo "Press Ctrl-C to stop."
# no-store: after a rebuild the browser must not show a cached older site.
python3 - "$PORT" "$SITE" <<'PY'
import functools, http.server, sys

class NoCache(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-store")
        super().end_headers()

port, root = int(sys.argv[1]), sys.argv[2]
handler = functools.partial(NoCache, directory=root)
with http.server.ThreadingHTTPServer(("127.0.0.1", port), handler) as server:
    print(f"Serving {root} on http://127.0.0.1:{port}/")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
PY
