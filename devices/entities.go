package devices

import (
	"errors"
	"strconv"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/internal/normalize"
	"github.com/osieckiAdam/grenton-core-go/model"
)

// CommandSink executes the actions entities send: a System routes them to
// the CLU sessions, a test records them.
type CommandSink interface {
	Execute(a model.ActionRef) error
}

// ErrInvalidRule: AddRule got an hour outside 0–23, a minute outside 0–59,
// a day outside Sunday–Saturday or no days; nothing was sent (spec §6.3.13).
var ErrInvalidRule = errors.New("devices: a rule outside 0–23 h, 0–59 min or without days")

// Entity is a bound entity: one of the entity types of this package (Switch,
// DimmableLight, …). Its getters read the state store and never block on
// the network; its intents send through the CommandSink.
type Entity interface {
	// Info returns the entity's kind, id, name and icon.
	Info() EntityInfo
	// Updates returns a channel that receives a value after any change of
	// the entity's state (spec §6.5): a flag, not a history. When it
	// fires, the store already holds the change; read the getters for the
	// state. cancel stops the updates.
	Updates() (updates <-chan struct{}, cancel func())
}

// BoundDevice is a device with live entities.
type BoundDevice struct {
	Device
	Entities []Entity
}

// Bind binds descriptors to a store and a sink (spec §6.1 stage 2). It
// registers every reference the entities read in the store, in order.
func Bind(devs []Device, store *clu.Store, sink CommandSink) []BoundDevice {
	out := make([]BoundDevice, len(devs))
	for i, d := range devs {
		out[i] = BoundDevice{Device: d}
		for _, desc := range d.Entities {
			e := bind(desc, base{store: store, sink: sink})
			for _, r := range e.refs() {
				store.Register(r)
			}
			out[i].Entities = append(out[i].Entities, e)
		}
	}
	return out
}

type boundEntity interface {
	Entity
	refs() []model.ValueRef
}

// base is what every bound entity shares.
type base struct {
	store *clu.Store
	sink  CommandSink
}

func (b base) get(r model.ValueRef) (model.Value, bool) { return b.store.Get(r) }

// number is a numeric read (spec §6.2): an int or a double; null, unknown
// and other types are unknown.
func (b base) number(r model.ValueRef) (float64, bool) {
	v, ok := b.get(r)
	if !ok {
		return 0, false
	}
	return v.Number()
}

func (b base) optNumber(r *model.ValueRef) (float64, bool) {
	if r == nil {
		return 0, false
	}
	return b.number(*r)
}

func (b base) integer(r model.ValueRef) (int64, bool) {
	v, ok := b.get(r)
	if !ok {
		return 0, false
	}
	return normalize.Integer(v)
}

func (b base) truthy(r model.ValueRef) (bool, bool) {
	v, ok := b.get(r)
	if !ok {
		return false, false
	}
	return normalize.Truthy(v)
}

func (b base) send(actions ...model.ActionRef) error {
	for _, a := range actions {
		if err := b.sink.Execute(a); err != nil {
			return err
		}
	}
	return nil
}

func (b base) updates(refs []model.ValueRef) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	cancel := b.store.Listen(func([]clu.Change) {
		select {
		case ch <- struct{}{}:
		default: // a signal is already pending: it covers this change too
		}
	}, refs...)
	return ch, cancel
}

func bind(d Descriptor, b base) boundEntity {
	switch d := d.(type) {
	case *SwitchDescriptor:
		return &Switch{base: b, d: *d}
	case *DimmableLightDescriptor:
		return &DimmableLight{base: b, d: *d}
	case *HsbLightDescriptor:
		return &HsbLight{base: b, d: *d}
	case *ContactSensorDescriptor:
		return &ContactSensor{base: b, d: *d}
	case *SensorDescriptor:
		return &Sensor{base: b, d: *d}
	case *NumberSettingDescriptor:
		return &NumberSetting{base: b, d: *d}
	case *CoverDescriptor:
		return &Cover{base: b, d: *d}
	case *StateSensorDescriptor:
		return &StateSensor{base: b, d: *d}
	case *TriggerDescriptor:
		return &Trigger{base: b, d: *d}
	case *SceneTriggerDescriptor:
		return &SceneTrigger{base: b, d: *d}
	case *CameraDescriptor:
		return &Camera{base: b, d: *d}
	case *ThermostatDescriptor:
		return &Thermostat{base: b, d: *d}
	case *EventScheduleDescriptor:
		return &EventSchedule{base: b, d: *d}
	case *TvRemoteDescriptor:
		return &TvRemote{base: b, d: *d}
	}
	panic("devices: unbound descriptor")
}

