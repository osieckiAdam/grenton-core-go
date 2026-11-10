package provisioning

import (
	"strconv"
	"strings"
)

// The editor-to-interface conversion of spec §4.5.6. It works on the
// generic JSON of internal/strictjson and builds data.json-shaped JSON,
// which then goes through the ordinary parsing of §3.6.

// errMissing marks a widget without a field its conversion needs: it is
// passed on as a stub of id and type, which §3.6 then skips.
type errMissing struct{}

func (errMissing) Error() string { return "a field the conversion needs is missing" }

// copyPresent copies the keys that are present, null included, unchanged.
func copyPresent(out, in map[string]any, keys ...string) {
	for _, k := range keys {
		if v, has := in[k]; has {
			out[k] = v
		}
	}
}

// need returns a required field: present and not null.
func need(in map[string]any, key string) (any, error) {
	v, has := in[key]
	if !has || v == nil {
		return nil, errMissing{}
	}
	return v, nil
}

// needObject returns a required field that must be an object.
func needObject(in map[string]any, key string) (map[string]any, error) {
	v, err := need(in, key)
	if err != nil {
		return nil, err
	}
	o, isObj := v.(map[string]any)
	if !isObj {
		return nil, errMissing{}
	}
	return o, nil
}

// exists: an object whose objectName is a non-empty string.
func exists(v any) (map[string]any, bool) {
	o, isObj := v.(map[string]any)
	if !isObj {
		return nil, false
	}
	name, isStr := o["objectName"].(string)
	return o, isStr && name != ""
}

// convertRef passes a state reference through; nil when it does not exist.
func convertRef(v any) map[string]any {
	r, ok := exists(v)
	if !ok {
		return nil
	}
	out := map[string]any{}
	copyPresent(out, r, "cluId", "objectName", "index", "callType")
	return out
}

func isAction(v any) bool {
	o, isObj := v.(map[string]any)
	if !isObj {
		return false
	}
	_, has := o["valueFormattedForDto"]
	return has
}

// convertAction passes an action through; nil when it does not exist.
func convertAction(v any) map[string]any {
	a, ok := exists(v)
	if !ok {
		return nil
	}
	out := map[string]any{}
	copyPresent(out, a, "cluId", "objectName", "value")
	if idx, has := a["index"]; has && idx != nil {
		out["index"] = idx
	}
	if t, has := a["type"]; has {
		out["event"] = t
	}
	if ct, has := a["callType"]; has {
		if ct == "PSEUDO_METHOD" {
			ct = "ATTRIBUTE"
		}
		out["callType"] = ct
	}
	return out
}

func withRef(out map[string]any, key string, v any) {
	if r := convertRef(v); r != nil {
		out[key] = r
	}
}

func actionList(in map[string]any, keys ...string) []any {
	out := []any{}
	for _, k := range keys {
		if a := convertAction(in[k]); a != nil {
			out = append(out, a)
		}
	}
	return out
}

// bistable is P: a bistable button.
func bistable(b map[string]any) map[string]any {
	out := map[string]any{"unit": "UNKNOWN"}
	copyPresent(out, b, "image", "label", "onIndication", "offIndication", "type", "rowId")
	withRef(out, "state", b["state"])
	out["actions"] = actionList(b, "actionOn", "actionOff")
	return out
}

// monostable is M: a scene button.
func monostable(m map[string]any) map[string]any {
	out := map[string]any{"unit": "UNKNOWN"}
	copyPresent(out, m, "label", "type", "rowId")
	out["actions"] = actionList(m, "actionClick")
	return out
}

// addBackground is B: a scene's background, when it is an object.
func addBackground(out, w map[string]any) {
	b, isObj := w["background"].(map[string]any)
	if !isObj {
		return
	}
	bg := map[string]any{}
	if c, has := b["color"]; has {
		if s, isStr := c.(string); isStr {
			c = strings.ToUpper(s)
		}
		bg["color"] = c
	}
	if img, has := b["image"]; has {
		bg["name"] = img
	}
	out["background"] = bg
}

func valueFields(c map[string]any) map[string]any {
	out := map[string]any{}
	copyPresent(out, c, "unit", "valueType", "precision", "icon", "label")
	obj := map[string]any{}
	withRef(obj, "value", c["value"])
	out["object"] = obj
	return out
}

func contactFields(c map[string]any) (map[string]any, error) {
	out := map[string]any{}
	copyPresent(out, c, "icon", "label", "onIndication", "offIndication", "reverseState")
	obj, err := needObject(c, "object")
	if err != nil {
		return nil, err
	}
	out["object"] = objectBlock(obj)
	return out, nil
}

