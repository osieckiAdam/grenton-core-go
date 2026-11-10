// Command import-project reads the mobile interface from an Object Manager
// project file and lists its devices.
//
//	go run ./examples/import-project <project.omp> [interface-id]
//
// No PIN and no running Object Manager: the .omp you save in Object Manager
// already holds every mobile interface of the project, the CLUs and the
// encryption key. With one interface in the project the id can be omitted;
// with several, run once without it to see their ids.
//
// The key is never printed; the file contains it, so keep the file private.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/osieckiAdam/grenton-core-go/devices"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func main() {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Println("usage: import-project <project.omp> [interface-id]")
		os.Exit(2)
	}
	interfaceID := ""
	if len(os.Args) == 3 {
		interfaceID = os.Args[2]
	}

	// 1. Open the project: the archive and its XML are validated here.
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	project, err := provisioning.OpenProject(data)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("%d interface(s) in the project\n", len(project.Interfaces()))
	for _, entry := range project.Interfaces() {
		fmt.Printf("  %s  %s\n", entry.ID, entry.Name)
	}

	// 2. Import one of them: the same model.Interface a download gives.
	in, err := project.Interface(interfaceID)
	if errors.Is(err, provisioning.ErrInterfaceAmbiguous) {
		fmt.Println("\nseveral interfaces: pass the id of the one to import")
		os.Exit(2)
	} else if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("\nimported %q: %d CLU(s), %d page(s)\n", in.Name, len(in.Clus), len(in.Pages))

	// 3. From here on nothing depends on the source.
	mapping := devices.Map(in)
	fmt.Printf("%d devices\n", len(mapping.Devices))
	for _, d := range mapping.Devices {
		fmt.Printf("%s %q — page %s\n", d.Kind, d.Name, d.Page.Name)
	}
	for _, page := range mapping.Skipped {
		for _, widgetType := range page {
			if widgetType == nil {
				fmt.Println("skipped widget of type ?")
			} else {
				fmt.Println("skipped widget of type", *widgetType)
			}
		}
	}
}
