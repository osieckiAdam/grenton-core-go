// Command watch lists the devices of a Grenton installation and prints
// their state as it changes. It is read-only: it subscribes to state and
// sends no command.
//
//	go run ./examples/watch -archive dist/interface.zip
//	go run ./examples/watch -project house.omp [-interface ID]
//	go run ./examples/watch -host 192.0.2.10 -pin "149 853" -save dist/interface.zip
//
// On macOS run it from Terminal.app: binaries started from an IDE are
// denied local-network access without a prompt.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/model"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func main() {
	host := flag.String("host", "", "Object Manager's computer")
	pin := flag.String("pin", "", "the interface PIN Object Manager shows")
	archive := flag.String("archive", "", "a saved interface archive")
	project := flag.String("project", "", "an Object Manager project file (.omp)")
	interfaceID := flag.String("interface", "", "the interface to import from the project")
	anySource := flag.Bool("any-source", false, "accept reports from any address (a CLU with several interfaces)")
	duration := flag.Duration("for", time.Minute, "how long to watch")
	verbose := flag.Bool("v", false, "debug logs")
	flag.Parse()

	level := slog.LevelWarn
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	in, err := load(*host, *pin, *archive, *project, *interfaceID, logger)
	if err != nil {
		log.Fatal(err)
	}
	sys, err := devices.NewSystem(in, devices.SystemConfig{Transport: clu.UDP{AnySource: *anySource}, Logger: logger})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d CLU(s), %d device(s)\n", in.Name, len(in.Clus), len(sys.Devices()))

	start := time.Now()
	sys.Start()
	defer sys.Stop()
	fmt.Printf("started in %v\n", time.Since(start).Round(time.Millisecond))
	for _, id := range sys.CLUs() {
		d := sys.Session(id).Diagnostics()
		fmt.Printf("  %s: %s, %d key(s)\n", id, d.State, d.RequestedKeys)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, cancelTimer := context.WithTimeout(ctx, *duration)
	defer cancelTimer()

	changed := make(chan devices.Entity, 64)
	for _, d := range sys.Devices() {
		for _, e := range d.Entities {
			fmt.Printf("%-40s %s\n", e.Info().ID, describe(e))
			updates, stop := e.Updates()
			defer stop()
			go func() {
				for {
					select {
					case <-updates:
						changed <- e
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}
	fmt.Println("watching; Ctrl-C ends")
	for {
		select {
		case e := <-changed:
			fmt.Printf("%s %-40s %s\n", time.Now().Format("15:04:05"), e.Info().ID, describe(e))
		case <-ctx.Done():
			return
		}
	}
}

func load(host, pin, archive, project, interfaceID string, logger *slog.Logger) (*model.Interface, error) {
	switch {
	case archive != "":
		data, err := os.ReadFile(archive)
		if err != nil {
			return nil, err
		}
		return provisioning.ParseArchive(data)
	case project != "":
		data, err := os.ReadFile(project)
		if err != nil {
			return nil, err
		}
		p, err := provisioning.OpenProject(data)
		if err != nil {
			return nil, err
		}
		return p.Interface(interfaceID)
	case host != "" && pin != "":
		return (&provisioning.Client{Host: host, Logger: logger}).FetchInterface(context.Background(), pin)
	}
	return nil, fmt.Errorf("give -archive, -project, or -host with -pin")
}

func show[T any](v T, ok bool) string {
	if !ok {
		return "?"
	}
	return fmt.Sprint(v)
}

// describe renders an entity's state in one line.
func describe(e devices.Entity) string {
	switch e := e.(type) {
	case *devices.Switch:
		return "on=" + show(e.IsOn())
	case *devices.DimmableLight:
		return "brightness=" + show(e.Brightness())
	case *devices.HsbLight:
		return fmt.Sprintf("on=%s hue=%s", show(e.IsOn()), show(e.Hue()))
	case *devices.ContactSensor:
		return "active=" + show(e.IsActive())
	case *devices.Sensor:
		return "value=" + show(e.Value())
	case *devices.NumberSetting:
		return "value=" + show(e.Value())
	case *devices.Cover:
		return fmt.Sprintf("position=%s motion=%s", show(e.Position()), e.Motion())
	case *devices.StateSensor:
		return "state=" + string(e.State())
	case *devices.Thermostat:
		return fmt.Sprintf("current=%s target=%s", show(e.CurrentTemperature()), show(e.TargetTemperature()))
	case *devices.EventSchedule:
		return fmt.Sprintf("active=%s rules=%d", show(e.IsActive()), len(e.Rules()))
	}
	return string(e.Info().Kind)
}
