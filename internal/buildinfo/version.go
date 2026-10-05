package buildinfo

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version is shared by the desktop footer, update checks, and release packaging.
func Version() string { return strings.TrimSpace(version) }
