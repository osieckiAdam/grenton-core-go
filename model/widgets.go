package model

import (
	"encoding/json"
	"fmt"
)

// Unit is the Grenton unit of a list component or a value widget.
type Unit string

// The units of spec §3.5.
const (
	UnitUnknown Unit = "UNKNOWN"
	UnitPercent Unit = "PERCENT"
	UnitDegree  Unit = "DEGREE"
)

// ValueType is how a value widget presents its value.
type ValueType string

// The value types of spec §3.5.
const (
	ValueTypeString  ValueType = "STRING"
	ValueTypeFloat   ValueType = "FLOAT"
	ValueTypeInteger ValueType = "INTEGER"
)

// Indication is how a legacy roller shutter labels its state.
type Indication string

// The indications of spec §3.5.
const (
	IndicationOnOff      Indication = "ON_OFF"
	IndicationOpenClosed Indication = "OPEN_CLOSED"
	IndicationOpenLocked Indication = "OPEN_LOCKED"
	IndicationUpDown     Indication = "UP_DOWN"
)

// Widget is one widget of a page (spec §3.5): one of the *Widget types of
// this package.
type Widget interface {
	// WidgetID is the widget's id.
	WidgetID() string
	// WidgetType is its type discriminator, e.g. "ON_OFF".
	WidgetType() string
	isWidget()
}

// Components

// BistableButton is a BUTTON_BISTABLE (ON_OFF) or BUTTON_BISTABLE_V2
// (ON_OFF_DOUBLE) component. Image is the icon chosen in Object Manager.
type BistableButton struct {
	Label         string
	RowID         int64
	Unit          Unit
	Image         *string
	State         ValueRef
	Actions       []ActionRef
	OnIndication  string
	OffIndication string
}

// MonostableButton is a BUTTON_MONOSTABLE component of a scene.
type MonostableButton struct {
	Label   string
	RowID   int64
	Unit    Unit
	Actions []ActionRef
}

// CameraStream is a CAMERA_STREAM component; Value is the stream URL.
type CameraStream struct {
	Label string
	RowID int64
	Unit  Unit
	Image string
	Value string
}

// RollerShutterButton is the component of a legacy ROLLER_SHUTTER.
type RollerShutterButton struct {
	Label      string
	RowID      int64
	Unit       Unit
	Image      string
	State      ValueRef
	Indication Indication
	Actions    []ActionRef
}

// LedComponent is one component of an LED widget: Button for
// BUTTON_BISTABLE, Slider for the three sliders (Type tells which).
type LedComponent struct {
	Type   string
	Button *LedButton
	Slider *LedSlider
}

// LedButton is an LED widget's on/off button.
type LedButton struct {
	Label         string
	RowID         int64
	Unit          Unit
	Image         string
	State         ValueRef
	Actions       []ActionRef
	OnIndication  string
	OffIndication string
}

// LedSlider is an LED widget's hue, saturation or brightness slider.
type LedSlider struct {
	Label   string
	RowID   int64
	Unit    Unit
	State   ValueRef
	Actions []ActionRef
	Range   IntRange
}

// IntRange is a slider's device range.
type IntRange struct{ Min, Max int64 }

// ValueSide is one side of a VALUE_DOUBLE.
type ValueSide struct {
	Label     string
	Icon      string
	Unit      Unit
	ValueType ValueType
	Precision int64
	Value     ValueRef
}

// ContactSide is one side of a CONTACT_SENSOR_DOUBLE.
type ContactSide struct {
	Label         string
	Icon          string
	OnIndication  string
	OffIndication string
	ReverseState  bool
	Value         ValueRef
	ClickAction   *ActionRef
}

// MultisensorChannel is one channel object of a MULTISENSOR.
type MultisensorChannel struct {
	Value                                      ValueRef
	MinValue, MaxValue, Threshold, Sensitivity *ValueRef
}

// Widgets

// ValueV2Widget is VALUE_V2: a sensor value.
type ValueV2Widget struct {
	ID, Label, Icon string
	Unit            Unit
	ValueType       ValueType
	Precision       int64
	Value           ValueRef
}

// ValueDoubleWidget is VALUE_DOUBLE: two sensor values.
type ValueDoubleWidget struct {
	ID          string
	Left, Right ValueSide
}

// OnOffWidget is ON_OFF (Double false) or ON_OFF_DOUBLE (Double true).
type OnOffWidget struct {
	ID         string
	Double     bool
	Components []BistableButton
}

