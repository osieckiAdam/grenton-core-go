package conformance_test

import (
	"os"
	"path/filepath"

	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/model"
)

func init() {
	registry["mapping.interface"] = mappingInterface
}

func readFixture(s *suite, input map[string]any) ([]byte, error) {
	rel, err := str(input, "fixture")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, runnerErr("fixture: %v", err)
	}
	return data, nil
}

func mappingInterface(s *suite, input map[string]any) (any, error) {
	data, err := readFixture(s, input)
	if err != nil {
		return nil, err
	}
	in, err := model.DecodeInterface(data)
	if err != nil {
		return reject, nil // the interface does not decode as a whole
	}
	return ok(mappingJSON(in)), nil
}

// mappingJSON is the projection of conformance.md §4a.
func mappingJSON(in *model.Interface) map[string]any {
	m := devices.Map(in)
	devs := make([]any, len(m.Devices))
	for i, d := range m.Devices {
		ents := make([]any, len(d.Entities))
		for j, e := range d.Entities {
			ents[j] = entityJSON(e)
		}
		var section any
		if d.Section != nil {
			section = map[string]any{"id": d.Section.ID, "title": d.Section.Title}
		}
		devs[i] = map[string]any{"id": d.ID, "widgetType": d.WidgetType, "kind": string(d.Kind), "name": d.Name,
			"icon": optString(d.Icon), "page": pageJSON(d.Page), "section": section, "entities": ents}
	}
	objects := make([]any, len(m.PhysicalObjects))
	for i, o := range m.PhysicalObjects {
		widgets := make([]any, len(o.Widgets))
		for j, w := range o.Widgets {
			ids := make([]any, len(w.EntityIDs))
			for k, id := range w.EntityIDs {
				ids[k] = id
			}
			widgets[j] = map[string]any{"deviceId": w.DeviceID, "widgetType": w.WidgetType, "name": w.Name,
				"page": pageJSON(w.Page), "entityIds": ids}
		}
		var index any
		if o.Key.Kind == devices.ObjectFeature {
			index = o.Key.Index
		}
		objects[i] = map[string]any{"key": map[string]any{"cluId": o.Key.CluID, "kind": string(o.Key.Kind),
			"name": o.Key.Name, "index": index}, "widgets": widgets}
	}
	skipped := make([]any, len(m.Skipped))
	for i, page := range m.Skipped {
		types := make([]any, len(page))
		for j, t := range page {
			types[j] = optString(t)
		}
		skipped[i] = types
	}
	return map[string]any{"devices": devs, "physicalObjects": objects, "skippedWidgets": skipped}
}

func pageJSON(p devices.PageRef) map[string]any {
	return map[string]any{"index": p.Index, "name": p.Name, "icon": p.Icon}
}

func optString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func refJSON(r model.ValueRef) map[string]any {
	if r.CallType == model.ValueVariable {
		return map[string]any{"callType": "VARIABLE", "cluId": r.CluID, "index": r.Index}
	}
	return map[string]any{"callType": "ATTRIBUTE", "cluId": r.CluID, "objectName": r.ObjectName, "index": r.Index}
}

func optRefJSON(r *model.ValueRef) any {
	if r == nil {
		return nil
	}
	return refJSON(*r)
}

func actionJSON(a model.ActionRef) map[string]any {
	o := map[string]any{"callType": string(a.CallType), "cluId": a.CluID, "objectName": a.ObjectName,
		"event": string(a.Event), "value": a.Value}
	if a.CallType != model.ActionScript {
		o["index"] = a.Index
	}
	return o
}

func optActionJSON(a *model.ActionRef) any {
	if a == nil {
		return nil
	}
	return actionJSON(*a)
}

