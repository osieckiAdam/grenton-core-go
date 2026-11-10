// Command watch-state connects to the CLUs, prints every entity's state,
// then prints changes as they happen.
//
//	go run ./examples/watch-state [-any-source] <interface.zip> [seconds]
//
// Read-only: nothing is sent to the CLUs except subscriptions and pings.
// State is read from the entities, already normalized (a switch is on/off,
// a cover is a 0–100 position), never from raw CLU values. -any-source
// accepts reports from any address, for a CLU with several network
// interfaces that answers from another one.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/osieckiAdam/grenton-core-go"
	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
)

func main() {
	anySource := flag.Bool("any-source", false, "accept reports from any address")
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		fmt.Println("usage: watch-state [-any-source] <interface.zip> [seconds]")
		os.Exit(2)
	}
	seconds := 30
	if flag.NArg() == 2 {
		n, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			fmt.Println("bad number of seconds:", flag.Arg(1))
			os.Exit(2)
		}
		seconds = n
	}

	// Library warnings and errors (unreachable CLU, timeouts) go to stderr.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	archive, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sys, err := grenton.FromArchive(archive, devices.SystemConfig{
		Transport: clu.UDP{AnySource: *anySource},
		Logger:    logger,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Start returns after the initial registration, which delivers every
	// current value, so the snapshot below is complete. Stop sends
	// clientDestroy, so every CLU stops pushing at once.
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

	var entities []devices.Entity
	for _, d := range sys.Devices() {
		entities = append(entities, d.Entities...)
	}
	fmt.Printf("state of %d entities:\n", len(entities))
	for _, e := range entities {
		if reading := describe(e); reading != "" {
			fmt.Printf("  %s [%s]: %s\n", e.Info().Name, e.Info().ID, reading)
		}
	}

	// Run for the given time, or until Ctrl-C.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, cancelTimer := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancelTimer()

	// Subscribe after the snapshot, so the initial values are not printed
	// twice. Updates is a flag, not a history: when it fires, read the
	// entity again.
	changed := make(chan devices.Entity)
	for _, e := range entities {
		updates, stop := e.Updates()
		defer stop()
		go func() {
			for {
				select {
				case <-updates:
					select {
					case changed <- e:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	fmt.Printf("\nwatching for %d s…\n", seconds)
	for {
		select {
		case e := <-changed:
			fmt.Printf("Δ %s [%s]: %s\n", e.Info().Name, e.Info().ID, describe(e))
		case <-ctx.Done():
			return
		}
	}
}

// describe is a one-line reading of an entity's current state; "" for
// entities without state (buttons, scenes, cameras, TV remotes).
func describe(e devices.Entity) string {
	switch e := e.(type) {
	case *devices.Switch:
		return onOff(e.IsOn())
	case *devices.DimmableLight:
		return onOff(e.IsOn()) + ", brightness " + show(e.Brightness())
	case *devices.HsbLight:
		return onOff(e.IsOn()) + ", hue " + show(e.Hue()) + ", brightness " + show(e.Brightness())
	case *devices.ContactSensor:
		active, ok := e.IsActive()
		return pick(active, ok, "active", "inactive")
	case *devices.Sensor:
		return show(e.Value())
	case *devices.NumberSetting:
		return show(e.Value())
	case *devices.Cover:
		return "position " + show(e.Position()) + ", " + string(e.Motion())
	case *devices.StateSensor:
		return string(e.State())
	case *devices.EventSchedule:
		active, ok := e.IsActive()
		return pick(active, ok, "active", "stopped") + fmt.Sprintf(", %d rule(s)", len(e.Rules()))
	case *devices.Thermostat:
		return show(e.CurrentTemperature()) + " °C, target " + show(e.TargetTemperature()) + " °C"
	}
	return ""
}

func show[T any](v T, ok bool) string {
	if !ok {
		return "?"
	}
	return fmt.Sprint(v)
}

func onOff(on, ok bool) string { return pick(on, ok, "on", "off") }

func pick(b, ok bool, yes, no string) string {
	switch {
	case !ok:
		return "?"
	case b:
		return yes
	}
	return no
}
