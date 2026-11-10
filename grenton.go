// Package grenton is the umbrella of Grenton Core for Go: it gets an
// interface (from Object Manager, a saved archive or a project file) and
// makes the System that runs it. It holds no protocol rule; those live in
// the packages model, provisioning, clu and devices.
//
//	sys, err := grenton.FromObjectManager(ctx, "192.0.2.10", "149 853", devices.SystemConfig{})
//	if err != nil { … }
//	sys.Start()
//	defer sys.Stop()
//	for _, d := range sys.Devices() { … }
package grenton

import (
	"context"

	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

// FromObjectManager downloads the interface from Object Manager at host
// (port 9998) with the interface PIN, then makes its System. Object Manager
// serves it only while its "send to mobile device" dialog is open.
func FromObjectManager(ctx context.Context, host, pin string, cfg devices.SystemConfig) (*devices.System, error) {
	in, err := (&provisioning.Client{Host: host, Logger: cfg.Logger}).FetchInterface(ctx, pin)
	if err != nil {
		return nil, err
	}
	return devices.NewSystem(in, cfg)
}

// FromArchive reads a saved interface archive, then makes its System.
func FromArchive(archive []byte, cfg devices.SystemConfig) (*devices.System, error) {
	in, err := provisioning.ParseArchive(archive)
	if err != nil {
		return nil, err
	}
	return devices.NewSystem(in, cfg)
}

// FromProject imports an interface from an Object Manager project file
// (.omp), then makes its System. interfaceID chooses among several
// interfaces; "" takes the only one.
func FromProject(project []byte, interfaceID string, cfg devices.SystemConfig) (*devices.System, error) {
	p, err := provisioning.OpenProject(project)
	if err != nil {
		return nil, err
	}
	in, err := p.Interface(interfaceID)
	if err != nil {
		return nil, err
	}
	return devices.NewSystem(in, cfg)
}