func refs(rs ...*model.ValueRef) []model.ValueRef {
	var out []model.ValueRef
	for _, r := range rs {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// Switch is an on/off relay (spec §6.3.1).
type Switch struct {
	base
	d SwitchDescriptor
}

func (e *Switch) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *Switch) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *Switch) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// Descriptor returns the switch's references, actions and indications.
func (e *Switch) Descriptor() SwitchDescriptor { return e.d }

// IsOn reports the state; ok is false while it is unknown.
func (e *Switch) IsOn() (on, ok bool) {
	v, known := e.get(e.d.State)
	if !known {
		return false, false
	}
	return normalize.BoolFromOnOff(v)
}

// TurnOn sends the ON action.
func (e *Switch) TurnOn() error { return e.send(e.d.OnAction) }

// TurnOff sends the OFF action.
func (e *Switch) TurnOff() error { return e.send(e.d.OffAction) }

// DimmableLight is a dimmer (spec §6.3.2).
type DimmableLight struct {
	base
	d DimmableLightDescriptor
}

func (e *DimmableLight) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *DimmableLight) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *DimmableLight) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// IsOn is value > 0.
func (e *DimmableLight) IsOn() (on, ok bool) {
	v, ok := e.number(e.d.State)
	return v > 0, ok
}

// Brightness is in [0,1].
func (e *DimmableLight) Brightness() (float64, bool) {
	v, ok := e.number(e.d.State)
	if !ok {
		return 0, false
	}
	return normalize.Clamp(normalize.MapRange(e.d.Min, e.d.Max, 0, 1, v), 0, 1), true
}

// TurnOn sends the on action.
func (e *DimmableLight) TurnOn() error { return e.send(e.d.OnAction) }

// TurnOff sends the off action.
func (e *DimmableLight) TurnOff() error { return e.send(e.d.OffAction) }

// SetBrightness sends b in [0,1] (clamped) on the device's scale.
func (e *DimmableLight) SetBrightness(b float64) error {
	v := normalize.MapRange(0, 1, e.d.Min, e.d.Max, normalize.Clamp(b, 0, 1))
	return e.send(e.d.SetValueAction.WithValue(normalize.FormatNumber(v, int(e.d.Precision))))
}

// HsbLight is an LED (spec §6.3.3).
type HsbLight struct {
	base
	d HsbLightDescriptor
}

func (e *HsbLight) Info() EntityInfo { return e.d.EntityInfo }
func (e *HsbLight) refs() []model.ValueRef {
	return []model.ValueRef{e.d.PowerState, e.d.Hue.State, e.d.Saturation.State, e.d.Brightness.State}
}
func (e *HsbLight) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// IsOn is the button's truthiness.
func (e *HsbLight) IsOn() (on, ok bool) { return e.truthy(e.d.PowerState) }

func (e *HsbLight) channel(c HsbChannel, lo, hi float64) (float64, bool) {
	v, ok := e.number(c.State)
	if !ok {
		return 0, false
	}
	return normalize.Clamp(normalize.MapRange(float64(c.Range.Min), float64(c.Range.Max), lo, hi, v), lo, hi), true
}

// Hue is in [0,360].
func (e *HsbLight) Hue() (float64, bool) { return e.channel(e.d.Hue, 0, 360) }

// Saturation is in [0,1].
func (e *HsbLight) Saturation() (float64, bool) { return e.channel(e.d.Saturation, 0, 1) }

// Brightness is in [0,1].
func (e *HsbLight) Brightness() (float64, bool) { return e.channel(e.d.Brightness, 0, 1) }

func (e *HsbLight) set(c HsbChannel, lo, hi, v float64) model.ActionRef {
	device := normalize.MapRange(lo, hi, float64(c.Range.Min), float64(c.Range.Max), normalize.Clamp(v, lo, hi))
	return c.SetAction.WithValue(normalize.FormatNumber(device, 2))
}

