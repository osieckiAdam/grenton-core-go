// Command set-thermostat sets a thermostat's target temperature and checks
// that the CLU applied it. It changes your house.
//
//	go run ./examples/set-thermostat <interface.zip> <object-name> <celsius>
//	go run ./examples/set-thermostat dist/interface.zip THE1001 21
//
// The thermostat is found by its CLU object (e.g. THE1001) through the
// physical objects, so it does not matter how many widgets show it. The
// previous target is read first and printed, so it can be restored.
// Nothing is written if the current target cannot be read.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/osieckiAdam/grenton-core-go"
	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("usage: set-thermostat <interface.zip> <object-name> <celsius>")
		os.Exit(2)
	}
	interfacePath, objectName := os.Args[1], os.Args[2]
	newTarget, err := strconv.ParseFloat(os.Args[3], 64)
	if err != nil {
		fmt.Println("bad temperature:", os.Args[3])
		os.Exit(2)
	}

	// 1. Load the interface saved from Object Manager (fetch-interface).
	// Library warnings and errors (unreachable CLU, timeouts) go to stderr.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	archive, err := os.ReadFile(interfacePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sys, err := grenton.FromArchive(archive, devices.SystemConfig{Logger: logger})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 2. Find the thermostat by its CLU object; one object may be shown by
	// several widgets, all operating the same thermostat.
	var thermostat *devices.Thermostat
	for _, o := range sys.PhysicalObjects() {
		if o.Key.Name != objectName || len(o.Widgets) == 0 || len(o.Widgets[0].EntityIDs) == 0 {
			continue
		}
		thermostat, _ = sys.Entity(o.Widgets[0].EntityIDs[0]).(*devices.Thermostat)
		var names []string
		for _, w := range o.Widgets {
			names = append(names, fmt.Sprintf("%q (page %s)", w.Name, w.Page.Name))
		}
		fmt.Printf("%s is shown as: %s\n", objectName, strings.Join(names, ", "))
		break
	}
	if thermostat == nil {
		fmt.Println("no thermostat for object", objectName)
		os.Exit(1)
	}
	updates, cancel := thermostat.Updates()
	defer cancel()

	// 3. Connect; the initial registration delivers the current values.
	sys.Start()
	defer sys.Stop()
	ready := false
	for _, id := range sys.CLUs() {
		state := sys.Session(id).State()
		fmt.Printf("CLU %s: %s\n", id, state)
		ready = ready || state == clu.StateReady
	}
	if !ready {
		fmt.Println("no CLU answered. Check that this computer is on the installation's network " +
			"(on macOS, that your terminal may use it: System Settings → Privacy & Security → Local Network).")
		return
	}
	if !waitUntil(updates, 10*time.Second, func() bool { _, ok := thermostat.TargetTemperature(); return ok }) {
		fmt.Println("current target not received within 10 s — nothing was written")
		return
	}
	previous, _ := thermostat.TargetTemperature()
	fmt.Println("before:", describe(thermostat))
	if previous == newTarget {
		fmt.Printf("the target is already %v °C — nothing to verify; pick another value\n", newTarget)
		return
	}

	// 4. Write the target and wait for the CLU to report it back.
	if err := thermostat.SetTargetTemperature(newTarget); err != nil {
		fmt.Println("the command failed:", err)
		return
	}
	applied := waitUntil(updates, 15*time.Second, func() bool {
		t, ok := thermostat.TargetTemperature()
		return ok && t == newTarget
	})
	fmt.Println("after: ", describe(thermostat))
	if applied {
		fmt.Printf("✓ the CLU applied %v °C\n", newTarget)
	} else {
		fmt.Println("✗ the target did not change within 15 s (mode not manual, or the value was rejected)")
	}
	fmt.Printf("to restore: go run ./examples/set-thermostat %s %s %v\n", interfacePath, objectName, previous)
}

// waitUntil re-checks condition on every update, until it holds or the
// time is up.
func waitUntil(updates <-chan struct{}, limit time.Duration, condition func() bool) bool {
	deadline := time.After(limit)
	for !condition() {
		select {
		case <-updates:
		case <-deadline:
			return condition()
		}
	}
	return true
}

func describe(t *devices.Thermostat) string {
	celsius := func(v float64, ok bool) string {
		if !ok {
			return "?"
		}
		return fmt.Sprintf("%v °C", v)
	}
	mode, ok := t.Mode()
	modeText := "?"
	if ok {
		modeText = string(mode)
	}
	on, ok := t.IsOn()
	onText := "?"
	if ok {
		onText = strconv.FormatBool(on)
	}
	return fmt.Sprintf("current %s, target %s, mode %s, on %s",
		celsius(t.CurrentTemperature()), celsius(t.TargetTemperature()), modeText, onText)
}