// DimmerV2Widget is DIMMER_V2.
type DimmerV2Widget struct {
	ID, Label, Icon                     string
	Min, Max                            float64
	Precision                           int64
	Value                               ValueRef
	SetValueAction, OnAction, OffAction ActionRef
}

// LedWidget is LED: an HSB light.
type LedWidget struct {
	ID         string
	Components []LedComponent
}

// ContactSensorWidget is CONTACT_SENSOR.
type ContactSensorWidget struct {
	ID, Label, Icon             string
	OnIndication, OffIndication string
	ReverseState                bool
	Value                       ValueRef
	ClickAction                 *ActionRef
}

// ContactSensorDoubleWidget is CONTACT_SENSOR_DOUBLE.
type ContactSensorDoubleWidget struct {
	ID          string
	Left, Right ContactSide
}

// SliderWidget is SLIDER: a number setting.
type SliderWidget struct {
	ID, Label, Icon string
	Unit            Unit
	Min, Max        float64
	Precision       int64
	Value           ValueRef
	SetValueAction  ActionRef
	ClickAction     *ActionRef
}

// MultisensorWidget is MULTISENSOR: seven fixed channels.
type MultisensorWidget struct {
	ID, Label, Icon                                               string
	LabelVisible                                                  bool
	AirCo2, Sound, AirVoc, Light, Pressure, Humidity, Temperature MultisensorChannel
}

// RollerShutterWidget is the legacy ROLLER_SHUTTER.
type RollerShutterWidget struct {
	ID         string
	Components []RollerShutterButton
}

// RollerShutterV3Widget is ROLLER_SHUTTER_V3. Refs and Actions hold every
// reference and action of its object by field name, the required ones and
// the optional ones present (the DTO keeps fields Core does not use).
type RollerShutterV3Widget struct {
	ID, Icon, Label string
	Refs            map[string]ValueRef
	Actions         map[string]ActionRef
}

// SceneWidget is SCENE (Double false) or SCENE_DOUBLE (Double true).
type SceneWidget struct {
	ID         string
	Double     bool
	Components []MonostableButton
}

// CameraWidget is CAMERA.
type CameraWidget struct {
	ID         string
	Components []CameraStream
}

// ThermostatV2Widget is THERMOSTAT_V2. Refs and Actions hold its object's
// references and actions by field name.
type ThermostatV2Widget struct {
	ID, Label     string
	Icon          *string
	NoOfFanSpeeds *int64
	Refs          map[string]ValueRef
	Actions       map[string]ActionRef
}

// EventSchedulerWidget is EVENT_SCHEDULER. Refs and Actions hold its
// object's references and actions by field name.
type EventSchedulerWidget struct {
	ID, Label, Icon string
	Refs            map[string]ValueRef
	Actions         map[string]ActionRef
}

// TvRemoteWidget is TV_REMOTE_CONTROL; Buttons holds the buttons of spec
// §3.5 whose entry parses as an action.
type TvRemoteWidget struct {
	ID, Label, Icon, VodLabel string
	Buttons                   map[string]ActionRef
}

func (w *ValueV2Widget) WidgetID() string             { return w.ID }
func (w *ValueDoubleWidget) WidgetID() string         { return w.ID }
func (w *OnOffWidget) WidgetID() string               { return w.ID }
func (w *DimmerV2Widget) WidgetID() string            { return w.ID }
func (w *LedWidget) WidgetID() string                 { return w.ID }
func (w *ContactSensorWidget) WidgetID() string       { return w.ID }
func (w *ContactSensorDoubleWidget) WidgetID() string { return w.ID }
func (w *SliderWidget) WidgetID() string              { return w.ID }
func (w *MultisensorWidget) WidgetID() string         { return w.ID }
func (w *RollerShutterWidget) WidgetID() string       { return w.ID }
func (w *RollerShutterV3Widget) WidgetID() string     { return w.ID }
func (w *SceneWidget) WidgetID() string               { return w.ID }
func (w *CameraWidget) WidgetID() string              { return w.ID }
func (w *ThermostatV2Widget) WidgetID() string        { return w.ID }
func (w *EventSchedulerWidget) WidgetID() string      { return w.ID }
func (w *TvRemoteWidget) WidgetID() string            { return w.ID }

