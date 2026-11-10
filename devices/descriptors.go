// Package devices is the canonical device model of Grenton Core (spec §6):
// widgets mapped to devices and entities with normalized state and intents,
// the grouping by physical object, and the System that runs every CLU of an
// interface.
//
// Mapping has two stages. [Map] is a pure function from an interface to
// [Device] descriptors, usable without a network. [Bind] turns descriptors
// into live entities over a state store and a command sink.
package devices

import (
	"fmt"
	"strings"

	"github.com/osieckiAdam/grenton-core-go/model"
)

// DeviceKind is what a device is, fixed by its widget type (spec §6.1).
type DeviceKind string

// The device kinds.
const (
	KindSwitch        DeviceKind = "switch"
	KindDimmableLight DeviceKind = "dimmableLight"
	KindHsbLight      DeviceKind = "hsbLight"
	KindContactSensor DeviceKind = "contactSensor"
	KindSensor        DeviceKind = "sensor"
	KindMultisensor   DeviceKind = "multisensor"
	KindNumberSetting DeviceKind = "numberSetting"
	KindCover         DeviceKind = "cover"
	KindScene         DeviceKind = "scene"
	KindThermostat    DeviceKind = "thermostat"
	KindCamera        DeviceKind = "camera"
	KindEventSchedule DeviceKind = "eventSchedule"
	KindTvRemote      DeviceKind = "tvRemote"
)

// EntityKind is the kind of a canonical entity.
type EntityKind string

// The entity kinds.
const (
	EntitySwitch        EntityKind = "switch"
	EntityDimmableLight EntityKind = "dimmableLight"
	EntityHsbLight      EntityKind = "hsbLight"
	EntityContactSensor EntityKind = "contactSensor"
	EntitySensor        EntityKind = "sensor"
	EntityNumberSetting EntityKind = "numberSetting"
	EntityCover         EntityKind = "cover"
	EntityStateSensor   EntityKind = "stateSensor"
	EntityTrigger       EntityKind = "trigger"
	EntitySceneTrigger  EntityKind = "sceneTrigger"
	EntityCamera        EntityKind = "camera"
	EntityThermostat    EntityKind = "thermostat"
	EntityEventSchedule EntityKind = "eventSchedule"
	EntityTvRemote      EntityKind = "tvRemote"
)

// PageRef is the page a device lies on.
type PageRef struct {
	Index int
	Name  string
	Icon  string
}

// Section is the HEADER a device appears under (spec §6.1).
type Section struct {
	ID    string
	Title string
}

// Device is a device descriptor: one widget (spec §6.1).
type Device struct {
	ID         string
	WidgetType string
	Kind       DeviceKind
	Name       string
	// Icon is the widget's Object Manager icon, nil for types without one.
	Icon     *string
	Page     PageRef
	Section  *Section
	Entities []Descriptor
}

// Descriptor describes one canonical entity: one of the *Descriptor types
// of this package.
type Descriptor interface {
	// Info returns the fields every entity has.
	Info() EntityInfo
}

// EntityInfo is what every entity has. ID is {widgetId}_{suffix}, a stable
// contract (spec §6.1); Icon is its component's icon, else the widget's.
type EntityInfo struct {
	Kind EntityKind
	ID   string
	Name string
	Icon *string
}

// Info returns the info itself, so that descriptors embedding it are
// Descriptors.
func (i EntityInfo) Info() EntityInfo { return i }

// SwitchDescriptor is a switch (spec §6.3.1).
type SwitchDescriptor struct {
	EntityInfo
	State                       model.ValueRef
	OnAction, OffAction         model.ActionRef
	OnIndication, OffIndication string
}

// DimmableLightDescriptor is a dimmer (spec §6.3.2).
type DimmableLightDescriptor struct {
	EntityInfo
	State                               model.ValueRef
	Min, Max                            float64
	Precision                           int64
	OnAction, OffAction, SetValueAction model.ActionRef
}

// HsbChannel is one slider of an HSB light.
type HsbChannel struct {
	State     model.ValueRef
	SetAction model.ActionRef
	Range     model.IntRange
}

// HsbLightDescriptor is an LED (spec §6.3.3).
type HsbLightDescriptor struct {
	EntityInfo
	PowerState                  model.ValueRef
	OnAction, OffAction         model.ActionRef
	Hue, Saturation, Brightness HsbChannel
}