// SetColor sends hue then saturation.
func (e *HsbLight) SetColor(hue, saturation float64) error {
	return e.send(e.set(e.d.Hue, 0, 360, hue), e.set(e.d.Saturation, 0, 1, saturation))
}

// SetBrightness sends the brightness.
func (e *HsbLight) SetBrightness(b float64) error { return e.send(e.set(e.d.Brightness, 0, 1, b)) }

// TurnOn sends the ON action.
func (e *HsbLight) TurnOn() error { return e.send(e.d.OnAction) }

// TurnOff sends the OFF action.
func (e *HsbLight) TurnOff() error { return e.send(e.d.OffAction) }

// ContactSensor is a binary sensor (spec §6.3.4).
type ContactSensor struct {
	base
	d ContactSensorDescriptor
}

func (e *ContactSensor) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *ContactSensor) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *ContactSensor) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// IsActive is truthy(value) XOR reverseState.
func (e *ContactSensor) IsActive() (active, ok bool) {
	t, ok := e.truthy(e.d.State)
	return t != e.d.Reversed, ok
}

// Indications returns the labels of the on and off states.
func (e *ContactSensor) Indications() (on, off string) { return e.d.OnIndication, e.d.OffIndication }

// Sensor is a value (spec §6.3.5–§6.3.6).
type Sensor struct {
	base
	d SensorDescriptor
}

func (e *Sensor) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *Sensor) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *Sensor) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// Value is the stored value as it is; ok is false until it is reported.
func (e *Sensor) Value() (model.Value, bool) { return e.get(e.d.State) }

// Descriptor returns the kind, unit and presentation metadata.
func (e *Sensor) Descriptor() SensorDescriptor { return e.d }

// NumberSetting is a slider (spec §6.3.7).
type NumberSetting struct {
	base
	d NumberSettingDescriptor
}

func (e *NumberSetting) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *NumberSetting) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *NumberSetting) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// Value is the raw number.
func (e *NumberSetting) Value() (float64, bool) { return e.number(e.d.State) }

// Range returns min, max and the step 10^−precision.
func (e *NumberSetting) Range() (min, max, step float64) {
	return e.d.Min, e.d.Max, normalize.PowerOfTen(-int(e.d.Precision))
}

// SetValue sends v clamped to [min, max].
func (e *NumberSetting) SetValue(v float64) error {
	v = normalize.Clamp(v, e.d.Min, e.d.Max)
	return e.send(e.d.SetValueAction.WithValue(normalize.FormatNumber(v, int(e.d.Precision))))
}

// Motion is a cover's movement.
type Motion string

// The motions of spec §6.3.8.
const (
	MotionIdle    Motion = "idle"
	MotionOpening Motion = "opening"
	MotionClosing Motion = "closing"
	MotionUnknown Motion = "unknown"
)

// Cover is a roller shutter (spec §6.3.8). Position and tilt are 0–100,
// 0 = closed.
type Cover struct {
	base
	d CoverDescriptor
}

func (e *Cover) Info() EntityInfo { return e.d.EntityInfo }
func (e *Cover) refs() []model.ValueRef {
	r := []model.ValueRef{e.d.MotionState, e.d.Position}
	if e.d.Tilt != nil {
		r = append(r, e.d.Tilt.Position)
	}
	return r
}
func (e *Cover) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

func clampInt(v, lo, hi int64) int64 {
	return max(lo, min(hi, v))
}

// Position is the integer read of position, clamped to 0–100.
func (e *Cover) Position() (int64, bool) {
	p, ok := e.integer(e.d.Position)
	return clampInt(p, 0, 100), ok
}

// IsClosed is position == 0.
func (e *Cover) IsClosed() (closed, ok bool) {
	p, ok := e.Position()
	return p == 0, ok
}

// Motion is idle, opening, closing or unknown.
func (e *Cover) Motion() Motion {
	m, ok := e.integer(e.d.MotionState)
	if !ok {
		return MotionUnknown
	}
	switch m {
	case 0:
		return MotionIdle
	case 1:
		return MotionOpening
	case 2:
		return MotionClosing
	}
	return MotionUnknown
}

// HasTilt reports slats.
func (e *Cover) HasTilt() bool { return e.d.Tilt != nil }

