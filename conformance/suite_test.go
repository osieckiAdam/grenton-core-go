package conformance_test

// Loads the language-neutral conformance suite of grenton-core-spec
// (spec/conformance.md) from the vendor/grenton-core-spec submodule. The
// loader fails closed (conformance.md §5): a missing checkout, a manifest
// that does not match the files, a malformed case or a bad port
// declaration is an error, never "0 tests ran".

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const supportedSchemaVersion = 1

type suiteCase struct {
	ID        string
	Profile   string
	Operation string
	Input     map[string]any
	Expect    map[string]any
}

type portSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Issue  string `json:"issue,omitempty"`
}

type portDeclaration struct {
	SuiteVersion string     `json:"suiteVersion"`
	Profiles     []string   `json:"profiles"`
	Skips        []portSkip `json:"skips"`
}

type suite struct {
	Root     string // the conformance/ directory
	Version  string
	Cases    map[string]suiteCase
	Profiles map[string][]string
	Port     portDeclaration
}

// declared reports whether the port claims the case's profile.
func (s *suite) declared(c suiteCase) bool {
	for _, p := range s.Port.Profiles {
		if p == c.Profile {
			return true
		}
	}
	return false
}

func (s *suite) skip(id string) (portSkip, bool) {
	for _, sk := range s.Port.Skips {
		if sk.ID == id {
			return sk, true
		}
	}
	return portSkip{}, false
}

