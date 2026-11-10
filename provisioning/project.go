package provisioning

import (
	"archive/zip"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/osieckiAdam/grenton-core-go/internal/strictjson"
	"github.com/osieckiAdam/grenton-core-go/model"
)

// ProjectLimit is the limit of spec §4.5.1 item 3 (recommended): for the
// file, each used entry after decompression and their total.
const ProjectLimit = 64 * mib

// projectCluPort is the CLU port; the project does not store it (§4.5.4).
const projectCluPort = 1234

var nodeEntry = regexp.MustCompile(`^nodes/[0-9]+/data\.json$`)

// Project is an Object Manager project file (.omp) opened for import (spec
// §4.5). It needs neither the PIN nor a running Object Manager.
type Project struct {
	interfaces []map[string]any
	properties *xmlDoc
	system     *xmlDoc
}

// InterfaceInfo names one interface of a project.
type InterfaceInfo struct {
	ID   string
	Name string
}

// OpenProject reads a project: the archive (§4.5.1), properties.xml and
// system.xml, and every node, to list its interfaces (§4.5.2). Every error
// is an [InvalidData] error.
func OpenProject(data []byte) (*Project, error) {
	if len(data) > ProjectLimit {
		return nil, invalidData("the project is larger than %d bytes", ProjectLimit)
	}
	r, err := openZip(data)
	if err != nil {
		return nil, err
	}
	entries := map[string]*zip.File{}
	var nodes []*zip.File
	for _, f := range r.File {
		if _, dup := entries[f.Name]; dup {
			return nil, invalidData("two archive entries named %q", f.Name)
		}
		entries[f.Name] = f
		if nodeEntry.MatchString(f.Name) {
			nodes = append(nodes, f)
		}
	}
	var total int64
	read := func(f *zip.File) ([]byte, error) {
		raw, err := r.readEntry(f, ProjectLimit)
		if err != nil {
			return nil, err
		}
		if total += int64(len(raw)); total > ProjectLimit {
			return nil, invalidData("the used entries inflate beyond %d bytes", ProjectLimit)
		}
		return raw, nil
	}
	p := &Project{}
	for _, name := range []string{"properties.xml", "system.xml"} {
		f, ok := entries[name]
		if !ok {
			return nil, invalidData("the project has no %s", name)
		}
		raw, err := read(f)
		if err != nil {
			return nil, err
		}
		doc, err := parseXML(raw)
		if err != nil {
			return nil, err
		}
		if name == "properties.xml" {
			p.properties = doc
		} else {
			p.system = doc
		}
	}
	ids := map[string]bool{}
	for _, f := range nodes {
		raw, err := read(f)
		if err != nil {
			return nil, err
		}
		v, err := strictjson.Parse(raw)
		if err != nil {
			return nil, invalidData("%s: %v", f.Name, err)
		}
		node, isObj := v.(map[string]any)
		if !isObj {
			continue
		}
		if _, isList := node["pages"].([]any); !isList {
			continue
		}
		id, okID := node["id"].(string)
		_, okLabel := node["label"].(string)
		if !okID || !okLabel {
			return nil, invalidData("%s: an interface without a string id and label", f.Name)
		}
		if ids[id] {
			return nil, invalidData("two interfaces with id %q", id)
		}
		ids[id] = true
		p.interfaces = append(p.interfaces, node)
	}
	return p, nil
}

// Interfaces lists the project's interfaces in archive order.
func (p *Project) Interfaces() []InterfaceInfo {
	out := make([]InterfaceInfo, len(p.interfaces))
	for i, in := range p.interfaces {
		out[i] = InterfaceInfo{ID: in["id"].(string), Name: in["label"].(string)}
	}
	return out
}

// Interface imports the interface with the given id; with id "" the
// project's only interface. No match, or no interface at all, is
// [InterfaceNotFound]; several without an id is [InterfaceAmbiguous].
func (p *Project) Interface(id string) (*model.Interface, error) {
	var editor map[string]any
	switch {
	case id != "":
		for _, in := range p.interfaces {
			if in["id"] == id {
				editor = in
			}
		}
		if editor == nil {
			return nil, &Error{Kind: InterfaceNotFound, Detail: "no interface with id " + id}
		}
	case len(p.interfaces) == 1:
		editor = p.interfaces[0]
	case len(p.interfaces) == 0:
		return nil, &Error{Kind: InterfaceNotFound, Detail: "the project has no interface"}
	default:
		return nil, &Error{Kind: InterfaceAmbiguous, Detail: strconv.Itoa(len(p.interfaces)) + " interfaces and no id"}
	}
	mobile, err := p.convert(editor)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(mobile)
	if err != nil {
		return nil, invalidData("%v", err)
	}
	in, err := model.DecodeInterface(raw)
	if err != nil {
		return nil, invalidData("%v", err)
	}
	return in, nil
}

