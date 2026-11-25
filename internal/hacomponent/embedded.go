package hacomponent

import homeassistant "github.com/Adrient-tech/Adrient-Workspace/integrations/homeassistant"

// Embedded serves the integration compiled into this binary, zipped as
// custom_components/adrient/...
func Embedded() *Handler {
	return New(homeassistant.Component, homeassistant.ComponentRoot, "custom_components/adrient")
}