func ledSlider(s map[string]any) map[string]any {
	out := map[string]any{}
	copyPresent(out, s, "unit", "label", "type", "rowId")
	rng := map[string]any{}
	copyPresent(rng, s, "min", "max")
	out["range"] = rng
	withRef(out, "state", s["state"])
	out["actions"] = actionList(s, "actionClick")
	return out
}

// objectBlock is O: actions under a name ending in Action, references under
// their own name; a field already so named wins over a bare-named action.
func objectBlock(obj map[string]any) map[string]any {
	out := map[string]any{}
	for name, v := range obj {
		if _, isObj := v.(map[string]any); !isObj {
			continue
		}
		if isAction(v) {
			if strings.HasSuffix(name, "Action") {
				if a := convertAction(v); a != nil {
					out[name] = a
				}
			}
		} else if r := convertRef(v); r != nil {
			out[name] = r
		}
	}
	for name, v := range obj {
		if !isAction(v) || strings.HasSuffix(name, "Action") {
			continue
		}
		target := name + "Action"
		if _, taken := obj[target]; taken {
			continue
		}
		if a := convertAction(v); a != nil {
			out[target] = a
		}
	}
	return out
}

// multisensorChannels: editor key → mobile key (the editor writes VOC as
// objectAirVco).
var multisensorChannels = [][2]string{
	{"objectAirCo2", "objectAirCo2"}, {"objectSound", "objectSound"}, {"objectAirVco", "objectAirVoc"},
	{"objectLight", "objectLight"}, {"objectPressure", "objectPressure"}, {"objectHumidity", "objectHumidity"},
	{"objectTemperature", "objectTemperature"},
}

// widgetID is dtoId when it is a non-empty string, else omp-{id} for a
// non-negative integer id, else "".
func widgetID(w map[string]any) string {
	if s, isStr := w["dtoId"].(string); isStr && s != "" {
		return s
	}
	if n, isInt := w["id"].(int64); isInt && n >= 0 {
		return "omp-" + strconv.FormatInt(n, 10)
	}
	return ""
}

// convertWidget converts one editor widget, or returns errMissing.
func convertWidget(w map[string]any) (map[string]any, error) {
	tv, err := need(w, "type")
	if err != nil {
		return nil, err
	}
	t, isStr := tv.(string)
	id := widgetID(w)
	if !isStr || id == "" {
		return nil, errMissing{}
	}
	out := map[string]any{"id": id, "type": t}
	obj := func(key string) (map[string]any, error) { return needObject(w, key) }
	switch t {
	case "HEADER":
		text, err := obj("text")
		if err != nil {
			return nil, err
		}
		c := map[string]any{"unit": "UNKNOWN"}
		copyPresent(c, text, "label", "type", "rowId")
		out["components"] = []any{c}
	case "ON_OFF":
		b, err := obj("buttonBistable")
		if err != nil {
			return nil, err
		}
		out["components"] = []any{bistable(b)}
	case "ON_OFF_DOUBLE":
		b1, err := obj("buttonBistableFirst")
		if err != nil {
			return nil, err
		}
		b2, err := obj("buttonBistableSecond")
		if err != nil {
			return nil, err
		}
		out["components"] = []any{bistable(b1), bistable(b2)}
	case "SCENE":
		m, err := obj("buttonMonostable")
		if err != nil {
			return nil, err
		}
		out["components"] = []any{monostable(m)}
		addBackground(out, w)
	case "SCENE_DOUBLE":
		m1, err := obj("buttonMonostableFirst")
		if err != nil {
			return nil, err
		}
		m2, err := obj("buttonMonostableSecond")
		if err != nil {
			return nil, err
		}
		out["components"] = []any{monostable(m1), monostable(m2)}
		addBackground(out, w)
	case "VALUE_V2":
		for k, v := range valueFields(w) {
			out[k] = v
		}
	case "VALUE_DOUBLE":
		l, err := obj("componentLeft")
		if err != nil {
			return nil, err
		}
		r, err := obj("componentRight")
		if err != nil {
			return nil, err
		}
		out["componentLeft"], out["componentRight"] = valueFields(l), valueFields(r)
	case "DIMMER_V2", "SLIDER", "THERMOSTAT_V2", "ROLLER_SHUTTER_V3", "EVENT_SCHEDULER", "TV_REMOTE_CONTROL":
		switch t {
		case "DIMMER_V2":
			copyPresent(out, w, "min", "max", "precision")
		case "SLIDER":
			copyPresent(out, w, "unit", "min", "max", "precision")
		case "TV_REMOTE_CONTROL":
			copyPresent(out, w, "vodLabel")
		case "THERMOSTAT_V2":
			if n, has := w["noOfFanSpeeds"]; has {
				out["no_of_fan_speeds"] = n
			}
		}
		copyPresent(out, w, "icon", "label")
		o, err := obj("object")
		if err != nil {
			return nil, err
		}
		out["object"] = objectBlock(o)
	case "CAMERA":
		c, err := obj("cameraStream")
		if err != nil {
			return nil, err
		}
		comp := map[string]any{"unit": "UNKNOWN"}
		copyPresent(comp, c, "image", "label", "type", "rowId")
		if u, has := c["url"]; has {
			comp["value"] = u
		}
		out["components"] = []any{comp}
	case "LED":
		b, err := obj("buttonBistable")
		if err != nil {
			return nil, err
		}
		comps := []any{bistable(b)}
		for _, k := range []string{"sliderHue", "sliderSaturation", "sliderBrightness"} {
			s, err := obj(k)
			if err != nil {
				return nil, err
			}
			comps = append(comps, ledSlider(s))
		}
		out["components"] = comps
	case "CONTACT_SENSOR":
		f, err := contactFields(w)
		if err != nil {
			return nil, err
		}
		for k, v := range f {
			out[k] = v
		}
	case "CONTACT_SENSOR_DOUBLE":
		l, err := obj("componentLeft")
		if err != nil {
			return nil, err
		}
		r, err := obj("componentRight")
		if err != nil {
			return nil, err
		}
		if out["componentLeft"], err = contactFields(l); err != nil {
			return nil, err
		}
		if out["componentRight"], err = contactFields(r); err != nil {
			return nil, err
		}
	case "MULTISENSOR":
		copyPresent(out, w, "icon", "label", "labelVisible")
		for _, ch := range multisensorChannels {
			c, isObj := w[ch[0]].(map[string]any)
			if !isObj {
				continue
			}
			refs := map[string]any{}
			for name, v := range c {
				withRef(refs, name, v)
			}
			out[ch[1]] = refs
		}
	case "ROLLER_SHUTTER":
		b, err := obj("buttonBistableClick")
		if err != nil {
			return nil, err
		}
		c := map[string]any{"unit": "UNKNOWN"}
		copyPresent(c, b, "image", "label", "indication", "rowId")
		withRef(c, "state", b["state"])
		if bt, has := b["type"]; has {
			if bt == "BUTTON_BISTABLE_CLICK" {
				bt = "BUTTON_BISTABLE"
			}
			c["type"] = bt
		}
		c["actions"] = actionList(b, "actionClick")
		out["components"] = []any{c}
	default:
		return nil, errMissing{}
	}
	return out, nil
}

