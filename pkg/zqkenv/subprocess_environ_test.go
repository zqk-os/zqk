package zqkenv

import (
	"strings"
	"testing"
)

func TestSubprocessEnvironWithTestRoot_stripsProjectRootAndTestDataDir(t *testing.T) {
	const isolated = "/tmp/zqk-isolated-test-root"
	t.Setenv(ProjectRoot(), "/real/workspace")
	t.Setenv(TestRoot(), "/wrong-previous-root")
	t.Setenv(TestDataDir(), "/wrong-process-data")
	env := SubprocessEnvironWithTestRoot(isolated)
	var sawProject, sawDataDir, countChildTestRoot int
	childPrefix := subprocessChildTestRootEnvKey + "="
	for _, e := range env {
		switch {
		case strings.HasPrefix(e, ProjectRoot()+"="):
			sawProject++
		case strings.HasPrefix(e, TestDataDir()+"="):
			sawDataDir++
		case strings.HasPrefix(e, childPrefix):
			countChildTestRoot++
			if want := childPrefix + isolated; e != want {
				t.Errorf("child TEST_ROOT entry: got %q want %q", e, want)
			}
		}
	}
	if sawProject != 0 || sawDataDir != 0 {
		t.Fatalf("expected PROJECT_ROOT and TEST_DATA_DIR stripped from subprocess env")
	}
	if countChildTestRoot != 1 {
		t.Fatalf("expected exactly one %s in subprocess env, got %d", subprocessChildTestRootEnvKey, countChildTestRoot)
	}
}

func TestSubprocessEnvironWithTestRoot_stripsInheritedZQKProjectRootAndTestDataDir(t *testing.T) {
	const isolated = "/tmp/zqk-isolated-inherited-zqk-keys"
	// Simulate inherited shell/IDE ZQK_* while the test binary's ProjectRoot() is a different key.
	t.Setenv(subprocessChildZQKProjectRootEnvKey, "/real/workspace-from-shell")
	t.Setenv(subprocessChildZQKTestDataDirEnvKey, "/wrong-zqk-test-data")
	env := SubprocessEnvironWithTestRoot(isolated)
	for _, e := range env {
		if strings.HasPrefix(e, subprocessChildZQKProjectRootEnvKey+"=") {
			t.Fatalf("ZQK_PROJECT_ROOT should be stripped: %q", e)
		}
		if strings.HasPrefix(e, subprocessChildZQKTestDataDirEnvKey+"=") {
			t.Fatalf("ZQK_TEST_DATA_DIR should be stripped: %q", e)
		}
	}
}

func TestSubprocessEnvironWithTestRootAndExtras_replacesSameKey(t *testing.T) {
	const isolated = "/tmp/zqk-isolated-extras"
	t.Setenv(GraphEnabled(), "false")
	env := SubprocessEnvironWithTestRootAndExtras(isolated, GraphEnabled()+"=true")
	var graphCount int
	for _, e := range env {
		if strings.HasPrefix(e, GraphEnabled()+"=") {
			graphCount++
			if e != GraphEnabled()+"=true" {
				t.Errorf("graph entry: %q", e)
			}
		}
	}
	if graphCount != 1 {
		t.Fatalf("expected exactly one GRAPH_ENABLED, got %d", graphCount)
	}
}