// ContactSensorDescriptor is a contact sensor (spec §6.3.4).
type ContactSensorDescriptor struct {
	EntityInfo
	State                       model.ValueRef
	Reversed                    bool
	OnIndication, OffIndication string
}

// SensorKind is the canonical kind of a sensor (spec §6.3.6).
type SensorKind string

// Sensor kinds: generic for value widgets, the rest for multisensor channels.
const (
	SensorGeneric       SensorKind = "generic"
	SensorCO2           SensorKind = "co2"
	SensorSoundPressure SensorKind = "soundPressure"
	SensorVOC           SensorKind = "voc"
	SensorIlluminance   SensorKind = "illuminance"
	SensorPressure      SensorKind = "pressure"
	SensorHumidity      SensorKind = "humidity"
	SensorTemperature   SensorKind = "temperature"
)

// SensorDescriptor is a sensor (spec §6.3.5–§6.3.6). Unit is the canonical
// unit of a multisensor channel; GrentonUnit, ValueType and Precision are a
// value widget's metadata.
type SensorDescriptor struct {
	EntityInfo
	State       model.ValueRef
	SensorKind  SensorKind
	Unit        *string
	GrentonUnit *model.Unit
	ValueType   *model.ValueType
	Precision   *int64
}

// NumberSettingDescriptor is a slider (spec §6.3.7).
type NumberSettingDescriptor struct {
	EntityInfo
	State          model.ValueRef
	SetValueAction model.ActionRef
	Min, Max       float64
	Precision      int64
}

// CoverTilt is a cover's slats.
type CoverTilt struct {
	Position  model.ValueRef
	SetAction model.ActionRef
}

// CoverDescriptor is a ROLLER_SHUTTER_V3 (spec §6.3.8); its id has no suffix.
type CoverDescriptor struct {
	EntityInfo
	MotionState       model.ValueRef
	Position          model.ValueRef
	SetPositionAction model.ActionRef
	Tilt              *CoverTilt
	StopAction        *model.ActionRef
}

// StateSensorDescriptor is a legacy roller shutter's state (spec §6.3.9).
type StateSensorDescriptor struct {
	EntityInfo
	State model.ValueRef
}

// TriggerDescriptor is a button: one action (spec §6.3.9).
type TriggerDescriptor struct {
	EntityInfo
	Action model.ActionRef
}

// SceneTriggerDescriptor is a scene's script (spec §6.3.10).
type SceneTriggerDescriptor struct {
	EntityInfo
	Action model.ActionRef
}

// CameraDescriptor is a camera stream (spec §6.3.11).
type CameraDescriptor struct {
	EntityInfo
	StreamURL string
}

// ThermostatControl is a thermostat feature with its setter.
type ThermostatControl struct {
	State     model.ValueRef
	SetAction model.ActionRef
}

// ThermostatDescriptor is a THERMOSTAT_V2 (spec §6.3.12).
type ThermostatDescriptor struct {
	EntityInfo
	CurrentTemperature, TargetTemperature model.ValueRef
	SetTargetTemperatureAction            model.ActionRef
	MinTemperature, MaxTemperature        *model.ValueRef
	Output                                *model.ValueRef
	Mode, Power                           *ThermostatControl
}

// EventScheduleDescriptor is an EVENT_SCHEDULER (spec §6.3.13).
type EventScheduleDescriptor struct {
	EntityInfo
	State, RuleList                                              model.ValueRef
	AddRuleAction, DeleteRuleAction                              model.ActionRef
	RuleCount, RuleSpace, NextRule, CurrentRule                  *model.ValueRef
	StartAction, StopAction, EnableRuleAction, DisableRuleAction *model.ActionRef
}

// TvRemoteDescriptor is a TV remote (spec §6.3.14): its buttons by name.
type TvRemoteDescriptor struct {
	EntityInfo
	Buttons map[string]model.ActionRef
}

// Mapping is the result of [Map].
type Mapping struct {
	Devices         []Device
	PhysicalObjects []PhysicalObject
	// Skipped lists, per page, the type of every widget that did not parse
	// (nil when it had none).
	Skipped [][]*string
}

