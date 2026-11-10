package conformance_test

// The entities profile (conformance.md §4e): the production mapper and
// binder over a store the case fills, with a command sink in place of the
// CLU sessions. Reads call the entity's getters, intents its methods.

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/model"
)

func init() {
	registry["entity.read"] = entityOp(true)
	registry["entity.intent"] = entityOp(false)
}

// sink records every action, in order, and succeeds.
type sink struct{ actions []model.ActionRef }

func (s *sink) Execute(a model.ActionRef) error {
	s.actions = append(s.actions, a)
	return nil
}

func entityOp(read bool) operation {
	return func(s *suite, input map[string]any) (any, error) {
		data, err := readFixture(s, input)
		if err != nil {
			return nil, err
		}
		in, err := model.DecodeInterface(data)
		if err != nil {
			return nil, runnerErr("entity fixture does not decode: %v", err)
		}
		store := clu.NewStore()
		rec := &sink{}
		bound := devices.Bind(devices.Map(in).Devices, store, rec)
		values, err := list(input, "values")
		if err != nil {
			return nil, err
		}
		for _, entry := range values {
			e, _ := entry.(map[string]any)
			ref, err := valueRefInput(e["key"])
			if err != nil {
				return nil, err
			}
			v, err := untagged(e["value"])
			if err != nil {
				return nil, err
			}
			store.Register(ref)
			store.Update([]model.ValueRef{ref}, []clu.Slot{{Value: v}})
		}
		id, err := str(input, "entityId")
		if err != nil {
			return nil, err
		}
		var entity devices.Entity
		for _, d := range bound {
			for _, e := range d.Entities {
				if e.Info().ID == id {
					entity = e
				}
			}
		}
		if entity == nil {
			return nil, runnerErr("the mapping produces no entity %s", id)
		}
		if read {
			state, err := entityState(entity)
			if err != nil {
				return nil, err
			}
			return ok(map[string]any{"state": state}), nil
		}
		name, err := str(input, "intent")
		if err != nil {
			return nil, err
		}
		args, _ := input["args"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		if err := entityIntent(entity, name, args); err != nil {
			if errors.Is(err, devices.ErrInvalidRule) {
				if len(rec.actions) != 0 {
					return nil, runnerErr("refused, but sent %v", rec.actions)
				}
				return reject, nil
			}
			return nil, err
		}
		actions := make([]any, len(rec.actions))
		for i, a := range rec.actions {
			actions[i] = actionJSON(a)
		}
		return ok(map[string]any{"actions": actions}), nil
	}
}

func valueRefInput(k any) (model.ValueRef, error) {
	m, isMap := k.(map[string]any)
	if !isMap {
		return model.ValueRef{}, runnerErr("bad key %v", k)
	}
	cluID, err := str(m, "cluId")
	if err != nil {
		return model.ValueRef{}, err
	}
	index, err := str(m, "index")
	if err != nil {
		return model.ValueRef{}, err
	}
	switch m["callType"] {
	case "VARIABLE":
		return model.Variable(cluID, index), nil
	case "ATTRIBUTE":
		obj, err := str(m, "objectName")
		return model.Attribute(cluID, obj, index), err
	}
	return model.ValueRef{}, runnerErr("bad key %v", k)
}

func optBool(b, known bool) any { return optionalBool(b, known) }

func optDouble(f float64, known bool) any {
	if !known {
		return nil
	}
	return tagged(model.Double(f))
}

func optInt(i int64, known bool) any {
	if !known {
		return nil
	}
	return tagged(model.Int(i))
}

func optRule(r *devices.ScheduleRule) any {
	if r == nil {
		return nil
	}
	return ruleJSON(*r)
}

func entityState(e devices.Entity) (map[string]any, error) {
	switch e := e.(type) {
	case *devices.Switch:
		return map[string]any{"isOn": optBool(e.IsOn())}, nil
	case *devices.DimmableLight:
		return map[string]any{"isOn": optBool(e.IsOn()), "brightness": optDouble(e.Brightness())}, nil
	case *devices.HsbLight:
		return map[string]any{"isOn": optBool(e.IsOn()), "hue": optDouble(e.Hue()),
			"saturation": optDouble(e.Saturation()), "brightness": optDouble(e.Brightness())}, nil
	case *devices.ContactSensor:
		return map[string]any{"isActive": optBool(e.IsActive())}, nil
	case *devices.Sensor:
		v, known := e.Value()
		var value any
		if known {
			value = tagged(v)
		}
		return map[string]any{"value": value}, nil
	case *devices.NumberSetting:
		return map[string]any{"value": optDouble(e.Value())}, nil
	case *devices.Cover:
		state := map[string]any{"position": optInt(e.Position()), "isClosed": optBool(e.IsClosed()),
			"motion": string(e.Motion()), "tilt": optInt(e.Tilt())}
		return state, nil
	case *devices.StateSensor:
		return map[string]any{"state": string(e.State())}, nil
	case *devices.Thermostat:
		var mode any
		if m, known := e.Mode(); known {
			mode = string(m)
		}
		return map[string]any{"currentTemperature": optDouble(e.CurrentTemperature()),
			"targetTemperature": optDouble(e.TargetTemperature()), "minTemperature": optDouble(e.MinTemperature()),
			"maxTemperature": optDouble(e.MaxTemperature()), "output": optDouble(e.Output()), "mode": mode,
			"isOn": optBool(e.IsOn())}, nil
	case *devices.EventSchedule:
		var rules any
		if rs := e.Rules(); rs != nil {
			list := make([]any, len(rs))
			for i, r := range rs {
				list[i] = ruleJSON(r)
			}
			rules = list
		}
		return map[string]any{"isActive": optBool(e.IsActive()), "rules": rules,
			"ruleCount": optInt(e.RuleCount()), "freeRuleSlots": optInt(e.FreeRuleSlots()),
			"nextRule": optRule(e.NextRule()), "currentRule": optRule(e.CurrentRule())}, nil
	}
	return nil, runnerErr("entity.read: %s has no state", e.Info().ID)
}

func argNumber(args map[string]any, key string) (float64, error) {
	n, isNum := args[key].(json.Number)
	f, err := n.Float64()
	if !isNum || err != nil {
		return 0, runnerErr("intent needs a number %q", key)
	}
	return f, nil
}

func entityIntent(e devices.Entity, name string, args map[string]any) error {
	num := func(k string) float64 { f, _ := argNumber(args, k); return f }
	whole := func(k string) int64 { i, _ := integer(args, k); return i }
	text := func(k string) string { s, _ := str(args, k); return s }
	switch e := e.(type) {
	case *devices.Switch:
		switch name {
		case "turnOn":
			return e.TurnOn()
		case "turnOff":
			return e.TurnOff()
		}
	case *devices.DimmableLight:
		switch name {
		case "turnOn":
			return e.TurnOn()
		case "turnOff":
			return e.TurnOff()
		case "setBrightness":
			return e.SetBrightness(num("brightness"))
		}
	case *devices.HsbLight:
		switch name {
		case "turnOn":
			return e.TurnOn()
		case "turnOff":
			return e.TurnOff()
		case "setColor":
			return e.SetColor(num("hue"), num("saturation"))
		case "setBrightness":
			return e.SetBrightness(num("brightness"))
		}
	case *devices.NumberSetting:
		if name == "setValue" {
			return e.SetValue(num("value"))
		}
	case *devices.Cover:
		switch name {
		case "open":
			return e.Open()
		case "close":
			return e.Close()
		case "stop":
			return e.Stop()
		case "openTilt":
			return e.OpenTilt()
		case "closeTilt":
			return e.CloseTilt()
		case "setPosition":
			return e.SetPosition(whole("position"))
		case "setTilt":
			return e.SetTilt(whole("tilt"))
		}
	case *devices.Trigger:
		if name == "press" {
			return e.Press()
		}
	case *devices.SceneTrigger:
		if name == "run" {
			if _, has := args["parameter"]; has {
				return e.RunWith(text("parameter"))
			}
			return e.Run()
		}
	case *devices.Thermostat:
		switch name {
		case "setTargetTemperature":
			return e.SetTargetTemperature(num("temperature"))
		case "setMode":
			return e.SetMode(devices.ThermostatMode(text("mode")))
		case "turnOn":
			return e.TurnOn()
		case "turnOff":
			return e.TurnOff()
		}
	case *devices.EventSchedule:
		switch name {
		case "setActive":
			active, _ := args["active"].(bool)
			return e.SetActive(active)
		case "addRule":
			raw, _ := args["days"].([]any)
			days := make([]time.Weekday, len(raw))
			for i, d := range raw {
				n, _ := d.(json.Number).Int64()
				days[i] = time.Weekday(n)
			}
			return e.AddRule(int(whole("hour")), int(whole("minute")), days)
		case "deleteRule":
			return e.DeleteRule(whole("id"))
		case "enableRule":
			return e.EnableRule(whole("id"))
		case "disableRule":
			return e.DisableRule(whole("id"))
		}
	case *devices.TvRemote:
		if name == "press" {
			return e.Press(text("button"))
		}
	}
	return runnerErr("entity.intent: %s has no intent %s", e.Info().ID, name)
}