// Tilt is int(100 − mapRange((0,90)→(0,100), L)) clamped, L the integer
// read of the slats (reported 0–90 on the inverse scale).
func (e *Cover) Tilt() (int64, bool) {
	if e.d.Tilt == nil {
		return 0, false
	}
	l, ok := e.integer(e.d.Tilt.Position)
	if !ok {
		return 0, false
	}
	return clampInt(int64(100-normalize.MapRange(0, 90, 0, 100, float64(l))), 0, 100), true
}

func (e *Cover) setPosition(p int64) error {
	return e.send(e.d.SetPositionAction.WithValue(strconv.FormatInt(clampInt(p, 0, 100), 10)))
}

// Open sets position 100.
func (e *Cover) Open() error { return e.setPosition(100) }

// Close sets position 0.
func (e *Cover) Close() error { return e.setPosition(0) }

// SetPosition sends p clamped to 0–100.
func (e *Cover) SetPosition(p int64) error { return e.setPosition(p) }

// Stop sends the stop action; a no-op without one.
func (e *Cover) Stop() error {
	if e.d.StopAction == nil {
		return nil
	}
	return e.send(*e.d.StopAction)
}

// SetTilt sends t clamped to 0–100 on the device's inverse 0–90 scale; a
// no-op without slats.
func (e *Cover) SetTilt(t int64) error {
	if e.d.Tilt == nil {
		return nil
	}
	device := int64(90 - normalize.MapRange(0, 100, 0, 90, float64(clampInt(t, 0, 100))))
	return e.send(e.d.Tilt.SetAction.WithValue(strconv.FormatInt(device, 10)))
}

// OpenTilt is SetTilt(100).
func (e *Cover) OpenTilt() error { return e.SetTilt(100) }

// CloseTilt is SetTilt(0).
func (e *Cover) CloseTilt() error { return e.SetTilt(0) }

// ShutterState is a legacy roller shutter's state (spec §6.3.9).
type ShutterState string

// The states.
const (
	ShutterStopped ShutterState = "stopped"
	ShutterOpening ShutterState = "opening"
	ShutterClosing ShutterState = "closing"
	ShutterLocked  ShutterState = "locked"
	ShutterUnknown ShutterState = "unknown"
)

// StateSensor is a legacy roller shutter's state.
type StateSensor struct {
	base
	d StateSensorDescriptor
}

func (e *StateSensor) Info() EntityInfo                   { return e.d.EntityInfo }
func (e *StateSensor) refs() []model.ValueRef             { return []model.ValueRef{e.d.State} }
func (e *StateSensor) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// State is the enumerated state.
func (e *StateSensor) State() ShutterState {
	s, ok := e.integer(e.d.State)
	if !ok {
		return ShutterUnknown
	}
	switch s {
	case 0:
		return ShutterStopped
	case 1:
		return ShutterOpening
	case 2:
		return ShutterClosing
	case 3:
		return ShutterLocked
	}
	return ShutterUnknown
}

// stateless is the update side of an entity without state.
type stateless struct{}

func (stateless) refs() []model.ValueRef { return nil }

// Updates of a stateless entity never fire.
func (stateless) Updates() (<-chan struct{}, func()) { return make(chan struct{}), func() {} }

// Trigger is a button (spec §6.3.9).
type Trigger struct {
	base
	stateless
	d TriggerDescriptor
}

func (e *Trigger) Info() EntityInfo { return e.d.EntityInfo }

// Press sends the action.
func (e *Trigger) Press() error { return e.send(e.d.Action) }

// SceneTrigger runs a scene's script (spec §6.3.10).
type SceneTrigger struct {
	base
	stateless
	d SceneTriggerDescriptor
}

func (e *SceneTrigger) Info() EntityInfo { return e.d.EntityInfo }

// Run executes the script with the widget's value.
func (e *SceneTrigger) Run() error { return e.send(e.d.Action) }

// RunWith executes the script with parameter as its value, verbatim: Lua
// syntax is the caller's ("abc" with the quotes, 42, "" for none).
func (e *SceneTrigger) RunWith(parameter string) error {
	return e.send(e.d.Action.WithValue(parameter))
}

// Camera is a stream (spec §6.3.11); Core does not touch it.
type Camera struct {
	base
	stateless
	d CameraDescriptor
}