// Map maps an interface to device descriptors (spec §6.1 stage 1), in page
// and widget order, with the physical objects of §6.6.
func Map(in *model.Interface) Mapping {
	var m Mapping
	for pageIndex, page := range in.Pages {
		var section *Section
		for _, item := range page.Items {
			if item.Header != nil {
				section = &Section{ID: item.Header.ID, Title: item.Header.Label}
				continue
			}
			w := item.Widget
			d := Device{ID: w.WidgetID(), WidgetType: w.WidgetType(), Kind: deviceKind(w), Name: deviceName(w),
				Icon: widgetIcon(w), Page: PageRef{Index: pageIndex, Name: page.Name, Icon: page.Icon}, Section: section}
			d.Entities = entities(w)
			m.Devices = append(m.Devices, d)
		}
		skipped := make([]*string, 0, len(page.Skipped))
		for _, s := range page.Skipped {
			skipped = append(skipped, s.Type)
		}
		m.Skipped = append(m.Skipped, skipped)
	}
	m.PhysicalObjects = physicalObjects(m.Devices)
	return m
}

func deviceKind(w model.Widget) DeviceKind {
	switch w.(type) {
	case *model.OnOffWidget:
		return KindSwitch
	case *model.DimmerV2Widget:
		return KindDimmableLight
	case *model.LedWidget:
		return KindHsbLight
	case *model.ContactSensorWidget, *model.ContactSensorDoubleWidget:
		return KindContactSensor
	case *model.ValueV2Widget, *model.ValueDoubleWidget:
		return KindSensor
	case *model.MultisensorWidget:
		return KindMultisensor
	case *model.SliderWidget:
		return KindNumberSetting
	case *model.RollerShutterWidget, *model.RollerShutterV3Widget:
		return KindCover
	case *model.SceneWidget:
		return KindScene
	case *model.ThermostatV2Widget:
		return KindThermostat
	case *model.CameraWidget:
		return KindCamera
	case *model.EventSchedulerWidget:
		return KindEventSchedule
	case *model.TvRemoteWidget:
		return KindTvRemote
	}
	panic(fmt.Sprintf("devices: unmapped widget %T", w))
}

func joinLabels(labels ...string) string {
	var parts []string
	for _, l := range labels {
		if l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " / ")
}

// deviceName is the widget's label, or its components' labels joined.
func deviceName(w model.Widget) string {
	switch w := w.(type) {
	case *model.ValueV2Widget:
		return w.Label
	case *model.DimmerV2Widget:
		return w.Label
	case *model.ContactSensorWidget:
		return w.Label
	case *model.SliderWidget:
		return w.Label
	case *model.MultisensorWidget:
		return w.Label
	case *model.RollerShutterV3Widget:
		return w.Label
	case *model.ThermostatV2Widget:
		return w.Label
	case *model.EventSchedulerWidget:
		return w.Label
	case *model.TvRemoteWidget:
		return w.Label
	case *model.ValueDoubleWidget:
		return joinLabels(w.Left.Label, w.Right.Label)
	case *model.ContactSensorDoubleWidget:
		return joinLabels(w.Left.Label, w.Right.Label)
	case *model.LedWidget:
		for _, c := range w.Components {
			if c.Button != nil {
				return c.Button.Label
			}
		}
		return ""
	case *model.OnOffWidget:
		labels := make([]string, len(w.Components))
		for i, c := range w.Components {
			labels[i] = c.Label
		}
		return joinLabels(labels...)
	case *model.RollerShutterWidget:
		labels := make([]string, len(w.Components))
		for i, c := range w.Components {
			labels[i] = c.Label
		}
		return joinLabels(labels...)
	case *model.SceneWidget:
		labels := make([]string, len(w.Components))
		for i, c := range w.Components {
			labels[i] = c.Label
		}
		return joinLabels(labels...)
	case *model.CameraWidget:
		labels := make([]string, len(w.Components))
		for i, c := range w.Components {
			labels[i] = c.Label
		}
		return joinLabels(labels...)
	}
	return ""
}

func ptr[T any](v T) *T { return &v }

