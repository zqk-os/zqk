package main

import "github.com/zqk-os/zqk/cmd/zqk/object"

const emptyValue = ""

// Isolating TEST_ROOT is not enough: the child inherits HOME and authenticates as
// the developer's live session, then fails resolving that ZQK-* id against the
// empty temp store. Same contract as object.wireExecForTest / EnvWithTestRoot.
func envForIsolatedCLIProject(tmpRoot string) []string {
	return object.EnvWithTestRoot(tmpRoot)
}
