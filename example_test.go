package grenton_test

import (
	"context"
	"fmt"
	"log"

	"github.com/osieckiAdam/grenton-core-go"
	"github.com/osieckiAdam/grenton-core-go/devices"
)

// The quick start of the README: download the interface while Object
// Manager's "send to mobile device" dialog is open, list the devices, read
// a switch and watch it.
func Example() {
	sys, err := grenton.FromObjectManager(context.Background(), "192.0.2.10", "149 853", devices.SystemConfig{})
	if err != nil {
		log.Fatal(err)
	}
	sys.Start()
	defer sys.Stop()

	for _, d := range sys.Devices() {
		fmt.Println(d.Name, d.Kind)
	}
	if lamp, ok := sys.Entity("w-onoff_0").(*devices.Switch); ok {
		on, known := lamp.IsOn()
		fmt.Println("lamp on:", on, known)
		updates, cancel := lamp.Updates()
		defer cancel()
		<-updates // the state changed; read it again
		fmt.Println(lamp.IsOn())
	}
}