// widgetIcon is the widget's own icon, for the types that have one.
func widgetIcon(w model.Widget) *string {
	switch w := w.(type) {
	case *model.ValueV2Widget:
		return ptr(w.Icon)
	case *model.DimmerV2Widget:
		return ptr(w.Icon)
	case *model.ContactSensorWidget:
		return ptr(w.Icon)
	case *model.SliderWidget:
		return ptr(w.Icon)
	case *model.MultisensorWidget:
		return ptr(w.Icon)
	case *model.RollerShutterV3Widget:
		return ptr(w.Icon)
	case *model.ThermostatV2Widget:
		return w.Icon
	case *model.EventSchedulerWidget:
		return ptr(w.Icon)
	case *model.TvRemoteWidget:
		return ptr(w.Icon)
	}
	return nil
}

// componentIcon is an entity's icon: its component's, else the widget's.
func componentIcon(w model.Widget, component *string) *string {
	if component != nil {
		return component
	}
	return widgetIcon(w)
}

func info(kind EntityKind, id, name string, icon *string) EntityInfo {
	return EntityInfo{Kind: kind, ID: id, Name: name, Icon: icon}
}

// firstAction is the first action with the event, or nil.
func firstAction(actions []model.ActionRef, event model.ActionEvent) *model.ActionRef {
	for i := range actions {
		if actions[i].Event == event {
			return &actions[i]
		}
	}
	return nil
}

// multisensorChannels are the channels of spec §6.3.6, in table order.
var multisensorChannels = []struct {
	suffix string
	kind   SensorKind
	unit   string
	pick   func(*model.MultisensorWidget) model.MultisensorChannel
}{
	{"air_co2", SensorCO2, "ppm", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.AirCo2 }},
	{"sound", SensorSoundPressure, "dB", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.Sound }},
	{"air_voc", SensorVOC, "ppb", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.AirVoc }},
	{"light", SensorIlluminance, "lx", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.Light }},
	{"pressure", SensorPressure, "hPa", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.Pressure }},
	{"humidity", SensorHumidity, "%", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.Humidity }},
	{"temperature", SensorTemperature, "°C", func(w *model.MultisensorWidget) model.MultisensorChannel { return w.Temperature }},
}

