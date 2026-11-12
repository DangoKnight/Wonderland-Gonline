package app

import "wonderland-gonline/client/wlo/seui"

// Variant names are operation values, not executable expressions. Their schemas
// describe the parameters argument and are validated before queue admission.
func agentOperationSchemas(command string) map[string]seui.AutomationSchema {
	var samples map[string]any
	switch command {
	case "workspace.action":
		samples = map[string]any{
			"add": nil, "collapse": false,
			"switch": struct{ ID int }{}, "close": struct{ ID int }{},
			"title": struct {
				ID    int
				Title string
			}{},
		}
	case "housing.action":
		samples = map[string]any{
			"enter": struct{ Owner uint32 }{}, "exit": nil, "close": nil,
			"place": struct {
				Slot int
				Item uint16
				X, Y int
			}{},
			"move": struct {
				Index, Item uint16
				X, Y        int
			}{},
			"floorSwitch": struct{ Index, Item uint16 }{},
		}
	}
	if samples == nil {
		return nil
	}
	out := map[string]seui.AutomationSchema{}
	for name, sample := range samples {
		out[name] = seui.SchemaOf(sample)
	}
	return out
}
