package conformance_test

// The loader fails closed (conformance.md §5). Each rule is proven on a
// temporary copy of the vendored suite with one thing broken.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copySuite copies the vendored suite and port.json into a temporary
// directory and returns the copy's conformance root and port path.
func copySuite(t *testing.T) (root, port string) {
	t.Helper()
	dir := t.TempDir()
	root = filepath.Join(dir, "conformance")
	err := filepath.WalkDir(suiteRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(suiteRoot, p)
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	port = filepath.Join(dir, "port.json")
	writeFile(t, port, `{"suiteVersion": "`+theSuite(t).Version+`", "profiles": ["stateless"], "skips": []}`)
	return root, port
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expectLoadError(t *testing.T, root, port, want string) {
	t.Helper()
	_, err := loadSuite(root, port)
	if err == nil {
		t.Fatalf("the loader accepted a broken suite; want an error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not mention %q", err, want)
	}
}

// aCase returns the path of one case file in the copy.
func aCase(t *testing.T, root string) string {
	t.Helper()
	return filepath.Join(root, "cases", "stateless", "value", "value.cast.nil-lower.json")
}

func TestFailsClosedOnMissingSuite(t *testing.T) {
	_, port := copySuite(t)
	expectLoadError(t, filepath.Join(t.TempDir(), "absent"), port, "git submodule update --init")
}

func TestFailsClosedOnChecksumMismatch(t *testing.T) {
	root, port := copySuite(t)
	p := aCase(t, root)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, string(data)+" ")
	expectLoadError(t, root, port, "SHA-256 does not match")
}

func TestFailsClosedOnUnlistedCase(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, filepath.Join(root, "cases", "stateless", "value.cast.extra.json"), "{}")
	expectLoadError(t, root, port, "is not listed in manifest.json")
}

func TestFailsClosedOnUnlistedFixture(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, filepath.Join(root, "fixtures", "stray.zip"), "PK")
	expectLoadError(t, root, port, "is not listed in manifest.json")
}

func TestFailsClosedOnUnknownOperation(t *testing.T) {
	root, port := copySuite(t)
	rewriteCase(t, root, `"operation": "value.cast"`, `"operation": "value.guess"`)
	expectLoadError(t, root, port, "belongs to no manifest profile")
}

func TestFailsClosedOnUnknownRequirement(t *testing.T) {
	root, port := copySuite(t)
	rewriteCase(t, root, `"model.cast"`, `"model.nothing"`)
	expectLoadError(t, root, port, "unknown requirement")
}

func TestFailsClosedOnExtraCaseField(t *testing.T) {
	root, port := copySuite(t)
	rewriteCase(t, root, `"schemaVersion": 1,`, `"schemaVersion": 1, "note": "x",`)
	expectLoadError(t, root, port, "fields must be exactly")
}

func TestFailsClosedOnSuiteVersionMismatch(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, port, `{"suiteVersion": "0.0.1", "profiles": [], "skips": []}`)
	expectLoadError(t, root, port, "declares suite 0.0.1")
}

func TestFailsClosedOnUnknownProfile(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, port, `{"suiteVersion": "`+theSuite(t).Version+`", "profiles": ["telepathy"], "skips": []}`)
	expectLoadError(t, root, port, "does not define")
}

func TestFailsClosedOnSkipOfUnknownCase(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, port, `{"suiteVersion": "`+theSuite(t).Version+`", "profiles": ["stateless"],
		"skips": [{"id": "value.cast.no-such-case", "reason": "x"}]}`)
	expectLoadError(t, root, port, "skips unknown case")
}

func TestFailsClosedOnSkipOutsideDeclaredProfiles(t *testing.T) {
	root, port := copySuite(t)
	writeFile(t, port, `{"suiteVersion": "`+theSuite(t).Version+`", "profiles": ["mapping"],
		"skips": [{"id": "value.cast.nil-lower", "reason": "x"}]}`)
	expectLoadError(t, root, port, "outside the declared profiles")
}

// A handler missing from the registry fails the case instead of passing it.
func TestUnknownOperationHandlerFailsTheCase(t *testing.T) {
	_, _, _, err := runCase(theSuite(t), suiteCase{ID: "x", Operation: "value.guess", Input: map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "no handler") {
		t.Fatalf("want a runner error for a missing handler, got %v", err)
	}
}

// A skipped case that passes is stale: TestConformance fails it. The
// decision is the same comparison the runner makes, proven here on a case
// whose observation equals its expectation.
func TestStaleSkipIsDetected(t *testing.T) {
	expected := map[string]any{"outcome": "reject"}
	a, _ := normalize(expected)
	b, _ := normalize(reject)
	if firstDifference(a, b, "$") != "" {
		t.Fatal("a passing case must compare equal, so that its skip is reported as stale")
	}
}

// rewriteCase edits one case in the copy and re-hashes it in the manifest,
// so that only the edited property is wrong.
func rewriteCase(t *testing.T, root, old, new string) {
	t.Helper()
	p := aCase(t, root)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", p, old)
	}
	edited := strings.Replace(string(data), old, new, 1)
	writeFile(t, p, edited)
	manifestPath := filepath.Join(root, "manifest.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, manifestPath, strings.Replace(string(manifest), sha256Hex(data), sha256Hex([]byte(edited)), 1))
}
