package devices

import "github.com/osieckiAdam/grenton-core-go/model"

// ObjectKind is what a physical object is (spec §6.6).
type ObjectKind string

// The physical object kinds.
const (
	ObjectFeature  ObjectKind = "object"
	ObjectVariable ObjectKind = "variable"
	ObjectScript   ObjectKind = "script"
)

// ObjectKey identifies what an entity controls on a CLU: a feature (or
// method) Index of an object, a variable, or a script. Index is "" for a
// variable and a script.
type ObjectKey struct {
	CluID string
	Kind  ObjectKind
	Name  string
	Index string
}

// PhysicalObject is one CLU object and the widgets that show it.
type PhysicalObject struct {
	Key     ObjectKey
	Widgets []ObjectWidget
}

// ObjectWidget is one device's share of a physical object.
type ObjectWidget struct {
	DeviceID   string
	WidgetType string
	Name       string
	Page       PageRef
	EntityIDs  []string
}

// ObjectKeyOf returns the object an entity belongs to, by its primary
// reference, and false for a camera and a TV remote, which are not grouped.
func ObjectKeyOf(d Descriptor) (ObjectKey, bool) {
	switch d := d.(type) {
	case *SwitchDescriptor:
		return refKey(d.State), true
	case *DimmableLightDescriptor:
		return refKey(d.State), true
	case *HsbLightDescriptor:
		return refKey(d.PowerState), true
	case *ContactSensorDescriptor:
		return refKey(d.State), true
	case *SensorDescriptor:
		return refKey(d.State), true
	case *NumberSettingDescriptor:
		return refKey(d.State), true
	case *StateSensorDescriptor:
		return refKey(d.State), true
	case *CoverDescriptor:
		return refKey(d.Position), true
	case *ThermostatDescriptor:
		return refKey(d.CurrentTemperature), true
	case *EventScheduleDescriptor:
		return refKey(d.State), true
	case *TriggerDescriptor:
		return actionKey(d.Action), true
	case *SceneTriggerDescriptor:
		return actionKey(d.Action), true
	}
	return ObjectKey{}, false
}

func refKey(r model.ValueRef) ObjectKey {
	if r.CallType == model.ValueVariable {
		return ObjectKey{CluID: r.CluID, Kind: ObjectVariable, Name: r.Index}
	}
	return ObjectKey{CluID: r.CluID, Kind: ObjectFeature, Name: r.ObjectName, Index: r.Index}
}

func actionKey(a model.ActionRef) ObjectKey {
	switch a.CallType {
	case model.ActionVariable:
		return ObjectKey{CluID: a.CluID, Kind: ObjectVariable, Name: a.Index}
	case model.ActionScript:
		return ObjectKey{CluID: a.CluID, Kind: ObjectScript, Name: a.ObjectName}
	}
	return ObjectKey{CluID: a.CluID, Kind: ObjectFeature, Name: a.ObjectName, Index: a.Index}
}

// physicalObjects groups entities by object, in order of first use.
func physicalObjects(devices []Device) []PhysicalObject {
	var objects []PhysicalObject
	index := map[ObjectKey]int{}
	for _, d := range devices {
		var keys []ObjectKey
		ids := map[ObjectKey][]string{}
		for _, e := range d.Entities {
			k, grouped := ObjectKeyOf(e)
			if !grouped {
				continue
			}
			if _, seen := ids[k]; !seen {
				keys = append(keys, k)
			}
			ids[k] = append(ids[k], e.Info().ID)
		}
		for _, k := range keys {
			i, known := index[k]
			if !known {
				i = len(objects)
				index[k] = i
				objects = append(objects, PhysicalObject{Key: k})
			}
			objects[i].Widgets = append(objects[i].Widgets, ObjectWidget{DeviceID: d.ID, WidgetType: d.WidgetType,
				Name: d.Name, Page: d.Page, EntityIDs: ids[k]})
		}
	}
	return objects
}