// sortedIDs lists the case ids in a stable order.
func (s *suite) sortedIDs() []string {
	ids := make([]string, 0, len(s.Cases))
	for id := range s.Cases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// decodeJSON decodes a document with numbers kept as json.Number, so that
// integers in inputs (subscription numbers, ports) are not rounded.
func decodeJSON(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after the JSON value")
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// loadSuite reads and checks the suite under root (a conformance/
// directory) and the port declaration at portPath.
func loadSuite(root, portPath string) (*suite, error) {
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("conformance suite not found at %s — run `git submodule update --init`", root)
		}
		return nil, err
	}
	var manifest struct {
		SchemaVersion json.Number                `json:"schemaVersion"`
		SuiteVersion  string                     `json:"suiteVersion"`
		Profiles      map[string][]string        `json:"profiles"`
		Requirements  map[string]json.RawMessage `json:"requirements"`
		Cases         []struct{ ID, Path, Sha256 string }
		Fixtures      []struct{ Path, Sha256 string }
	}
	if err := decodeJSON(data, &manifest); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	if manifest.SchemaVersion.String() != fmt.Sprint(supportedSchemaVersion) {
		return nil, fmt.Errorf("unsupported manifest schemaVersion %q", manifest.SchemaVersion)
	}
	if manifest.SuiteVersion == "" || manifest.Cases == nil || manifest.Profiles == nil || manifest.Requirements == nil {
		return nil, errors.New("manifest.json lacks suiteVersion, cases, profiles or requirements")
	}

	profileOf := map[string]string{}
	for profile, operations := range manifest.Profiles {
		for _, op := range operations {
			profileOf[op] = profile
		}
	}

	s := &suite{Root: root, Version: manifest.SuiteVersion, Cases: map[string]suiteCase{}, Profiles: manifest.Profiles}
	listed := map[string]bool{}
	for _, entry := range manifest.Cases {
		if entry.ID == "" || entry.Path == "" || entry.Sha256 == "" {
			return nil, fmt.Errorf("malformed manifest case entry %+v", entry)
		}
		body, err := readListed(root, entry.Path, entry.Sha256)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(entry.Path, "cases/") || path.Base(entry.Path) != entry.ID+".json" {
			return nil, fmt.Errorf("%s: a case lives under cases/ in a file named after its id (%s.json)", entry.Path, entry.ID)
		}
		c, err := decodeCase(body, entry.Path, manifest.Requirements)
		if err != nil {
			return nil, err
		}
		if c.ID != entry.ID {
			return nil, fmt.Errorf("%s: id %q does not match the manifest's %q", entry.Path, c.ID, entry.ID)
		}
		profile, ok := profileOf[c.Operation]
		if !ok {
			return nil, fmt.Errorf("%s: operation %s belongs to no manifest profile", entry.Path, c.Operation)
		}
		c.Profile = profile
		if _, dup := s.Cases[c.ID]; dup {
			return nil, fmt.Errorf("duplicate case id %s", c.ID)
		}
		s.Cases[c.ID] = c
		listed[entry.Path] = true
	}
	for _, entry := range manifest.Fixtures {
		if entry.Path == "" || entry.Sha256 == "" {
			return nil, fmt.Errorf("malformed manifest fixture entry %+v", entry)
		}
		if _, err := readListed(root, entry.Path, entry.Sha256); err != nil {
			return nil, err
		}
		listed[entry.Path] = true
	}
	// Every file on disk must be listed: an unlisted one would never run.
	for _, dir := range []string{"cases", "fixtures"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if !listed[rel] {
				return fmt.Errorf("%s is not listed in manifest.json", rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	portData, err := os.ReadFile(portPath)
	if err != nil {
		return nil, fmt.Errorf("port declaration: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(portData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s.Port); err != nil {
		return nil, fmt.Errorf("port.json: %w", err)
	}
	if s.Port.Profiles == nil || s.Port.Skips == nil {
		return nil, errors.New("port.json needs suiteVersion, profiles and skips")
	}
	if s.Port.SuiteVersion != s.Version {
		return nil, fmt.Errorf("port.json declares suite %s, the vendored suite is %s", s.Port.SuiteVersion, s.Version)
	}
	for _, p := range s.Port.Profiles {
		if _, ok := manifest.Profiles[p]; !ok {
			return nil, fmt.Errorf("port.json declares profile %s, which the suite does not define", p)
		}
	}
	for _, sk := range s.Port.Skips {
		c, ok := s.Cases[sk.ID]
		if !ok {
			return nil, fmt.Errorf("port.json skips unknown case %s", sk.ID)
		}
		if !s.declared(c) {
			return nil, fmt.Errorf("port.json skips %s, which is outside the declared profiles", sk.ID)
		}
		if sk.Reason == "" {
			return nil, fmt.Errorf("port.json: skip %s needs a reason", sk.ID)
		}
	}
	return s, nil
}

func readListed(root, rel, want string) ([]byte, error) {
	if path.IsAbs(rel) || strings.Contains(rel, "..") {
		return nil, fmt.Errorf("manifest path %s leaves the suite", rel)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("%s: listed in manifest.json but unreadable: %w", rel, err)
	}
	if sha256Hex(data) != want {
		return nil, fmt.Errorf("%s: SHA-256 does not match the manifest", rel)
	}
	return data, nil
}

// decodeCase checks the case shape of conformance.md §2: exactly these
// fields, a known schema version, non-empty specRefs naming known
// requirements, and one of the three outcomes.
func decodeCase(data []byte, where string, requirements map[string]json.RawMessage) (suiteCase, error) {
	fail := func(format string, args ...any) (suiteCase, error) {
		return suiteCase{}, fmt.Errorf("%s: %s", where, fmt.Sprintf(format, args...))
	}
	var doc map[string]any
	if err := decodeJSON(data, &doc); err != nil {
		return fail("not a JSON object: %v", err)
	}
	want := []string{"description", "expect", "id", "input", "operation", "provenance", "schemaVersion", "specRefs"}
	got := make([]string, 0, len(doc))
	for k := range doc {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		return fail("fields must be exactly %v, got %v", want, got)
	}
	if n, ok := doc["schemaVersion"].(json.Number); !ok || n.String() != fmt.Sprint(supportedSchemaVersion) {
		return fail("unsupported schemaVersion %v", doc["schemaVersion"])
	}
	for _, k := range []string{"id", "description", "provenance", "operation"} {
		if _, ok := doc[k].(string); !ok {
			return fail("%s must be a string", k)
		}
	}
	refs, ok := doc["specRefs"].([]any)
	if !ok || len(refs) == 0 {
		return fail("specRefs must be a non-empty list")
	}
	for _, r := range refs {
		id, ok := r.(string)
		if !ok || id == "" {
			return fail("specRefs must hold non-empty strings")
		}
		if _, known := requirements[id]; !known {
			return fail("unknown requirement %q in specRefs", id)
		}
	}
	input, ok := doc["input"].(map[string]any)
	if !ok {
		return fail("input must be an object")
	}
	expect, ok := doc["expect"].(map[string]any)
	if !ok {
		return fail("expect must be an object")
	}
	keys := make([]string, 0, len(expect))
	for k := range expect {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	shape := strings.Join(keys, ",")
	switch expect["outcome"] {
	case "ok":
		if shape != "outcome,value" {
			return fail("an ok expectation is {outcome, value}")
		}
	case "error":
		if _, isString := expect["code"].(string); shape != "code,outcome" || !isString {
			return fail("an error expectation is {outcome, code}")
		}
	case "reject":
		if shape != "outcome" {
			return fail("a reject expectation is {outcome}")
		}
	default:
		return fail("expect.outcome must be ok, error or reject")
	}
	return suiteCase{
		ID: doc["id"].(string), Operation: doc["operation"].(string),
		Input: input, Expect: expect,
	}, nil
}
