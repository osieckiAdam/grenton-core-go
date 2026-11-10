#!/usr/bin/env python3
"""Gate: every internal link of the built site points at something that exists.

    scripts/check-site-links.py      (after scripts/docs.sh build)

Checks every page in _site: href and src values without a scheme must be
relative (a root-relative "/…" link breaks under GitHub Pages'
/grenton-core-go/ base path) and resolve to a file, and a #fragment must
name an id on the target page. External links (https:, mailto:) are not
fetched. The repository-relative links of README.md are checked too:
they must name a path in the repository (tracked, or new and not ignored), and a #fragment one of its headings.
"""
import pathlib
import re
import subprocess
import sys
from urllib.parse import unquote, urldefrag, urlparse

ROOT = pathlib.Path(__file__).resolve().parent.parent
SITE = ROOT / "_site"
if not (SITE / "index.html").is_file():
    sys.exit("site links: no built site — run scripts/docs.sh build first")


def html_ids(path):
    return set(re.findall(r'\bid="([^"]+)"', path.read_text()))


def markdown_anchors(path):
    anchors = set()
    for heading in re.findall(r"^#+ (.+)$", path.read_text(), re.M):
        slug = re.sub(r"[^\w\- ]", "", heading.strip().lower()).replace(" ", "-")
        anchors.add(slug)
    return anchors


broken = []
pages = sorted(SITE.rglob("*.html"))
for page in pages:
    for link in re.findall(r'(?:href|src)="([^"]+)"', page.read_text()):
        target, fragment = urldefrag(link)
        if urlparse(link).scheme or link.startswith("//"):
            continue
        if target.startswith("/"):
            broken.append(f"{page.relative_to(SITE)}: {link} (root-relative; breaks under a base path)")
            continue
        resolved = (page.parent / unquote(target)).resolve() if target else page
        if resolved.is_dir():
            resolved = resolved / "index.html"
        if not resolved.is_file() or SITE.resolve() not in resolved.parents:
            broken.append(f"{page.relative_to(SITE)}: {link}")
        elif fragment and resolved.suffix == ".html" and fragment not in html_ids(resolved):
            broken.append(f"{page.relative_to(SITE)}: {link} (no id {fragment!r})")

# Tracked files and new ones not ignored, with every directory above them.
files = subprocess.run(["git", "ls-files", "--cached", "--others", "--exclude-standard"],
                       cwd=ROOT, capture_output=True, text=True, check=True).stdout.split("\n")
tracked = {f for f in files if f}
tracked |= {str(parent) for f in list(tracked) for parent in pathlib.PurePosixPath(f).parents}
readme = ROOT / "README.md"
for link in re.findall(r"\]\(([^)\s]+)\)", readme.read_text()):
    target, fragment = urldefrag(link)
    if urlparse(link).scheme:
        continue
    if not target:
        if fragment not in markdown_anchors(readme):
            broken.append(f"README.md: {link} (no heading)")
        continue
    if target.rstrip("/") not in tracked:
        broken.append(f"README.md: {link}")

for b in broken:
    print("broken link:", b)
print(f"site links: {len(pages)} page(s) and README.md, {len(broken)} broken")
sys.exit(1 if broken else 0)