func (w *ValueV2Widget) WidgetType() string             { return "VALUE_V2" }
func (w *ValueDoubleWidget) WidgetType() string         { return "VALUE_DOUBLE" }
func (w *DimmerV2Widget) WidgetType() string            { return "DIMMER_V2" }
func (w *LedWidget) WidgetType() string                 { return "LED" }
func (w *ContactSensorWidget) WidgetType() string       { return "CONTACT_SENSOR" }
func (w *ContactSensorDoubleWidget) WidgetType() string { return "CONTACT_SENSOR_DOUBLE" }
func (w *SliderWidget) WidgetType() string              { return "SLIDER" }
func (w *MultisensorWidget) WidgetType() string         { return "MULTISENSOR" }
func (w *RollerShutterWidget) WidgetType() string       { return "ROLLER_SHUTTER" }
func (w *RollerShutterV3Widget) WidgetType() string     { return "ROLLER_SHUTTER_V3" }
func (w *CameraWidget) WidgetType() string              { return "CAMERA" }
func (w *ThermostatV2Widget) WidgetType() string        { return "THERMOSTAT_V2" }
func (w *EventSchedulerWidget) WidgetType() string      { return "EVENT_SCHEDULER" }
func (w *TvRemoteWidget) WidgetType() string            { return "TV_REMOTE_CONTROL" }

// WidgetType is ON_OFF or ON_OFF_DOUBLE.
func (w *OnOffWidget) WidgetType() string {
	if w.Double {
		return "ON_OFF_DOUBLE"
	}
	return "ON_OFF"
}

// WidgetType is SCENE or SCENE_DOUBLE.
func (w *SceneWidget) WidgetType() string {
	if w.Double {
		return "SCENE_DOUBLE"
	}
	return "SCENE"
}

func (*ValueV2Widget) isWidget()             {}
func (*ValueDoubleWidget) isWidget()         {}
func (*OnOffWidget) isWidget()               {}
func (*DimmerV2Widget) isWidget()            {}
func (*LedWidget) isWidget()                 {}
func (*ContactSensorWidget) isWidget()       {}
func (*ContactSensorDoubleWidget) isWidget() {}
func (*SliderWidget) isWidget()              {}
func (*MultisensorWidget) isWidget()         {}
func (*RollerShutterWidget) isWidget()       {}
func (*RollerShutterV3Widget) isWidget()     {}
func (*SceneWidget) isWidget()               {}
func (*CameraWidget) isWidget()              {}
func (*ThermostatV2Widget) isWidget()        {}
func (*EventSchedulerWidget) isWidget()      {}
func (*TvRemoteWidget) isWidget()            {}

// TvButtons are the button names of spec §3.5, in its order.
var TvButtons = []string{"on", "off", "up", "down", "left", "right", "ok", "menu", "back", "guide", "play",
	"pause", "fastForward", "fastRewind", "nextChannel", "previousChannel", "volumeUp", "volumeDown",
	"mute", "vod", "init"}

// Decoding. A reader keeps the first error, so a widget decoder reads its
// fields in a row and checks once; every field it reads is required unless
// read with an opt* method.

type reader struct {
	o      object
	err    error
	parent *reader // a child's first error is also its parent's
}

func (r *reader) fail(err error) {
	if err == nil {
		return
	}
	for p := r; p != nil; p = p.parent {
		if p.err == nil {
			p.err = err
		}
	}
}

func (r *reader) str(key string) string {
	if r.err != nil {
		return ""
	}
	s, err := r.o.str(key)
	r.fail(err)
	return s
}

func (r *reader) optStr(key string) *string {
	if r.err != nil {
		return nil
	}
	s, ok, err := r.o.optStr(key)
	r.fail(err)
	if !ok {
		return nil
	}
	return &s
}

func (r *reader) integer(key string) int64 {
	if r.err != nil {
		return 0
	}
	i, err := r.o.integer(key)
	r.fail(err)
	return i
}

func (r *reader) double(key string) float64 {
	if r.err != nil {
		return 0
	}
	f, err := r.o.double(key)
	r.fail(err)
	return f
}

func (r *reader) boolean(key string) bool {
	if r.err != nil {
		return false
	}
	b, err := r.o.boolean(key)
	r.fail(err)
	return b
}

func (r *reader) unit() Unit {
	return Unit(r.enum("unit", string(UnitUnknown), string(UnitPercent), string(UnitDegree)))
}

func (r *reader) valueType() ValueType {
	return ValueType(r.enum("valueType", string(ValueTypeString), string(ValueTypeFloat), string(ValueTypeInteger)))
}

func (r *reader) enum(key string, allowed ...string) string {
	if r.err != nil {
		return ""
	}
	s, err := r.o.enum(key, allowed...)
	r.fail(err)
	return s
}

