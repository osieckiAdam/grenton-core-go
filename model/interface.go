package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Interface is a mobile interface (spec §3.4): the configuration Object
// Manager sends to the myGrenton app.
type Interface struct {
	ID                string
	Version           int64
	Name              string
	Icon              string
	Theme             string
	Encryption        Encryption
	Clus              []Clu
	Pages             []Page
	PushNotifications []PushNotification
}

// Encryption is the key material shared by every CLU of an interface, base64.
type Encryption struct {
	Key string
	IV  string
}

// ConnectionType is a CLU's cloud setting; parsed, never used (spec §1.2).
type ConnectionType string

// The three connection types.
const (
	LocalOnly    ConnectionType = "LOCAL_ONLY"
	CloudCapable ConnectionType = "CLOUD_CAPABLE"
	Cloud        ConnectionType = "CLOUD"
)

// Clu is one controller of the interface.
type Clu struct {
	ID             string
	SerialNumber   string
	Name           string
	IP             string
	Port           int64
	ConnectionType ConnectionType
}

// PushNotification lists an object the CLU notifies about through the cloud;
// parsed, unused (spec §1.2).
type PushNotification struct {
	CluID      int64
	ObjectID   string
	ObjectType string
}

// Page is a UI page. Items holds the widgets and section headers that
// parsed, in order; Skipped the widgets that did not (spec §3.6 item 2).
type Page struct {
	Name               string
	Icon               string
	IsFullscreenWidget bool
	Items              []PageItem
	Skipped            []SkippedWidget
}

// Widgets returns the page's widgets without its headers.
func (p Page) Widgets() []Widget {
	out := make([]Widget, 0, len(p.Items))
	for _, it := range p.Items {
		if it.Widget != nil {
			out = append(out, it.Widget)
		}
	}
	return out
}

// PageItem is either a widget or a section header (exactly one is set).
type PageItem struct {
	Widget Widget
	Header *SectionHeader
}

// SectionHeader is a HEADER widget: layout, not a device (spec §3.5).
type SectionHeader struct {
	ID    string
	Label string
}

// SkippedWidget records a widget that did not parse. Type is its type, or
// nil when it has none; Reason is a diagnostic for logs.
type SkippedWidget struct {
	Type   *string
	Reason string
}

// themes are the 15 values of spec §3.4.
var themes = []string{"GRENTON", "RED", "ORANGE", "YELLOW", "LIME", "GREEN", "STEEL", "TURQUOISE",
	"BLUE", "INDIGO", "VIOLET", "PURPLE", "PINK", "BEIGE", "BROWN"}

// ErrInvalidInterface is returned when an interface does not decode as a
// whole (an error at the root, a page or a CLU; spec §3.6 item 2).
var ErrInvalidInterface = errors.New("invalid mobile interface")

// DecodeInterface parses data.json (spec §3.4–§3.6). A widget that does not
// parse is skipped and recorded on its page; anything else that does not
// parse fails the whole interface with [ErrInvalidInterface].
func DecodeInterface(data []byte) (*Interface, error) {
	in, err := decodeInterface(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInterface, err)
	}
	return in, nil
}

func decodeInterface(data []byte) (*Interface, error) {
	o, err := decodeObject(data)
	if err != nil {
		return nil, err
	}
	var in Interface
	if in.ID, err = o.str("id"); err != nil {
		return nil, err
	}
	if in.Version, err = o.integer("version"); err != nil {
		return nil, err
	}
	if in.Name, err = o.str("name"); err != nil {
		return nil, err
	}
	if in.Icon, err = o.str("icon"); err != nil {
		return nil, err
	}
	if in.Theme, err = o.enum("theme", themes...); err != nil {
		return nil, err
	}
	enc, err := o.child("encryption")
	if err != nil {
		return nil, err
	}
	if in.Encryption.Key, err = enc.str("key"); err != nil {
		return nil, err
	}
	if in.Encryption.IV, err = enc.str("iv"); err != nil {
		return nil, err
	}
	clus, err := o.array("clus")
	if err != nil {
		return nil, err
	}
	for n, raw := range clus {
		c, err := decodeClu(raw)
		if err != nil {
			return nil, fmt.Errorf("clus[%d]: %w", n, err)
		}
		in.Clus = append(in.Clus, c)
	}
	pages, err := o.array("pages")
	if err != nil {
		return nil, err
	}
	for n, raw := range pages {
		p, err := decodePage(raw)
		if err != nil {
			return nil, fmt.Errorf("pages[%d]: %w", n, err)
		}
		in.Pages = append(in.Pages, p)
	}
	pushes, err := o.array("pushNotifications")
	if err != nil {
		return nil, err
	}
	for n, raw := range pushes {
		p, err := decodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("pushNotifications[%d]: %w", n, err)
		}
		var pn PushNotification
		if pn.CluID, err = p.integer("cluId"); err == nil {
			if pn.ObjectID, err = p.str("objectId"); err == nil {
				pn.ObjectType, err = p.str("objectType")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("pushNotifications[%d]: %w", n, err)
		}
		in.PushNotifications = append(in.PushNotifications, pn)
	}
	return &in, nil
}

