package system

import "testing"

func TestDefaultHostDaemons_ambientDisabledInTest(t *testing.T) {
	root := t.TempDir()
	units := defaultHostDaemons(root, nil)
	if len(units) != 2 {
		t.Fatalf("units=%d want 2", len(units))
	}
	var ambientUnit *hostDaemonUnit
	for i := range units {
		if units[i].ID == hostDaemonIDAmbient {
			ambientUnit = &units[i]
			break
		}
	}
	if ambientUnit == nil || ambientUnit.Enabled == nil {
		t.Fatal("ambient unit must declare Enabled")
	}
	if ambientUnit.Enabled(root) {
		t.Fatal("ambient host daemon must stay off inside go test")
	}
	logHostDaemonReport(nil, "test", hostDaemonReport{
		Skipped: []string{hostDaemonIDAmbient},
	})
}
