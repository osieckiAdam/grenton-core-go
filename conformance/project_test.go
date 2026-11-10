package conformance_test

import (
	"github.com/osieckiAdam/grenton-core-go/model"
	"github.com/osieckiAdam/grenton-core-go/provisioning"
)

func init() {
	registry["provisioning.omp"] = provisioningOmp
	registry["provisioning.archive"] = provisioningArchive
}

func provisioningOmp(s *suite, input map[string]any) (any, error) {
	data, err := readFixture(s, input)
	if err != nil {
		return nil, err
	}
	id := ""
	if _, has := input["interfaceId"]; has {
		if id, err = str(input, "interfaceId"); err != nil {
			return nil, err
		}
	}
	project, err := provisioning.OpenProject(data)
	if err != nil {
		return nil, err
	}
	in, err := project.Interface(id)
	if err != nil {
		return nil, err
	}
	list := []any{}
	for _, info := range project.Interfaces() {
		list = append(list, map[string]any{"id": info.ID, "name": info.Name})
	}
	return ok(map[string]any{"interfaces": list, "interface": interfaceJSON(in), "mapping": mappingJSON(in)}), nil
}

func provisioningArchive(s *suite, input map[string]any) (any, error) {
	data, err := readFixture(s, input)
	if err != nil {
		return nil, err
	}
	in, err := provisioning.ParseArchive(data)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"interface": interfaceJSON(in), "mapping": mappingJSON(in)}), nil
}

// interfaceJSON is the root projection of conformance.md §4c.
func interfaceJSON(in *model.Interface) map[string]any {
	clus := []any{}
	for _, c := range in.Clus {
		clus = append(clus, map[string]any{"id": c.ID, "name": c.Name, "ip": c.IP, "serialNumber": c.SerialNumber,
			"port": c.Port, "connectionType": string(c.ConnectionType)})
	}
	pushes := []any{}
	for _, p := range in.PushNotifications {
		pushes = append(pushes, map[string]any{"cluId": p.CluID, "objectId": p.ObjectID, "objectType": p.ObjectType})
	}
	pages := []any{}
	for _, p := range in.Pages {
		widgets := []any{}
		for _, it := range p.Items {
			if it.Header != nil {
				widgets = append(widgets, map[string]any{"id": it.Header.ID, "type": "HEADER"})
			} else {
				widgets = append(widgets, map[string]any{"id": it.Widget.WidgetID(), "type": it.Widget.WidgetType()})
			}
		}
		pages = append(pages, map[string]any{"name": p.Name, "icon": p.Icon, "isFullscreenWidget": p.IsFullscreenWidget,
			"widgets": widgets})
	}
	return map[string]any{"id": in.ID, "version": in.Version, "name": in.Name, "icon": in.Icon, "theme": in.Theme,
		"encryption": map[string]any{"key": in.Encryption.Key, "iv": in.Encryption.IV},
		"clus":       clus, "pushNotifications": pushes, "pages": pages}
}