func (e *Camera) Info() EntityInfo { return e.d.EntityInfo }

// StreamURL is the stream's URL, e.g. RTSP.
func (e *Camera) StreamURL() string { return e.d.StreamURL }

// ThermostatMode is a thermostat's mode (spec §6.3.12).
type ThermostatMode string

// The modes.
const (
	ModeManual   ThermostatMode = "manual"
	ModeHoliday  ThermostatMode = "holiday"
	ModeSchedule ThermostatMode = "schedule"
)

// Thermostat is a THERMOSTAT_V2 (spec §6.3.12).
type Thermostat struct {
	base
	d ThermostatDescriptor
}

func (e *Thermostat) Info() EntityInfo { return e.d.EntityInfo }
func (e *Thermostat) refs() []model.ValueRef {
	r := refs(&e.d.CurrentTemperature, &e.d.TargetTemperature, e.d.MinTemperature, e.d.MaxTemperature, e.d.Output)
	if e.d.Mode != nil {
		r = append(r, e.d.Mode.State)
	}
	if e.d.Power != nil {
		r = append(r, e.d.Power.State)
	}
	return r
}
func (e *Thermostat) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// CurrentTemperature is the measured temperature.
func (e *Thermostat) CurrentTemperature() (float64, bool) { return e.number(e.d.CurrentTemperature) }

// TargetTemperature is the resulting setpoint.
func (e *Thermostat) TargetTemperature() (float64, bool) { return e.number(e.d.TargetTemperature) }

// MinTemperature is the lower limit, when the widget has one.
func (e *Thermostat) MinTemperature() (float64, bool) { return e.optNumber(e.d.MinTemperature) }

// MaxTemperature is the upper limit, when the widget has one.
func (e *Thermostat) MaxTemperature() (float64, bool) { return e.optNumber(e.d.MaxTemperature) }

// Output is the control output (0/1, or a level).
func (e *Thermostat) Output() (float64, bool) { return e.optNumber(e.d.Output) }

// Mode is manual, holiday or schedule.
func (e *Thermostat) Mode() (ThermostatMode, bool) {
	if e.d.Mode == nil {
		return "", false
	}
	m, ok := e.integer(e.d.Mode.State)
	if !ok {
		return "", false
	}
	switch m {
	case 0:
		return ModeManual, true
	case 1:
		return ModeHoliday, true
	case 2:
		return ModeSchedule, true
	}
	return "", false
}

// IsOn: the thermostat controls its output.
func (e *Thermostat) IsOn() (on, ok bool) {
	if e.d.Power == nil {
		return false, false
	}
	return e.truthy(e.d.Power.State)
}

// SetTargetTemperature sends t, clamped to [min, max] when both limits are
// known and min ≤ max, rounded to one place.
func (e *Thermostat) SetTargetTemperature(t float64) error {
	lo, okLo := e.MinTemperature()
	hi, okHi := e.MaxTemperature()
	if okLo && okHi && lo <= hi {
		t = normalize.Clamp(t, lo, hi)
	}
	return e.send(e.d.SetTargetTemperatureAction.WithValue(normalize.FormatNumber(t, 1)))
}

// SetMode sends the mode; a no-op without the control.
func (e *Thermostat) SetMode(m ThermostatMode) error {
	if e.d.Mode == nil {
		return nil
	}
	value, known := map[ThermostatMode]string{ModeManual: "0", ModeHoliday: "1", ModeSchedule: "2"}[m]
	if !known {
		return errors.New("devices: unknown thermostat mode " + string(m))
	}
	return e.send(e.d.Mode.SetAction.WithValue(value))
}

// TurnOn starts the thermostat; a no-op without the control.
func (e *Thermostat) TurnOn() error { return e.setPower("1") }

// TurnOff stops it; a no-op without the control.
func (e *Thermostat) TurnOff() error { return e.setPower("0") }

func (e *Thermostat) setPower(v string) error {
	if e.d.Power == nil {
		return nil
	}
	return e.send(e.d.Power.SetAction.WithValue(v))
}

// EventSchedule is an EVENT_SCHEDULER (spec §6.3.13).
type EventSchedule struct {
	base
	d EventScheduleDescriptor
}