func (r *reader) ref(key string) ValueRef {
	if r.err != nil {
		return ValueRef{}
	}
	v, err := r.o.refField(key)
	r.fail(err)
	return v
}

func (r *reader) optRef(key string) *ValueRef {
	if r.err != nil {
		return nil
	}
	v, err := r.o.optRef(key)
	r.fail(err)
	return v
}

func (r *reader) action(key string) ActionRef {
	if r.err != nil {
		return ActionRef{}
	}
	a, err := r.o.actionField(key)
	r.fail(err)
	return a
}

func (r *reader) optAction(key string) *ActionRef {
	if r.err != nil {
		return nil
	}
	a, err := r.o.optAction(key)
	r.fail(err)
	return a
}

func (r *reader) actions(key string) []ActionRef {
	if r.err != nil {
		return nil
	}
	a, err := r.o.actions(key)
	r.fail(err)
	return a
}

func (r *reader) child(key string) *reader {
	if r.err != nil {
		return &reader{err: r.err, parent: r}
	}
	c, err := r.o.child(key)
	r.fail(err)
	return &reader{o: c, err: err, parent: r}
}

// list decodes every element of an array with decode; the first failing
// element fails the widget (spec §3.6 item 3).
func (r *reader) list(key string, decode func(*reader)) {
	if r.err != nil {
		return
	}
	raws, err := r.o.array(key)
	if err != nil {
		r.fail(err)
		return
	}
	for n, raw := range raws {
		o, err := decodeObject(raw)
		if err != nil {
			r.fail(errField{fmt.Sprintf("%s[%d]", key, n), "not an object"})
			return
		}
		c := &reader{o: o}
		decode(c)
		if c.err != nil {
			r.fail(errField{fmt.Sprintf("%s[%d]", key, n), c.err.Error()})
			return
		}
	}
}

// typeLiteral checks a component's fixed type: it may be absent, but when
// present it must be that string (null included is a mismatch).
func (r *reader) typeLiteral(want string) {
	if r.err != nil {
		return
	}
	if _, ok := r.o["type"]; !ok {
		return
	}
	var got string
	if err := json.Unmarshal(r.o["type"], &got); err != nil || got != want {
		r.fail(errField{"type", fmt.Sprintf("expected %q", want)})
	}
}

// refsAndActions reads the named required and optional references and
// actions of a widget object into maps.
func (r *reader) refsAndActions(reqRefs, optRefs, reqActions, optActions []string) (map[string]ValueRef, map[string]ActionRef) {
	refs, actions := map[string]ValueRef{}, map[string]ActionRef{}
	for _, k := range reqRefs {
		refs[k] = r.ref(k)
	}
	for _, k := range optRefs {
		if v := r.optRef(k); v != nil {
			refs[k] = *v
		}
	}
	for _, k := range reqActions {
		actions[k] = r.action(k)
	}
	for _, k := range optActions {
		if a := r.optAction(k); a != nil {
			actions[k] = *a
		}
	}
	return refs, actions
}

