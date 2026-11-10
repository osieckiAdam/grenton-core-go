// Command switch-light turns a switch or light on or off and confirms the
// CLU applied it. It changes your house.
//
//	go run ./examples/switch-light <interface.zip> <entity-id> on|off
//
// Entity ids come from list-devices. Works for switches, dimmers and RGB
// lights.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/osieckiAdam/grenton-core-go"
	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
)

// light is what switches, dimmers and RGB lights have in common.
type light interface {
	devices.Entity
	IsOn() (on, ok bool)
	TurnOn() error
	TurnOff() error
}

func main() {
	if len(os.Args) != 4 || (os.Args[3] != "on" && os.Args[3] != "off") {
		fmt.Println("usage: switch-light <interface.zip> <entity-id> on|off")
		os.Exit(2)
	}
	entityID, turnOn := os.Args[2], os.Args[3] == "on"

	// Library warnings and errors (unreachable CLU, timeouts) go to stderr.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	archive, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sys, err := grenton.FromArchive(archive, devices.SystemConfig{Logger: logger})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var lamp light
	switch e := sys.Entity(entityID).(type) {
	case nil:
		fmt.Println("no entity", entityID)
		os.Exit(1)
	case *devices.Switch:
		lamp = e
	case *devices.DimmableLight:
		lamp = e
	case *devices.HsbLight:
		lamp = e
	default:
		fmt.Println(entityID, "is not a switch or light")
		os.Exit(1)
	}

	sys.Start()
	defer sys.Stop()

	// Stop early when no CLU answered: every value would be unknown.
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
	fmt.Printf("%s: %s\n", lamp.Info().Name, state(lamp))

	// Subscribe before sending, so the confirmation cannot be missed.
	updates, cancel := lamp.Updates()
	defer cancel()
	send := lamp.TurnOff
	if turnOn {
		send = lamp.TurnOn
	}
	if err := send(); err != nil {
		fmt.Println("the command failed:", err)
		return
	}

	// The CLU pushes the new state back; wait up to 5 s for it.
	deadline := time.After(5 * time.Second)
	for {
		if on, ok := lamp.IsOn(); ok && on == turnOn {
			fmt.Printf("✓ %s is now %s\n", lamp.Info().Name, state(lamp))
			return
		}
		select {
		case <-updates:
		case <-deadline:
			fmt.Println("✗ no confirmation from the CLU within 5 s")
			return
		}
	}
}

func state(l light) string {
	on, ok := l.IsOn()
	switch {
	case !ok:
		return "unknown"
	case on:
		return "on"
	}
	return "off"
}