// entities maps a widget to its entities, in the order of spec §6.3.
func entities(w model.Widget) []Descriptor {
	var out []Descriptor
	add := func(d Descriptor) { out = append(out, d) }
	switch w := w.(type) {
	case *model.ValueV2Widget:
		add(&SensorDescriptor{EntityInfo: info(EntitySensor, w.ID+"_0", w.Label, widgetIcon(w)), State: w.Value,
			SensorKind: SensorGeneric, GrentonUnit: ptr(w.Unit), ValueType: ptr(w.ValueType), Precision: ptr(w.Precision)})
	case *model.ValueDoubleWidget:
		for n, side := range []model.ValueSide{w.Left, w.Right} {
			add(&SensorDescriptor{EntityInfo: info(EntitySensor, fmt.Sprintf("%s_%d", w.ID, n), side.Label, ptr(side.Icon)),
				State: side.Value, SensorKind: SensorGeneric, GrentonUnit: ptr(side.Unit), ValueType: ptr(side.ValueType),
				Precision: ptr(side.Precision)})
		}
	case *model.OnOffWidget:
		for _, c := range w.Components {
			on, off := firstAction(c.Actions, model.EventOn), firstAction(c.Actions, model.EventOff)
			if on == nil || off == nil {
				continue // §6.3.1: the component is skipped
			}
			add(&SwitchDescriptor{EntityInfo: info(EntitySwitch, fmt.Sprintf("%s_%d", w.ID, c.RowID), c.Label,
				componentIcon(w, c.Image)), State: c.State, OnAction: *on, OffAction: *off,
				OnIndication: c.OnIndication, OffIndication: c.OffIndication})
		}
	case *model.DimmerV2Widget:
		add(&DimmableLightDescriptor{EntityInfo: info(EntityDimmableLight, w.ID+"_0", w.Label, widgetIcon(w)),
			State: w.Value, Min: w.Min, Max: w.Max, Precision: w.Precision, OnAction: w.OnAction, OffAction: w.OffAction,
			SetValueAction: w.SetValueAction})
	case *model.LedWidget:
		if d := hsbLight(w); d != nil {
			add(d)
		}
	case *model.ContactSensorWidget:
		add(&ContactSensorDescriptor{EntityInfo: info(EntityContactSensor, w.ID+"_binary_sensor", w.Label, widgetIcon(w)),
			State: w.Value, Reversed: w.ReverseState, OnIndication: w.OnIndication, OffIndication: w.OffIndication})
		if w.ClickAction != nil {
			add(&TriggerDescriptor{EntityInfo: info(EntityTrigger, w.ID+"_button", w.Label, widgetIcon(w)),
				Action: *w.ClickAction})
		}
	case *model.ContactSensorDoubleWidget:
		for _, side := range []struct {
			name string
			c    model.ContactSide
		}{{"left", w.Left}, {"right", w.Right}} {
			icon := ptr(side.c.Icon)
			add(&ContactSensorDescriptor{EntityInfo: info(EntityContactSensor, w.ID+"_binary_sensor_"+side.name,
				side.c.Label, icon), State: side.c.Value, Reversed: side.c.ReverseState,
				OnIndication: side.c.OnIndication, OffIndication: side.c.OffIndication})
			if side.c.ClickAction != nil {
				add(&TriggerDescriptor{EntityInfo: info(EntityTrigger, w.ID+"_button_"+side.name, side.c.Label, icon),
					Action: *side.c.ClickAction})
			}
		}
	case *model.SliderWidget:
		add(&NumberSettingDescriptor{EntityInfo: info(EntityNumberSetting, w.ID+"_slider", w.Label, widgetIcon(w)),
			State: w.Value, SetValueAction: w.SetValueAction, Min: w.Min, Max: w.Max, Precision: w.Precision})
		if w.ClickAction != nil {
			add(&TriggerDescriptor{EntityInfo: info(EntityTrigger, w.ID+"_button", w.Label, widgetIcon(w)),
				Action: *w.ClickAction})
		}
	case *model.MultisensorWidget:
		for _, ch := range multisensorChannels {
			add(&SensorDescriptor{EntityInfo: info(EntitySensor, w.ID+"_"+ch.suffix, w.Label, widgetIcon(w)),
				State: ch.pick(w).Value, SensorKind: ch.kind, Unit: ptr(ch.unit)})
		}
	case *model.RollerShutterWidget:
		if len(w.Components) == 0 || len(w.Components[0].Actions) == 0 {
			break // §6.3.9 completeness
		}
		c := w.Components[0]
		icon := componentIcon(w, ptr(c.Image))
		add(&StateSensorDescriptor{EntityInfo: info(EntityStateSensor, w.ID+"_sensor", c.Label, icon), State: c.State})
		add(&TriggerDescriptor{EntityInfo: info(EntityTrigger, w.ID+"_button", c.Label, icon), Action: c.Actions[0]})
	case *model.RollerShutterV3Widget:
		d := &CoverDescriptor{EntityInfo: info(EntityCover, w.ID, w.Label, widgetIcon(w)), MotionState: w.Refs["state"],
			Position: w.Refs["position"], SetPositionAction: w.Actions["setPositionAction"]}
		if _, slats := w.Refs["lamelMoveTimeout"]; slats {
			d.Tilt = &CoverTilt{Position: w.Refs["lamelPosition"], SetAction: w.Actions["setLamelPositionAction"]}
		}
		if a, has := w.Actions["stopAction"]; has {
			d.StopAction = &a
		}
		add(d)
	case *model.CameraWidget:
		if len(w.Components) == 0 {
			break // §6.3.11 completeness
		}
		c := w.Components[0]
		add(&CameraDescriptor{EntityInfo: info(EntityCamera, w.ID+"_0", c.Label, componentIcon(w, ptr(c.Image))),
			StreamURL: c.Value})
	case *model.SceneWidget:
		for position, c := range w.Components {
			var script *model.ActionRef
			for i := range c.Actions {
				if c.Actions[i].Event == model.EventClick && c.Actions[i].CallType == model.ActionScript {
					script = &c.Actions[i]
					break
				}
			}
			if script == nil {
				continue // §6.3.10: the component is skipped
			}
			suffix := c.RowID
			if w.Double {
				suffix = int64(position)
			}
			add(&SceneTriggerDescriptor{EntityInfo: info(EntitySceneTrigger, fmt.Sprintf("%s_%d", w.ID, suffix), c.Label,
				widgetIcon(w)), Action: *script})
		}
	case *model.ThermostatV2Widget:
		d := &ThermostatDescriptor{EntityInfo: info(EntityThermostat, w.ID+"_0", w.Label, widgetIcon(w)),
			CurrentTemperature: w.Refs["currentTemperature"], TargetTemperature: w.Refs["targetTemperature"],
			SetTargetTemperatureAction: w.Actions["setTargetTemperatureAction"],
			MinTemperature:             optRef(w.Refs, "minTemperature"), MaxTemperature: optRef(w.Refs, "maxTemperature"),
			Output: optRef(w.Refs, "controlOutValue"),
			Mode:   control(w.Refs, w.Actions, "mode", "setModeAction"), Power: control(w.Refs, w.Actions, "state", "setStateAction")}
		add(d)
	case *model.EventSchedulerWidget:
		add(&EventScheduleDescriptor{EntityInfo: info(EntityEventSchedule, w.ID+"_0", w.Label, widgetIcon(w)),
			State: w.Refs["state"], RuleList: w.Refs["ruleList"], AddRuleAction: w.Actions["addRuleAction"],
			DeleteRuleAction: w.Actions["deleteRuleAction"],
			RuleCount:        optRef(w.Refs, "ruleCount"), RuleSpace: optRef(w.Refs, "ruleSpace"),
			NextRule: optRef(w.Refs, "nextRule"), CurrentRule: optRef(w.Refs, "currentRule"),
			StartAction: optAction(w.Actions, "startAction"), StopAction: optAction(w.Actions, "stopAction"),
			EnableRuleAction:  optAction(w.Actions, "enableRuleAction"),
			DisableRuleAction: optAction(w.Actions, "disableRuleAction")})
	case *model.TvRemoteWidget:
		buttons := make(map[string]model.ActionRef, len(w.Buttons))
		for k, v := range w.Buttons {
			buttons[k] = v
		}
		add(&TvRemoteDescriptor{EntityInfo: info(EntityTvRemote, w.ID+"_0", w.Label, widgetIcon(w)), Buttons: buttons})
	}
	return out
}