var widgetDecoders = map[string]func(object) (Widget, error){
	"VALUE_V2": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &ValueV2Widget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"), Unit: r.unit(),
			ValueType: r.valueType(), Precision: r.integer("precision")}
		w.Value = r.child("object").ref("value")
		return w, r.errOr()
	},
	"VALUE_DOUBLE": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &ValueDoubleWidget{ID: r.str("id"), Left: valueSide(r.child("componentLeft")),
			Right: valueSide(r.child("componentRight"))}
		return w, r.errOr()
	},
	"ON_OFF":        func(o object) (Widget, error) { return onOff(o, false) },
	"ON_OFF_DOUBLE": func(o object) (Widget, error) { return onOff(o, true) },
	"DIMMER_V2": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &DimmerV2Widget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"), Min: r.double("min"),
			Max: r.double("max"), Precision: r.integer("precision")}
		obj := r.child("object")
		w.Value, w.SetValueAction = obj.ref("value"), obj.action("setValueAction")
		w.OnAction, w.OffAction = obj.action("onAction"), obj.action("offAction")
		r.fail(obj.err)
		return w, r.errOr()
	},
	"LED": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &LedWidget{ID: r.str("id")}
		r.list("components", func(c *reader) {
			t := c.enum("type", "BUTTON_BISTABLE", "SLIDER_HUE", "SLIDER_SATURATION", "SLIDER_BRIGHTNESS")
			comp := LedComponent{Type: t}
			if t == "BUTTON_BISTABLE" {
				comp.Button = &LedButton{Label: c.str("label"), RowID: c.integer("rowId"), Unit: c.unit(),
					State: c.ref("state"), Actions: c.actions("actions"), Image: c.str("image"),
					OnIndication: c.str("onIndication"), OffIndication: c.str("offIndication")}
			} else {
				comp.Slider = &LedSlider{Label: c.str("label"), RowID: c.integer("rowId"), Unit: c.unit(),
					State: c.ref("state"), Actions: c.actions("actions")}
				rg := c.child("range")
				comp.Slider.Range = IntRange{Min: rg.integer("min"), Max: rg.integer("max")}
				c.fail(rg.err)
			}
			w.Components = append(w.Components, comp)
		})
		return w, r.errOr()
	},
	"CONTACT_SENSOR": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &ContactSensorWidget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"),
			OnIndication: r.str("onIndication"), OffIndication: r.str("offIndication"),
			ReverseState: r.boolean("reverseState")}
		obj := r.child("object")
		w.Value, w.ClickAction = obj.ref("value"), obj.optAction("clickAction")
		r.fail(obj.err)
		return w, r.errOr()
	},
	"CONTACT_SENSOR_DOUBLE": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &ContactSensorDoubleWidget{ID: r.str("id"), Left: contactSide(r.child("componentLeft")),
			Right: contactSide(r.child("componentRight"))}
		return w, r.errOr()
	},
	"SLIDER": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &SliderWidget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"), Unit: r.unit(),
			Min: r.double("min"), Max: r.double("max"), Precision: r.integer("precision")}
		obj := r.child("object")
		w.Value, w.SetValueAction, w.ClickAction = obj.ref("value"), obj.action("setValueAction"), obj.optAction("clickAction")
		r.fail(obj.err)
		return w, r.errOr()
	},
	"MULTISENSOR": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &MultisensorWidget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"),
			LabelVisible: r.boolean("labelVisible")}
		channel := func(key string) MultisensorChannel {
			c := r.child(key)
			ch := MultisensorChannel{Value: c.ref("value"), MinValue: c.optRef("minValue"), MaxValue: c.optRef("maxValue"),
				Threshold: c.optRef("threshold"), Sensitivity: c.optRef("sensitivity")}
			r.fail(c.err)
			return ch
		}
		w.AirCo2, w.Sound, w.AirVoc = channel("objectAirCo2"), channel("objectSound"), channel("objectAirVoc")
		w.Light, w.Pressure = channel("objectLight"), channel("objectPressure")
		w.Humidity, w.Temperature = channel("objectHumidity"), channel("objectTemperature")
		return w, r.errOr()
	},
	"ROLLER_SHUTTER": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &RollerShutterWidget{ID: r.str("id")}
		r.list("components", func(c *reader) {
			c.typeLiteral("BUTTON_BISTABLE")
			w.Components = append(w.Components, RollerShutterButton{Label: c.str("label"), RowID: c.integer("rowId"),
				Unit: c.unit(), Image: c.str("image"), State: c.ref("state"),
				Indication: Indication(c.enum("indication", string(IndicationOnOff), string(IndicationOpenClosed),
					string(IndicationOpenLocked), string(IndicationUpDown))),
				Actions: c.actions("actions")})
		})
		return w, r.errOr()
	},
	"ROLLER_SHUTTER_V3": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &RollerShutterV3Widget{ID: r.str("id"), Icon: r.str("icon"), Label: r.str("label")}
		obj := r.child("object")
		w.Refs, w.Actions = obj.refsAndActions(
			[]string{"state", "position", "lamelPosition"},
			[]string{"up", "down", "loadCurrent", "overcurrent", "voltageType", "lamelMoveTimeout",
				"distributedLogicGroup", "reversePosition", "blindsUpMaxTime", "blindsDownMaxTime", "mechanicalOffset"},
			[]string{"setPositionAction", "setLamelPositionAction", "moveUpAction", "moveDownAction", "startAction",
				"setRollerBlockedAction"},
			[]string{"calibrationAction", "setBlindsMaxTimeAction", "setLamelMoveTimeoutAction", "stopAction",
				"holdAction", "holdUpAction", "holdDownAction", "lamelStartAction", "setMechanicalOffsetAction",
				"setBlindsUpMaxTimeAction", "setBlindsDownMaxTimeAction"})
		r.fail(obj.err)
		return w, r.errOr()
	},
	"CAMERA": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &CameraWidget{ID: r.str("id")}
		r.list("components", func(c *reader) {
			c.typeLiteral("CAMERA_STREAM")
			w.Components = append(w.Components, CameraStream{Label: c.str("label"), RowID: c.integer("rowId"),
				Unit: c.unit(), Image: c.str("image"), Value: c.str("value")})
		})
		return w, r.errOr()
	},
	"SCENE":        func(o object) (Widget, error) { return scene(o, false) },
	"SCENE_DOUBLE": func(o object) (Widget, error) { return scene(o, true) },
	"THERMOSTAT_V2": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &ThermostatV2Widget{ID: r.str("id"), Label: r.str("label"), Icon: r.optStr("icon")}
		if r.err == nil && r.o.present("no_of_fan_speeds") {
			n := r.integer("no_of_fan_speeds")
			w.NoOfFanSpeeds = &n
		}
		obj := r.child("object")
		w.Refs, w.Actions = obj.refsAndActions(
			[]string{"currentTemperature", "targetTemperature"},
			[]string{"minTemperature", "maxTemperature", "mode", "state", "controlOutValue", "scheduleData"},
			[]string{"setTargetTemperatureAction"},
			[]string{"setMinTemperatureAction", "setMaxTemperatureAction", "setModeAction", "setStateAction",
				"setScheduleDataAction"})
		r.fail(obj.err)
		return w, r.errOr()
	},
	"EVENT_SCHEDULER": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &EventSchedulerWidget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon")}
		obj := r.child("object")
		w.Refs, w.Actions = obj.refsAndActions(
			[]string{"state", "ruleList"},
			[]string{"ruleCount", "ruleSpace", "nextRule", "currentRule"},
			[]string{"addRuleAction", "deleteRuleAction"},
			[]string{"startAction", "stopAction", "enableRuleAction", "disableRuleAction", "getRuleAction"})
		r.fail(obj.err)
		return w, r.errOr()
	},
	"TV_REMOTE_CONTROL": func(o object) (Widget, error) {
		r := &reader{o: o}
		w := &TvRemoteWidget{ID: r.str("id"), Label: r.str("label"), Icon: r.str("icon"), VodLabel: r.str("vodLabel"),
			Buttons: map[string]ActionRef{}}
		obj := r.child("object")
		if r.err == nil {
			// §6.3.14: an entry that does not parse is left out, not an error.
			for _, name := range TvButtons {
				if c, err := obj.o.child(name); err == nil {
					if a, err := c.actionRef(); err == nil {
						w.Buttons[name] = a
					}
				}
			}
		}
		return w, r.errOr()
	},
}

