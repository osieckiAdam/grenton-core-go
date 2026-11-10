// Command run-scene runs a scene from the mobile interface. It changes
// your house.
//
//	go run ./examples/run-scene <interface.zip> <scene>
//	go run ./examples/run-scene dist/interface.zip "Return to base"
//
// <scene> is the scene's name as shown in the app, or its entity id
// (list-devices prints both). A scene is a script on a CLU; running it does
// whatever the script does (lights, blinds, a vacuum), so choose one whose
// effect you want. The CLU answers when it has started the script; a scene
// has no state to read back.
package main

import (
	"fmt"
	"os"

	"github.com/osieckiAdam/grenton-core-go"
	"github.com/osieckiAdam/grenton-core-go/clu"
	"github.com/osieckiAdam/grenton-core-go/devices"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: run-scene <interface.zip> <scene name or entity id>")
		os.Exit(2)
	}
	archive, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sys, err := grenton.FromArchive(archive, devices.SystemConfig{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// 1. Scenes are entities like any other; find one by name or id.
	var scenes []*devices.SceneTrigger
	for _, d := range sys.Devices() {
		for _, e := range d.Entities {
			if s, ok := e.(*devices.SceneTrigger); ok {
				scenes = append(scenes, s)
			}
		}
	}
	var scene *devices.SceneTrigger
	for _, s := range scenes {
		if s.Info().Name == os.Args[2] || s.Info().ID == os.Args[2] {
			scene = s
			break
		}
	}
	if scene == nil {
		fmt.Printf("no scene named %q. Scenes in this interface:\n", os.Args[2])
		for _, s := range scenes {
			fmt.Printf("  %s  %s\n", s.Info().ID, s.Info().Name)
		}
		os.Exit(2)
	}

	// 2. Connect: the scene's CLU must answer before a command can be sent.
	sys.Start()
	defer sys.Stop()
	ready := false
	for _, id := range sys.CLUs() {
		ready = ready || sys.Session(id).State() == clu.StateReady
	}
	if !ready {
		fmt.Println("no CLU answered; is this computer on the installation's network?")
		return
	}

	// 3. Run it. The call returns once the CLU acknowledged the script.
	if err := scene.Run(); err != nil {
		fmt.Printf("%q failed: %v\n", scene.Info().Name, err)
		return
	}
	fmt.Printf("ran %q\n", scene.Info().Name)
}