func optRef(refs map[string]model.ValueRef, key string) *model.ValueRef {
	if r, ok := refs[key]; ok {
		return &r
	}
	return nil
}

func optAction(actions map[string]model.ActionRef, key string) *model.ActionRef {
	if a, ok := actions[key]; ok {
		return &a
	}
	return nil
}

// control exists when both the feature and its action are present.
func control(refs map[string]model.ValueRef, actions map[string]model.ActionRef, ref, action string) *ThermostatControl {
	r, okR := refs[ref]
	a, okA := actions[action]
	if !okR || !okA {
		return nil
	}
	return &ThermostatControl{State: r, SetAction: a}
}

// hsbLight needs all four components (the first of each type), ON and OFF
// on the button and an action on every slider (spec §6.3.3).
func hsbLight(w *model.LedWidget) *HsbLightDescriptor {
	var button *model.LedButton
	sliders := map[string]*model.LedSlider{}
	for _, c := range w.Components {
		if c.Button != nil && button == nil {
			button = c.Button
		}
		if c.Slider != nil && sliders[c.Type] == nil {
			sliders[c.Type] = c.Slider
		}
	}
	hue, sat, bri := sliders["SLIDER_HUE"], sliders["SLIDER_SATURATION"], sliders["SLIDER_BRIGHTNESS"]
	if button == nil || hue == nil || sat == nil || bri == nil {
		return nil
	}
	on, off := firstAction(button.Actions, model.EventOn), firstAction(button.Actions, model.EventOff)
	if on == nil || off == nil || len(hue.Actions) == 0 || len(sat.Actions) == 0 || len(bri.Actions) == 0 {
		return nil
	}
	channel := func(s *model.LedSlider) HsbChannel {
		return HsbChannel{State: s.State, SetAction: s.Actions[0], Range: s.Range}
	}
	return &HsbLightDescriptor{EntityInfo: info(EntityHsbLight, w.ID+"_0", button.Label, componentIcon(w, ptr(button.Image))),
		PowerState: button.State, OnAction: *on, OffAction: *off,
		Hue: channel(hue), Saturation: channel(sat), Brightness: channel(bri)}
}