func decodeClu(raw json.RawMessage) (Clu, error) {
	var c Clu
	o, err := decodeObject(raw)
	if err != nil {
		return c, err
	}
	if c.ID, err = o.str("id"); err != nil {
		return c, err
	}
	if c.SerialNumber, err = o.str("serialNumber"); err != nil {
		return c, err
	}
	if c.Name, err = o.str("name"); err != nil {
		return c, err
	}
	if c.IP, err = o.str("ip"); err != nil {
		return c, err
	}
	if c.Port, err = o.integer("port"); err != nil {
		return c, err
	}
	ct, err := o.enum("connectionType", string(LocalOnly), string(CloudCapable), string(Cloud))
	c.ConnectionType = ConnectionType(ct)
	return c, err
}

func decodePage(raw json.RawMessage) (Page, error) {
	var p Page
	o, err := decodeObject(raw)
	if err != nil {
		return p, err
	}
	if p.Name, err = o.str("name"); err != nil {
		return p, err
	}
	if p.Icon, err = o.str("icon"); err != nil {
		return p, err
	}
	if p.IsFullscreenWidget, err = o.boolean("isFullscreenWidget"); err != nil {
		return p, err
	}
	widgets, err := o.array("widgets")
	if err != nil {
		return p, err
	}
	for _, w := range widgets {
		item, err := decodeItem(w)
		if err != nil {
			p.Skipped = append(p.Skipped, SkippedWidget{Type: probeType(w), Reason: err.Error()})
			continue
		}
		p.Items = append(p.Items, item)
	}
	return p, nil
}

// probeType returns a skipped widget's type, or nil when it has no string one.
func probeType(raw json.RawMessage) *string {
	o, err := decodeObject(raw)
	if err != nil {
		return nil
	}
	t, ok, err := o.optStr("type")
	if !ok || err != nil {
		return nil
	}
	return &t
}

func decodeItem(raw json.RawMessage) (PageItem, error) {
	o, err := decodeObject(raw)
	if err != nil {
		return PageItem{}, err
	}
	t, err := o.str("type")
	if err != nil {
		return PageItem{}, err
	}
	if t == "HEADER" {
		h, err := decodeHeader(o)
		if err != nil {
			return PageItem{}, err
		}
		return PageItem{Header: h}, nil
	}
	decode, ok := widgetDecoders[t]
	if !ok {
		return PageItem{}, errField{"type", fmt.Sprintf("unknown widget type %q", t)}
	}
	w, err := decode(o)
	if err != nil {
		return PageItem{}, err
	}
	return PageItem{Widget: w}, nil
}

// decodeHeader reads a HEADER: its id and the first component's label.
func decodeHeader(o object) (*SectionHeader, error) {
	id, err := o.str("id")
	if err != nil {
		return nil, err
	}
	comps, err := o.array("components")
	if err != nil {
		return nil, err
	}
	if len(comps) == 0 {
		return nil, errField{"components", "a header needs a label"}
	}
	c, err := decodeObject(comps[0])
	if err != nil {
		return nil, errField{"components[0]", "not an object"}
	}
	label, err := c.str("label")
	if err != nil {
		return nil, errField{"components[0]", err.Error()}
	}
	return &SectionHeader{ID: id, Label: label}, nil
}
