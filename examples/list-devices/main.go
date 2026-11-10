// Command list-devices lists the devices of an installation, offline.
//
//	go run ./examples/list-devices <interface.zip>
//
// Mapping needs no network: devices come from the interface alone. Each
// widget is one device with its entities. The physical objects group the
// widgets that operate the same CLU object, such as a thermostat shown on
// two pages under two names.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: list-devices <interface.zip>")
		os.Exit(2)
	}
	archive, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	in, err := provisioning.ParseArchive(archive)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	mapping := devices.Map(in)

	// Devices: one per widget, in page order.
	fmt.Printf("%d devices\n", len(mapping.Devices))
	for _, d := range mapping.Devices {
		fmt.Printf("%s %q — page %s\n", d.Kind, d.Name, d.Page.Name)
		for _, e := range d.Entities {
			fmt.Printf("  %s: %s\n", e.Info().ID, e.Info().Name)
		}
	}

	// Widgets the library could not read (unknown type or broken data).
	for _, page := range mapping.Skipped {
		for _, widgetType := range page {
			if widgetType == nil {
				fmt.Println("skipped widget of type ?")
			} else {
				fmt.Println("skipped widget of type", *widgetType)
			}
		}
	}

	// Physical objects shown by more than one widget.
	var shared []devices.PhysicalObject
	for _, o := range mapping.PhysicalObjects {
		if len(o.Widgets) > 1 {
			shared = append(shared, o)
		}
	}
	fmt.Printf("\n%d objects shown by several widgets\n", len(shared))
	for _, o := range shared {
		var names []string
		for _, w := range o.Widgets {
			names = append(names, fmt.Sprintf("%q (page %s)", w.Name, w.Page.Name))
		}
		fmt.Printf("%s: %s\n", o.Key.Name, strings.Join(names, ", "))
	}
}
