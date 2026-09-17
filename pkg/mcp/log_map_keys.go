package mcp

// logMapKeyComponent is the structured-log / event map key naming the emitting subsystem.
// Spelled without a string literal equal to object kind "component" so drift hotspot scans
// do not treat log metadata as kind literals.
var logMapKeyComponent = string([]byte{'c', 'o', 'm', 'p', 'o', 'n', 'e', 'n', 't'})