func entityJSON(d devices.Descriptor) map[string]any {
	i := d.Info()
	o := map[string]any{"kind": string(i.Kind), "id": i.ID, "name": i.Name, "icon": optString(i.Icon)}
	switch d := d.(type) {
	case *devices.SwitchDescriptor:
		o["state"], o["onAction"], o["offAction"] = refJSON(d.State), actionJSON(d.OnAction), actionJSON(d.OffAction)
		o["onIndication"], o["offIndication"] = d.OnIndication, d.OffIndication
	case *devices.DimmableLightDescriptor:
		o["state"], o["min"], o["max"], o["precision"] = refJSON(d.State), d.Min, d.Max, d.Precision
		o["onAction"], o["offAction"], o["setValueAction"] = actionJSON(d.OnAction), actionJSON(d.OffAction), actionJSON(d.SetValueAction)
	case *devices.HsbLightDescriptor:
		channel := func(c devices.HsbChannel) map[string]any {
			return map[string]any{"state": refJSON(c.State), "setAction": actionJSON(c.SetAction),
				"range": map[string]any{"min": c.Range.Min, "max": c.Range.Max}}
		}
		o["powerState"], o["onAction"], o["offAction"] = refJSON(d.PowerState), actionJSON(d.OnAction), actionJSON(d.OffAction)
		o["hue"], o["saturation"], o["brightness"] = channel(d.Hue), channel(d.Saturation), channel(d.Brightness)
	case *devices.ContactSensorDescriptor:
		o["state"], o["reversed"] = refJSON(d.State), d.Reversed
		o["onIndication"], o["offIndication"] = d.OnIndication, d.OffIndication
	case *devices.SensorDescriptor:
		o["state"], o["sensorKind"], o["unit"] = refJSON(d.State), string(d.SensorKind), optString(d.Unit)
		o["grentonUnit"], o["valueType"], o["precision"] = nil, nil, nil
		if d.GrentonUnit != nil {
			o["grentonUnit"] = string(*d.GrentonUnit)
		}
		if d.ValueType != nil {
			o["valueType"] = string(*d.ValueType)
		}
		if d.Precision != nil {
			o["precision"] = *d.Precision
		}
	case *devices.NumberSettingDescriptor:
		o["state"], o["setValueAction"] = refJSON(d.State), actionJSON(d.SetValueAction)
		o["min"], o["max"], o["precision"] = d.Min, d.Max, d.Precision
	case *devices.CoverDescriptor:
		o["motionState"], o["position"], o["setPositionAction"] = refJSON(d.MotionState), refJSON(d.Position), actionJSON(d.SetPositionAction)
		o["tilt"], o["stopAction"] = nil, optActionJSON(d.StopAction)
		if d.Tilt != nil {
			o["tilt"] = map[string]any{"position": refJSON(d.Tilt.Position), "setAction": actionJSON(d.Tilt.SetAction)}
		}
	case *devices.StateSensorDescriptor:
		o["state"] = refJSON(d.State)
	case *devices.TriggerDescriptor:
		o["action"] = actionJSON(d.Action)
	case *devices.SceneTriggerDescriptor:
		o["action"] = actionJSON(d.Action)
	case *devices.CameraDescriptor:
		o["streamUrl"] = d.StreamURL
	case *devices.ThermostatDescriptor:
		control := func(c *devices.ThermostatControl) any {
			if c == nil {
				return nil
			}
			return map[string]any{"state": refJSON(c.State), "setAction": actionJSON(c.SetAction)}
		}
		o["currentTemperature"], o["targetTemperature"] = refJSON(d.CurrentTemperature), refJSON(d.TargetTemperature)
		o["setTargetTemperatureAction"] = actionJSON(d.SetTargetTemperatureAction)
		o["minTemperature"], o["maxTemperature"], o["output"] = optRefJSON(d.MinTemperature), optRefJSON(d.MaxTemperature), optRefJSON(d.Output)
		o["mode"], o["power"] = control(d.Mode), control(d.Power)
	case *devices.EventScheduleDescriptor:
		o["state"], o["ruleList"] = refJSON(d.State), refJSON(d.RuleList)
		o["addRuleAction"], o["deleteRuleAction"] = actionJSON(d.AddRuleAction), actionJSON(d.DeleteRuleAction)
		o["ruleCount"], o["ruleSpace"] = optRefJSON(d.RuleCount), optRefJSON(d.RuleSpace)
		o["nextRule"], o["currentRule"] = optRefJSON(d.NextRule), optRefJSON(d.CurrentRule)
		o["startAction"], o["stopAction"] = optActionJSON(d.StartAction), optActionJSON(d.StopAction)
		o["enableRuleAction"], o["disableRuleAction"] = optActionJSON(d.EnableRuleAction), optActionJSON(d.DisableRuleAction)
	case *devices.TvRemoteDescriptor:
		buttons := map[string]any{}
		for k, a := range d.Buttons {
			buttons[k] = actionJSON(a)
		}
		o["buttons"] = buttons
	}
	return o
}
