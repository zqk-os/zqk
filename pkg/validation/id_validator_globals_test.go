package validation

import "testing"

// restoreIDValidatorGlobals returns the process-global ID validator, prefix config, and paths
// config to their unseeded state when the test ends.
//
// These three are package singletons, so a test that points them at a t.TempDir and does not
// put them back leaves every later test in the binary resolving object specs against a
// directory the testing package has already deleted. That failure surfaces in an unrelated
// test, which is why it survived: TestSystemCheckScenario reported "object_specs: no such file
// or directory" naming TestGetIDValidatorWithEmptySpecsDir's temp root.
//
// This only registers cleanup. It deliberately does not reset on entry, so adding it to a test
// cannot change what that test observes while it runs.
func restoreIDValidatorGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ResetGlobalIDPrefixesConfig()
		ResetGlobalPathsConfig()
		ResetGlobalIDValidatorForTest()
	})
}
