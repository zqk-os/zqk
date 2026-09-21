package paths

import "testing"

func TestNamedFilePermissionsMatchTheirNames(t *testing.T) {
	t.Parallel()

	if FilePerm600 != 0o600 {
		t.Errorf("FilePerm600=%#o, want 0600", FilePerm600)
	}
	if FilePerm644 != 0o644 {
		t.Errorf("FilePerm644=%#o, want 0644", FilePerm644)
	}
	if FilePerm600 == FilePerm644 {
		t.Error("secure and standard file permissions must not alias")
	}
}

func TestProcessDirectoryDerivation(t *testing.T) {
	t.Parallel()

	prefix := ProjectDataDir + "/"
	checks := map[string]string{
		"ProcessDir":                    ProcessDir,
		"ProcessInternalDir":            ProcessInternalDir,
		"ProcessInternalConfigsDir":     ProcessInternalConfigsDir,
		"ProcessInternalObjectSpecsDir": ProcessInternalObjectSpecsDir,
		"ProcessPoliciesDir":            ProcessPoliciesDir,
		"ProcessPlanningDir":            ProcessPlanningDir,
		"ProcessAccountsDir":            ProcessAccountsDir,
		"ProcessBacklogDir":             ProcessBacklogDir,
		"ProcessGoalsDir":               ProcessGoalsDir,
		"ProcessWorkstreamsDir":         ProcessWorkstreamsDir,
		"ProcessPriorityPlansDir":       ProcessPriorityPlansDir,
	}

	for name, path := range checks {
		if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
			t.Errorf("%s = %q; expected to start with %q", name, path, prefix)
		}
	}
}