// convert builds the data.json root of §4.5.3.
func (p *Project) convert(editor map[string]any) (map[string]any, error) {
	key, err := p.firstText(p.properties.root, "keyBytes")
	if err != nil {
		return nil, err
	}
	iv, err := p.firstText(p.properties.root, "ivBytes")
	if err != nil {
		return nil, err
	}
	if key == "" || iv == "" {
		return nil, invalidData("properties.xml has no keyBytes or ivBytes")
	}
	icon, okIcon := editor["icon"].(string)
	theme, okTheme := editor["theme"].(string)
	if !okIcon || !okTheme {
		return nil, invalidData("the interface's icon or theme is not a string")
	}
	rawPages, _ := editor["pages"].([]any)
	pages := make([]any, 0, len(rawPages))
	seen := map[string]bool{}
	for _, rp := range rawPages {
		page, err := convertPage(rp)
		if err != nil {
			return nil, err
		}
		for _, w := range page["widgets"].([]any) {
			if id, has := w.(map[string]any)["id"].(string); has {
				if seen[id] {
					return nil, invalidData("two widgets with id %q", id)
				}
				seen[id] = true
			}
		}
		pages = append(pages, page)
	}
	clus, err := p.clus(editor, usedClus(pages))
	if err != nil {
		return nil, err
	}
	pushes, err := p.pushNotifications()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": editor["id"], "version": 0, "name": editor["label"], "icon": icon, "theme": strings.ToUpper(theme),
		"encryption": map[string]any{"key": key, "iv": iv},
		"clus":       clus, "pages": pages, "pushNotifications": pushes,
	}, nil
}

// firstText is the text of the first element named name in document order.
func (p *Project) firstText(root *xmlElement, name string) (string, error) {
	stack := []*xmlElement{root}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if e.name == name {
			r, err := p.properties.resolve(e)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(r.text), nil
		}
		for i := len(e.children) - 1; i >= 0; i-- {
			stack = append(stack, e.children[i])
		}
	}
	return "", nil
}

// documentOrder lists every element of a document, depth first.
func documentOrder(root *xmlElement) []*xmlElement {
	var out []*xmlElement
	stack := []*xmlElement{root}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, e)
		for i := len(e.children) - 1; i >= 0; i-- {
			stack = append(stack, e.children[i])
		}
	}
	return out
}

// clus are the project's CLUs referenced by the interface, in document
// order (§4.5.4).
func (p *Project) clus(editor map[string]any, used map[string]bool) ([]any, error) {
	connection := "CLOUD"
	if editor["blockCloud"] == true {
		connection = "LOCAL_ONLY"
	}
	out := []any{}
	seen := map[string]bool{}
	for _, e := range documentOrder(p.system.root) {
		if e.child("nameOnCLU") == nil || e.child("ipAddress") == nil {
			continue
		}
		id, err := p.system.text(e, "nameOnCLU")
		if err != nil {
			return nil, err
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if !used[id] {
			continue
		}
		name, err := p.system.text(e, "name")
		if err != nil {
			return nil, err
		}
		ip, err := p.system.text(e, "ipAddress")
		if err != nil {
			return nil, err
		}
		serialText, err := p.system.text(e, "serialNumber")
		if err != nil {
			return nil, err
		}
		serial, okSerial := hexNumber(serialText)
		if name == "" || ip == "" || !okSerial {
			return nil, invalidData("CLU %s lacks a name, an IP address or a serial number", id)
		}
		out = append(out, map[string]any{"id": id, "name": name, "ip": ip,
			"serialNumber": strconv.FormatUint(serial, 10), "port": projectCluPort, "connectionType": connection})
	}
	return out, nil
}

// pushNotifications walks the TreeObject tree depth first, each node once
// (§4.5.5), without recursion.
func (p *Project) pushNotifications() ([]any, error) {
	type visit struct {
		node   *xmlElement
		serial *uint64 // the serial number of the nearest CLU ancestor
	}
	out := []any{}
	var stack []visit
	push := func(parent *xmlElement, serial *uint64) {
		var kids []*xmlElement
		for _, c := range parent.children {
			if c.name == "TreeObject" {
				kids = append(kids, c)
			}
		}
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, visit{kids[i], serial})
		}
	}
	push(p.system.root, nil)
	visited := map[*xmlElement]bool{}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node, err := p.system.resolve(v.node)
		if err != nil {
			return nil, err
		}
		if visited[node] {
			continue
		}
		visited[node] = true
		obj, err := p.system.child(node, "specificObject")
		if err != nil {
			return nil, err
		}
		here := v.serial
		if obj != nil {
			typeName, err := p.system.text(obj, "typeName")
			if err != nil {
				return nil, err
			}
			if typeName == "Push" {
				entry, err := p.pushEntry(obj, v.serial)
				if err != nil {
					return nil, err
				}
				if entry != nil {
					out = append(out, entry)
				}
			}
			// An empty field is a missing one (§4.5.1 item 7).
			text, err := p.system.text(obj, "serialNumber")
			if err != nil {
				return nil, err
			}
			ip, err := p.system.text(obj, "ipAddress")
			if err != nil {
				return nil, err
			}
			if text != "" && ip != "" {
				if serial, ok := hexNumber(text); ok {
					here = &serial
				} else {
					here = nil
				}
			}
		}
		children, err := p.system.child(node, "children")
		if err != nil {
			return nil, err
		}
		if children != nil {
			push(children, here)
		}
	}
	return out, nil
}

// pushEntry is one Push object, or nil when it lacks a field (it is
// skipped, not an error: the field is parsed, not used).
func (p *Project) pushEntry(obj *xmlElement, serial *uint64) (map[string]any, error) {
	objectID, err := p.system.text(obj, "nameOnCLU")
	if err != nil {
		return nil, err
	}
	typeIDText, err := p.system.text(obj, "typeId")
	if err != nil {
		return nil, err
	}
	version, err := p.system.text(obj, "typeVersion")
	if err != nil {
		return nil, err
	}
	typeID, okType := hexNumber(typeIDText)
	if serial == nil || objectID == "" || !okType || version == "" {
		return nil, nil
	}
	return map[string]any{"cluId": *serial, "objectId": objectID,
		"objectType": "PUSH-" + strconv.FormatUint(typeID, 10) + "-" + version}, nil
}
