// Package homeassistant embeds the adrient Home Assistant custom integration
// (custom_components/adrient) so the Adrient server can hand it out; see
// internal/hacomponent. It lives next to the Python sources because go:embed
// cannot reach outside a package's directory.
package homeassistant

import "embed"

// Component holds custom_components/adrient. The all: prefix keeps
// underscore-prefixed files (__init__.py); build caches such as __pycache__
// are filtered out when the zip is built.
//
//go:embed all:custom_components/adrient
var Component embed.FS

// ComponentRoot is the directory inside Component that holds the integration.
const ComponentRoot = "custom_components/adrient"
