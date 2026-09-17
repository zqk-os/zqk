package lifecyclespec

import "embed"

// EmbeddedDefinitions contains all lifecycle YAML specs.  The registry package
// uses this embedded filesystem to load definitions at runtime.
// Lifecycle YAML files live alongside this file (no separate built-in subdirectory).
//
//go:embed *.yaml
var EmbeddedDefinitions embed.FS
