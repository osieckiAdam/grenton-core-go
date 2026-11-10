// Package archtest enforces the module boundaries of spec §2:
// provisioning → model ← clu ← devices, helpers on top. Every package's
// imports of this module must be in its allowed set.
package archtest

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/osieckiAdam/grenton-core-go"

// allowed lists, per package, the module packages it may import.
var allowed = map[string][]string{
	"model":               {},
	"internal/strictjson": {},
	"internal/wire":       {"model"},
	"internal/normalize":  {"model"},
	"clu":                 {"model", "internal/wire"},
	"provisioning":        {"model", "internal/strictjson"},
	"devices":             {"model", "clu", "internal/normalize"},
	"internal/clutest":    {"clu"},
	"":                    {"model", "clu", "devices", "provisioning"}, // the grenton umbrella
}

func TestDependencyDirection(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if strings.HasPrefix(rel, "vendor") || strings.HasPrefix(rel, ".") || strings.HasPrefix(rel, "dist") {
			return filepath.SkipDir
		}
		pkg, err := build.ImportDir(p, 0)
		if err != nil {
			return nil // no Go files
		}
		rules, known := allowed[rel]
		if !known {
			// Tests, examples and the conformance runner may use anything.
			if rel == "conformance" || rel == "internal/archtest" || strings.HasPrefix(rel, "examples") {
				return nil
			}
			t.Errorf("package %q has no rule here; add it to allowed", rel)
			return nil
		}
		seen[rel] = true
		for _, imp := range pkg.Imports {
			if !strings.HasPrefix(imp, module) {
				continue
			}
			target := strings.TrimPrefix(strings.TrimPrefix(imp, module), "/")
			ok := false
			for _, a := range rules {
				ok = ok || a == target
			}
			if !ok {
				t.Errorf("%q imports %q, which spec §2 does not allow", rel, target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg := range allowed {
		if !seen[pkg] {
			t.Errorf("rule for %q matches no package", pkg)
		}
	}
}
