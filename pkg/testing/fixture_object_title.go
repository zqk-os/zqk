package testing

import "fmt"

// FixtureObjectTitle returns a title for synthetic objects created in isolated test
// storage only (t.TempDir + ZQK_TEST_ROOT / SetupTestEnvironment). It must not be used
// for creates against the real project’s docs/process—test data belongs only under a
// dedicated test root. The "Fixture:" prefix is a readable label inside those temp
// trees; if such titles ever appear in production data, delete those objects and fix
// the test or run that wrote without isolation.
func FixtureObjectTitle(kind string, seq int) string {
	return fmt.Sprintf("Fixture: %s #%d", kind, seq)
}