// convertWidgetOrStub converts a widget; one the conversion cannot handle
// passes with only its id and type (§4.5.6).
func convertWidgetOrStub(v any) map[string]any {
	w, isObj := v.(map[string]any)
	if !isObj {
		return map[string]any{}
	}
	if out, err := convertWidget(w); err == nil {
		return out
	}
	stub := map[string]any{}
	if id := widgetID(w); id != "" {
		stub["id"] = id
	}
	if t, isStr := w["type"].(string); isStr {
		stub["type"] = t
	}
	return stub
}

// convertPage builds {name, icon, isFullscreenWidget: false, widgets}.
func convertPage(v any) (map[string]any, error) {
	p, isObj := v.(map[string]any)
	label, okLabel := p["label"].(string)
	icon, okIcon := p["icon"].(string)
	widgets, okWidgets := p["widgets"].([]any)
	if !isObj || !okLabel || !okIcon || !okWidgets {
		return nil, invalidData("a page without a string label, a string icon and a widget list")
	}
	out := make([]any, len(widgets))
	for i, w := range widgets {
		out[i] = convertWidgetOrStub(w)
	}
	return map[string]any{"name": label, "icon": icon, "isFullscreenWidget": false, "widgets": out}, nil
}

// usedClus collects the cluId of every reference and action of the
// converted pages (§4.5.4): component states and actions, and the members
// of objects and multisensor channels; stubs carry none.
func usedClus(pages []any) map[string]bool {
	used := map[string]bool{}
	add := func(v any) {
		if r, isObj := v.(map[string]any); isObj {
			if id, isStr := r["cluId"].(string); isStr {
				used[id] = true
			}
		}
	}
	members := func(v any) {
		if o, isObj := v.(map[string]any); isObj {
			for _, m := range o {
				add(m)
			}
		}
	}
	for _, p := range pages {
		for _, wv := range p.(map[string]any)["widgets"].([]any) {
			w := wv.(map[string]any)
			if comps, isList := w["components"].([]any); isList {
				for _, cv := range comps {
					c, _ := cv.(map[string]any)
					add(c["state"])
					if actions, isList := c["actions"].([]any); isList {
						for _, a := range actions {
							add(a)
						}
					}
				}
			}
			for _, side := range []string{"componentLeft", "componentRight"} {
				if s, isObj := w[side].(map[string]any); isObj {
					members(s["object"])
				}
			}
			members(w["object"])
			for _, ch := range multisensorChannels {
				members(w[ch[1]])
			}
		}
	}
	return used
}