func (r *reader) errOr() error { return r.err }

func onOff(o object, double bool) (Widget, error) {
	literal := "BUTTON_BISTABLE"
	if double {
		literal = "BUTTON_BISTABLE_V2"
	}
	r := &reader{o: o}
	w := &OnOffWidget{ID: r.str("id"), Double: double}
	r.list("components", func(c *reader) {
		c.typeLiteral(literal)
		w.Components = append(w.Components, BistableButton{Label: c.str("label"), RowID: c.integer("rowId"),
			Unit: c.unit(), State: c.ref("state"), Actions: c.actions("actions"),
			OnIndication: c.str("onIndication"), OffIndication: c.str("offIndication"), Image: c.optStr("image")})
	})
	return w, r.errOr()
}

func scene(o object, double bool) (Widget, error) {
	r := &reader{o: o}
	w := &SceneWidget{ID: r.str("id"), Double: double}
	r.list("components", func(c *reader) {
		c.typeLiteral("BUTTON_MONOSTABLE")
		w.Components = append(w.Components, MonostableButton{Label: c.str("label"), RowID: c.integer("rowId"),
			Unit: c.unit(), Actions: c.actions("actions")})
	})
	return w, r.errOr()
}

func valueSide(c *reader) ValueSide {
	s := ValueSide{Label: c.str("label"), Icon: c.str("icon"), Unit: c.unit(), ValueType: c.valueType(),
		Precision: c.integer("precision")}
	s.Value = c.child("object").ref("value")
	return s
}

func contactSide(c *reader) ContactSide {
	s := ContactSide{Label: c.str("label"), Icon: c.str("icon"), OnIndication: c.str("onIndication"),
		OffIndication: c.str("offIndication"), ReverseState: c.boolean("reverseState")}
	obj := c.child("object")
	s.Value, s.ClickAction = obj.ref("value"), obj.optAction("clickAction")
	c.fail(obj.err)
	return s
}
