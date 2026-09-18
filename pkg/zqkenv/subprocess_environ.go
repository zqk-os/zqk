package zqkenv

import (
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/brand"
)

// subprocessChildTestRootEnvKey is the TEST_ROOT variable name read by the default CLI binary
// (argv basename "zqk" → brand env prefix ZQK). Child processes must use this key, not [TestRoot]
// of the parent process: under "go test", argv is e.g. object.test so [TestRoot] would be
// OBJECT_TEST_TEST_ROOT while the spawned ./zqk binary only honors ZQK_TEST_ROOT.
var subprocessChildTestRootEnvKey = brand.DefaultEnvPrefix + "_TEST_ROOT"

// subprocessChildZQKProjectRootEnvKey and subprocessChildZQKTestDataDirEnvKey are the env keys
// read by the default binary (ZQK prefix). The parent "go test" process uses a different
// brand prefix ([ProjectRoot] / [TestDataDir] are e.g. SCHEDULER_TEST_PROJECT_ROOT), so inherited
// shell or IDE ZQK_PROJECT_ROOT / ZQK_TEST_DATA_DIR would not be removed by the tr/pr/td strip
// above and would make the child resolve the real workspace instead of the isolated test root.
var (
	subprocessChildZQKProjectRootEnvKey = brand.DefaultEnvPrefix + "_PROJECT_ROOT"
	subprocessChildZQKTestDataDirEnvKey = brand.DefaultEnvPrefix + "_TEST_DATA_DIR"
)

// SubprocessEnvironWithTestRoot returns a copy of [os.Environ] for spawning a zqk CLI subprocess
// against an isolated temp project. It removes every entry for [TestRoot], [ProjectRoot], and
// [TestDataDir], then appends ZQK_TEST_ROOT=testRoot (see [subprocessChildTestRootEnvKey]).
//
// Rationale: [internal/cli/context.ResolveProjectRoot] prefers PROJECT_ROOT over TEST_ROOT. Tests
// that only append TEST_ROOT to the parent environment leave PROJECT_ROOT set (from CI, IDE, or
// shell), so the child still resolves the real workspace and writes under .zqk/process/.
// Inherited TEST_DATA_DIR can likewise point subprocesses at the wrong tree ([pkg/testing.GetTestConfig]).
func SubprocessEnvironWithTestRoot(testRoot string) []string {
	tr := TestRoot().Name() + "="
	pr := ProjectRoot().Name() + "="
	td := TestDataDir().Name() + "="
	childTR := subprocessChildTestRootEnvKey + "="
	zqkPR := subprocessChildZQKProjectRootEnvKey + "="
	zqkTD := subprocessChildZQKTestDataDirEnvKey + "="
	out := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, tr) || strings.HasPrefix(e, pr) || strings.HasPrefix(e, td) {
			continue
		}
		if strings.HasPrefix(e, zqkPR) || strings.HasPrefix(e, zqkTD) {
			continue
		}
		if strings.HasPrefix(e, childTR) {
			continue
		}
		pfx := brand.EnvPrefix()
		if strings.HasPrefix(e, "ZQK_SESSION=") || strings.HasPrefix(e, "ZQK_TEST_SESSION=") ||
			(pfx != brand.DefaultEnvPrefix && (strings.HasPrefix(e, pfx+"_SESSION=") || strings.HasPrefix(e, pfx+"_TEST_SESSION="))) {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "ZQK_TEST_ROOT="+testRoot)
	out = append(out, "ZQK_ADMIN_TEST_ROOT="+testRoot)
	out = append(out, "ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1")
	out = append(out, "ZQK_ADMIN_TEST_ALLOW_CAS_FALLTHROUGH=1")
	out = append(out, "ZQK_TEST_BYPASS_AUTH=1")
	unreachableSocket := UnreachableTestSocketPath()
	out = append(out, "ZQK_PRIVILEGED_WRITER_SOCKET="+unreachableSocket)
	out = append(out, "ZQK_ADMIN_PRIVILEGED_WRITER_SOCKET="+unreachableSocket)
	out = append(out, "ZQK_MOCK_GRAPH=false")
	out = append(out, "ZQK_ADMIN_MOCK_GRAPH=false")
	out = append(out, "MOCK_GRAPH=false")
	out = append(out, "ZQK_GRAPH_ENABLED=false")
	out = append(out, "ZQK_ADMIN_GRAPH_ENABLED=false")
	// AllowCIOverrides removed (PRI-ENV-SIGNED-LOGIN-001). Do not reintroduce CI override inject.
	// Isolated CLI tests that need lifecycle motion must use promote/demote, not --override.

	if tr != "ZQK_TEST_ROOT=" && tr != "ZQK_ADMIN_TEST_ROOT=" {
		out = append(out, TestRoot().Name()+"="+testRoot)
	}
	return out
}

// WireExecForIsolatedProject sets cmd.Dir and cmd.Env so a zqk child process uses projectRoot
// and cannot inherit a conflicting ZQK_PROJECT_ROOT / ZQK_TEST_DATA_DIR from the parent.
func WireExecForIsolatedProject(cmd *exec.Cmd, projectRoot string) {
	if cmd == nil {
		return
	}
	cmd.Dir = projectRoot
	initGoEnv()
	var extras []string
	if goPathCache != "" {
		extras = append(extras, "GOPATH="+goPathCache)
	}
	if goCacheCache != "" {
		extras = append(extras, "GOCACHE="+goCacheCache)
	}
	cmd.Env = SubprocessEnvironWithTestRootAndExtras(projectRoot, extras...)
}

// SubprocessEnvironWithTestRootAndExtras is like [SubprocessEnvironWithTestRoot] but merges
// extra KEY=value pairs, replacing any prior environment entry with the same key.
func SubprocessEnvironWithTestRootAndExtras(testRoot string, extras ...string) []string {
	env := SubprocessEnvironWithTestRoot(testRoot)
	for _, kv := range extras {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			continue
		}
		env = stripEnvKeyWithPrefix(env, key)
		env = append(env, kv)
	}
	return env
}

// WireExecForIsolatedProjectWithExtras is [WireExecForIsolatedProject] plus merged extras
// (e.g. ZQK_GRAPH_ENABLED=true) with same-key replacement.
func WireExecForIsolatedProjectWithExtras(cmd *exec.Cmd, projectRoot string, extras ...string) {
	if cmd == nil {
		return
	}
	cmd.Dir = projectRoot
	initGoEnv()
	if goPathCache != "" {
		extras = append(extras, "GOPATH="+goPathCache)
	}
	if goCacheCache != "" {
		extras = append(extras, "GOCACHE="+goCacheCache)
	}
	cmd.Env = SubprocessEnvironWithTestRootAndExtras(projectRoot, extras...)
}

func stripEnvKeyWithPrefix(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

var (
	goPathCache   string
	goCacheCache  string
	goEnvInitOnce sync.Once
)

func initGoEnv() {
	goEnvInitOnce.Do(func() {
		if out, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
			goPathCache = strings.TrimSpace(string(out))
		}
		if out, err := exec.Command("go", "env", "GOCACHE").Output(); err == nil {
			goCacheCache = strings.TrimSpace(string(out))
		}
	})
}
