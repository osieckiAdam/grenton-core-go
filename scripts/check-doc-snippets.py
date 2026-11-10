#!/usr/bin/env python3
"""Gate: every Go code block in the documentation compiles and vets against
the public API, as a program using the module would.

    scripts/check-doc-snippets.py

Collects the ```go blocks of README.md and the `language-go` blocks of the
site pages (site/), and builds each one as its own package of a separate
module that requires this one through a local replace, so only exported
identifiers are reachable (no internal/ package, no test helpers).

- A block that starts with a `package` clause is a complete file and is
  built unchanged.
- Any other block is a fragment: it is wrapped in a function body, and it
  gets an import for each package it names (the standard-library packages
  and this module's packages listed in IMPORTS, when `name.` appears).
  Fragments may use `ctx` (context.Context) and `cfg`
  (devices.SystemConfig) without declaring them; they arrive as
  parameters. Like Go itself, a fragment that declares a variable it does
  not use fails: the documentation has to be fixed.

Doc comments are not collected: in Go the compiled form of documentation
is the Example functions, which `go test` already builds and runs.
"""
import html
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
MODULE = "github.com/osieckiAdam/grenton-core-go"
IMPORTS = {
    "context": "context", "errors": "errors", "fmt": "fmt", "log": "log", "slog": "log/slog",
    "os": "os", "time": "time",
    "grenton": MODULE, "clu": MODULE + "/clu", "devices": MODULE + "/devices",
    "model": MODULE + "/model", "provisioning": MODULE + "/provisioning",
}
PARAMS = {"ctx": "context.Context", "cfg": "devices.SystemConfig"}


def markdown_blocks(path):
    return re.findall(r"^```go\n(.*?)^```", path.read_text(), re.S | re.M)


def site_blocks(path):
    blocks = re.findall(r'<code class="language-go">(.*?)</code>', path.read_text(), re.S)
    return [html.unescape(re.sub(r"<[^>]+>", "", b)) for b in blocks]


def wrap(index, origin, block):
    if re.match(r"\s*(//.*\n\s*)*package \w+", block):
        return f"// {origin}\n{block}"
    body = block.rstrip("\n")
    declared = set(re.findall(r"\b(\w+)\s*(?:,\s*\w+\s*)*:=", body)) | set(re.findall(r"\bvar\s+(\w+)", body))
    params = {n: t for n, t in PARAMS.items() if n not in declared and re.search(rf"\b{n}\b", body)}
    signature = " ".join(params.values())
    used = {name for name in IMPORTS if re.search(rf"(?<![\w.]){name}\.", body + signature)}
    imports = "\n".join(f'\t"{IMPORTS[n]}"' if IMPORTS[n].rsplit("/", 1)[-1] == n
                        else f'\t{n} "{IMPORTS[n]}"' for n in sorted(used))
    indented = "\n".join("\t" + l if l else l for l in body.split("\n"))
    args = ", ".join(f"{n} {t}" for n, t in params.items())
    return (f"// {origin}\npackage snippet{index}\n\nimport (\n{imports}\n)\n\n"
            f"func snippet({args}) {{\n{indented}\n}}\n")


sources = [(ROOT / "README.md", markdown_blocks)] + [(p, site_blocks) for p in sorted(ROOT.glob("site/**/*.html"))]
files = []
for path, collect in sources:
    for n, block in enumerate(collect(path), 1):
        files.append(wrap(len(files), f"{path.relative_to(ROOT)}, Go block {n}", block))

work = pathlib.Path(tempfile.mkdtemp(prefix="grenton-doc-snippets-"))
try:
    (work / "go.mod").write_text(
        f"module docsnippets\n\ngo 1.26\n\nrequire {MODULE} v0.0.0\n\nreplace {MODULE} => {ROOT}\n")
    for index, code in enumerate(files):
        (work / f"s{index}").mkdir()
        (work / f"s{index}" / "snippet.go").write_text(code)
    failed = False
    for command in (["go", "build", "./..."], ["go", "vet", "./..."]):
        result = subprocess.run(command, cwd=work, capture_output=True, text=True,
                                env={**os.environ, "GOFLAGS": "-mod=mod", "GOWORK": "off"})
        if result.returncode != 0:
            failed = True
            print(f"$ {' '.join(command)}")
            print((result.stdout + result.stderr).strip())
            for index, code in enumerate(files):
                print(f"--- s{index}/snippet.go ({code.splitlines()[0][3:]})")
            break
    print(f"doc snippets: {len(files)} Go block(s) from README.md and site/, {'FAILED' if failed else 'OK'}")
finally:
    shutil.rmtree(work, ignore_errors=True)
sys.exit(1 if failed else 0)