func (e *EventSchedule) Info() EntityInfo { return e.d.EntityInfo }
func (e *EventSchedule) refs() []model.ValueRef {
	return refs(&e.d.State, &e.d.RuleList, e.d.RuleCount, e.d.RuleSpace, e.d.NextRule, e.d.CurrentRule)
}
func (e *EventSchedule) Updates() (<-chan struct{}, func()) { return e.updates(e.refs()) }

// IsActive: the scheduler fires its rules.
func (e *EventSchedule) IsActive() (active, ok bool) { return e.truthy(e.d.State) }

func (e *EventSchedule) rules(r *model.ValueRef) []ScheduleRule {
	if r == nil {
		return nil
	}
	v, ok := e.get(*r)
	if !ok {
		return nil
	}
	text, isText := v.AsString()
	if !isText {
		return nil
	}
	return ParseScheduleRules(text)
}

// Rules is the rule list in the CLU's order; nil when no rules are known.
func (e *EventSchedule) Rules() []ScheduleRule { return e.rules(&e.d.RuleList) }

// RuleCount is the number of rules, when the widget has it.
func (e *EventSchedule) RuleCount() (int64, bool) {
	if e.d.RuleCount == nil {
		return 0, false
	}
	return e.integer(*e.d.RuleCount)
}

// FreeRuleSlots is the number of free slots, when the widget has it.
func (e *EventSchedule) FreeRuleSlots() (int64, bool) {
	if e.d.RuleSpace == nil {
		return 0, false
	}
	return e.integer(*e.d.RuleSpace)
}

func first(rules []ScheduleRule) *ScheduleRule {
	if len(rules) == 0 {
		return nil
	}
	return &rules[0]
}

// NextRule is the rule due next, or nil.
func (e *EventSchedule) NextRule() *ScheduleRule { return first(e.rules(e.d.NextRule)) }

// CurrentRule is the rule running now, or nil.
func (e *EventSchedule) CurrentRule() *ScheduleRule { return first(e.rules(e.d.CurrentRule)) }

func (e *EventSchedule) optional(a *model.ActionRef, value string, override bool) error {
	if a == nil {
		return nil
	}
	if override {
		return e.send(a.WithValue(value))
	}
	return e.send(*a)
}

// SetActive starts or stops the scheduler; a no-op without the action.
func (e *EventSchedule) SetActive(active bool) error {
	if active {
		return e.optional(e.d.StartAction, "", false)
	}
	return e.optional(e.d.StopAction, "", false)
}

// AddRule adds a rule at hour:minute on the days; ErrInvalidRule (and
// nothing sent) for an hour, minute or day out of range or no days.
func (e *EventSchedule) AddRule(hour, minute int, days []time.Weekday) error {
	cron, ok := CronText(hour, minute, days)
	if !ok {
		return ErrInvalidRule
	}
	return e.send(e.d.AddRuleAction.WithValue(cron))
}

// DeleteRule deletes rule id.
func (e *EventSchedule) DeleteRule(id int64) error {
	return e.send(e.d.DeleteRuleAction.WithValue(strconv.FormatInt(id, 10)))
}

// EnableRule enables rule id; a no-op without the action.
func (e *EventSchedule) EnableRule(id int64) error {
	return e.optional(e.d.EnableRuleAction, strconv.FormatInt(id, 10), true)
}

// DisableRule disables rule id; a no-op without the action.
func (e *EventSchedule) DisableRule(id int64) error {
	return e.optional(e.d.DisableRuleAction, strconv.FormatInt(id, 10), true)
}

// TvRemote is a remote control (spec §6.3.14).
type TvRemote struct {
	base
	stateless
	d TvRemoteDescriptor
}

func (e *TvRemote) Info() EntityInfo { return e.d.EntityInfo }

// Buttons lists the buttons the remote has, in the order of spec §3.5.
func (e *TvRemote) Buttons() []string {
	var out []string
	for _, b := range model.TvButtons {
		if _, has := e.d.Buttons[b]; has {
			out = append(out, b)
		}
	}
	return out
}

// Press sends a button's action; a no-op for a button the remote lacks.
func (e *TvRemote) Press(button string) error {
	a, has := e.d.Buttons[button]
	if !has {
		return nil
	}
	return e.send(a)
}
