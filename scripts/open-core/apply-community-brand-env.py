#!/usr/bin/env python3
"""Apply community brand-env hygiene against zqk-public-candidate. Idempotent."""
from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent.parent


def read(rel: str) -> str:
    return (ROOT / rel).read_text()


def write(rel: str, text: str) -> None:
    path = ROOT / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
    print("write", rel)


def sub_all(rel: str, old: str, new: str) -> None:
    path = ROOT / rel
    if not path.exists():
        print("missing", rel, file=sys.stderr)
        return
    text = path.read_text()
    if old not in text:
        return
    path.write_text(text.replace(old, new))
    print("patch", rel)


def rm(rel: str) -> None:
    path = ROOT / rel
    if path.exists():
        path.unlink()
        print("rm", rel)


BRAND_KEYS = r'''package zqkenv

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

func DefaultBrandKey(suffix string) string {
	suffix = strings.TrimPrefix(suffix, "_")
	return brand.DefaultEnvPrefix + "_" + suffix
}

func AdminBrandKey(suffix string) string {
	suffix = strings.TrimPrefix(suffix, "_")
	return brand.DefaultEnvPrefix + "_ADMIN_" + suffix
}

func DefaultAssignment(suffix, value string) string {
	return DefaultBrandKey(suffix) + "=" + value
}

func AdminAssignment(suffix, value string) string {
	return AdminBrandKey(suffix) + "=" + value
}

func HasDefaultAssignment(entry, suffix string) bool {
	return strings.HasPrefix(entry, DefaultBrandKey(suffix)+"=")
}

func IsProductPrefixed(entry string) bool {
	cur := brand.EnvPrefix() + "_"
	def := brand.DefaultEnvPrefix + "_"
	if strings.HasPrefix(entry, cur) {
		return true
	}
	return cur != def && strings.HasPrefix(entry, def)
}

func Airgap() EnvVar               { return EnvVar{Key: brand.EnvVar("AIRGAP")} }
func AllowDegraded() EnvVar        { return EnvVar{Key: brand.EnvVar("ALLOW_DEGRADED")} }
func BypassHandslapper() EnvVar    { return EnvVar{Key: brand.EnvVar("BYPASS_HANDSLAPPER")} }
func Codegen() EnvVar              { return EnvVar{Key: brand.EnvVar("CODEGEN")} }
func ContextProfile() EnvVar       { return EnvVar{Key: brand.EnvVar("CONTEXT_PROFILE")} }
func DebugOperations() EnvVar      { return EnvVar{Key: brand.EnvVar("DEBUG_OPERATIONS")} }
func DevCodegen() EnvVar           { return EnvVar{Key: brand.EnvVar("DEV_CODEGEN")} }
func DisableLocalOllama() EnvVar   { return EnvVar{Key: brand.EnvVar("DISABLE_LOCAL_OLLAMA")} }
func DistDir() EnvVar              { return EnvVar{Key: brand.EnvVar("DIST_DIR")} }
func ForceLocalOllama() EnvVar     { return EnvVar{Key: brand.EnvVar("FORCE_LOCAL_OLLAMA")} }
func InfoOperations() EnvVar       { return EnvVar{Key: brand.EnvVar("INFO_OPERATIONS")} }
func LogLevel() EnvVar             { return EnvVar{Key: brand.EnvVar("LOG_LEVEL")} }
func NonInteractive() EnvVar       { return EnvVar{Key: brand.EnvVar("NON_INTERACTIVE")} }
func ObserverConcurrency() EnvVar  { return EnvVar{Key: brand.EnvVar("OBSERVER_CONCURRENCY")} }
func ObserverIncludeTests() EnvVar { return EnvVar{Key: brand.EnvVar("OBSERVER_INCLUDE_TESTS")} }
func ObserverSkipIntent() EnvVar   { return EnvVar{Key: brand.EnvVar("OBSERVER_SKIP_INTENT")} }
func Profile() EnvVar              { return EnvVar{Key: brand.EnvVar("PROFILE")} }
func TaskID() EnvVar               { return EnvVar{Key: brand.EnvVar("TASK_ID")} }
func Timeout() EnvVar              { return EnvVar{Key: brand.EnvVar("TIMEOUT")} }
func Env() EnvVar                  { return EnvVar{Key: brand.EnvVar("ENV")} }
'''

PREFIX_SH = r'''#!/bin/sh
if [ -z "${ZQK_BRAND_ENV_PREFIX_ROOT:-}" ]; then
	_brand_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
else
	_brand_root=$ZQK_BRAND_ENV_PREFIX_ROOT
fi
BRAND_CONFIG=
if [ -f "$_brand_root/config/zqk-local.yaml" ]; then
	BRAND_CONFIG="$_brand_root/config/zqk-local.yaml"
elif [ -f "$_brand_root/config/zqk.yaml" ]; then
	BRAND_CONFIG="$_brand_root/config/zqk.yaml"
fi
BRAND_EXE=zqk
if [ -n "$BRAND_CONFIG" ]; then
	_parsed=$(awk '/^[[:space:]]*executable_name:/{gsub(/["\047]/, "", $2); print $2; exit}' "$BRAND_CONFIG")
	if [ -n "$_parsed" ]; then
		BRAND_EXE=$_parsed
	fi
fi
BRAND_ENV_PREFIX=$(printf '%s' "$BRAND_EXE" | tr 'abcdefghijklmnopqrstuvwxyz-' 'ABCDEFGHIJKLMNOPQRSTUVWXYZ_')
if [ -z "$BRAND_ENV_PREFIX" ]; then
	BRAND_ENV_PREFIX=ZQK
fi
unset _brand_root _parsed
'''


def main() -> int:
    rm("pkg/zqkenv/agent_guard.go")
    rm("pkg/zqkenv/agent_guard_test.go")
    rm("pkg/zqkenv/agentguard/agent_guard.go")
    rm("pkg/zqkenv/agentguard/agent_guard_test.go")
    write("pkg/zqkenv/brand_keys.go", BRAND_KEYS)
    write("scripts/open-core/brand-env-prefix.sh", PREFIX_SH)
    write("scripts/open-core/apply-community-brand-env.py", pathlib.Path(__file__).read_text())

    sub_all(
        "cmd/zqk/main.go",
        '\t"github.com/zqk-os/zqk/pkg/zqkenv"\n\t_ "github.com/zqk-os/zqk/pkg/zqkenv/agentguard"\n)',
        '\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)',
    )

    vars = read("pkg/zqkenv/vars.go")
    vars = vars.replace('const _sfxAllowForegroundGoTest = "ALLOW_FOREGROUND_GO_TEST"\n', "")
    vars = re.sub(
        r"\n// AllowForegroundGoTest[^\n]*\n(?://[^\n]*\n)*func AllowForegroundGoTest\(\) EnvVar \{ return EnvVar\{Key: brand\.EnvVar\(_sfxAllowForegroundGoTest\)\} \}\n",
        "\n",
        vars,
    )
    vars = vars.replace('\t_rawZqkGraphEnabledLegacy    = "ZQK_GRAPH_ENABLED"\n', "")
    vars = vars.replace(
        "func ZqkGraphEnabledLegacy() EnvVar { return EnvVar{Key: _rawZqkGraphEnabledLegacy} }",
        'func ZqkGraphEnabledLegacy() EnvVar { return EnvVar{Key: DefaultBrandKey("GRAPH_ENABLED")} }',
    )
    vars = vars.replace(
        'const _sfxZqkShimBypassPolCode009 = "ZQK_SHIM_BYPASS_POLCODE009" //nolint:gosec',
        'const _sfxZqkShimBypassTraceability = "SHIM_BYPASS_TRACEABILITY" //nolint:gosec',
    )
    vars = vars.replace("func ZqkShimBypassPolCode009()", "func ZqkShimBypassTraceability()")
    vars = vars.replace("_sfxZqkShimBypassPolCode009", "_sfxZqkShimBypassTraceability")
    vars = vars.replace(
        'func ZQKAllowForegroundGoTest() EnvVar   { return EnvVar{Key: "ZQK_ALLOW_FOREGROUND_GO_TEST"} }\nfunc ZqkEnv() EnvVar                     { return EnvVar{Key: "ZQK_ENV"} }\nfunc ZQKProjectRoot() EnvVar             { return EnvVar{Key: brand.DefaultEnvPrefix + "_PROJECT_ROOT"} }\nfunc ZQKTestRoot() EnvVar                { return EnvVar{Key: brand.DefaultEnvPrefix + "_TEST_ROOT"} }',
        'func ZqkEnv() EnvVar                     { return Env() }\nfunc ZQKProjectRoot() EnvVar             { return EnvVar{Key: DefaultBrandKey("PROJECT_ROOT")} }\nfunc ZQKTestRoot() EnvVar                { return EnvVar{Key: DefaultBrandKey("TEST_ROOT")} }',
    )
    write("pkg/zqkenv/vars.go", vars)

    envvar = read("pkg/zqkenv/envvar.go")
    envvar = envvar.replace(
        'fallbackKey := brand.DefaultEnvPrefix + "_" + strings.TrimPrefix(e.Key, pfx+"_")',
        'fallbackKey := DefaultBrandKey(strings.TrimPrefix(e.Key, pfx+"_"))',
    )
    write("pkg/zqkenv/envvar.go", envvar)

    prop = read("pkg/config/property.go")
    if '"github.com/zqk-os/zqk/pkg/brand"' not in prop:
        prop = prop.replace(
            '\t"fmt"\n\t"os"\n\t"strings"\n)',
            '\t"fmt"\n\t"os"\n\t"strings"\n\n\t"github.com/zqk-os/zqk/pkg/brand"\n)',
        )
    prop = prop.replace('return "ZQK_" + strings.ToUpper(result.String())', "return brand.EnvVar(strings.ToUpper(result.String()))")
    write("pkg/config/property.go", prop)

    ci = read(".github/workflows/ci.yml")
    ci = ci.replace("          export ZQK_ALLOW_FOREGROUND_GO_TEST=1\n", "")
    ci = ci.replace(
        "        run: go build -v -o zqk ./cmd/zqk\n\n      - name: Test CLI\n        run: |\n          ./zqk --help\n          ./zqk system --help\n",
        '        run: |\n          . scripts/open-core/brand-env-prefix.sh\n          go build -v -o "$BRAND_EXE" ./cmd/zqk\n          echo "BRAND_EXE=$BRAND_EXE" >> "$GITHUB_ENV"\n\n      - name: Test CLI\n        run: |\n          ./"$BRAND_EXE" --help\n          ./"$BRAND_EXE" system --help\n',
    )
    write(".github/workflows/ci.yml", ci)

    mk = read("Makefile")
    mk = mk.replace("\tZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -short -timeout 10m ./pkg/...\n", "\tgo test -short -timeout 10m ./pkg/...\n")
    mk = mk.replace("\tZQK_ALLOW_FOREGROUND_GO_TEST=1 sh scripts/open-core/test-public-release-gates.sh\n", "\tsh scripts/open-core/test-public-release-gates.sh\n")
    write("Makefile", mk)

    gates = read("scripts/open-core/test-public-release-gates.sh")
    gates = gates.replace("\texport ZQK_ALLOW_FOREGROUND_GO_TEST=1\n", "")
    write("scripts/open-core/test-public-release-gates.sh", gates)

    verify = read("scripts/open-core/verify-bootstrap-portable.sh")
    verify = verify.replace("export ZQK_ALLOW_FOREGROUND_GO_TEST=1\n", "")
    write("scripts/open-core/verify-bootstrap-portable.sh", verify)

    sub_all("pkg/testrunner/stream.go", '\t\t"ZQK_ALLOW_FOREGROUND_GO_TEST=1",\n', "")
    sub_all("pkg/testrunner/testrunner.go", "\tcmd.Env = append(os.Environ(), zqkenv.ZQKAllowForegroundGoTest().Key+\"=1\")\n", "\tcmd.Env = os.Environ()\n")
    sub_all("pkg/testrunner/race_gate.go", "\tcmd.Env = append(os.Environ(), zqkenv.ZQKAllowForegroundGoTest().Key+\"=1\")\n", "\tcmd.Env = os.Environ()\n")
    race = read("pkg/testrunner/race_gate.go")
    if "zqkenv." not in race:
        write("pkg/testrunner/race_gate.go", race.replace('\t"github.com/zqk-os/zqk/pkg/zqkenv"\n', ""))
    sub_all("pkg/testrunner/test_runner_test.go", "\t_ = os.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, \"1\")\n", "")
    trt = read("pkg/testrunner/test_runner_test.go")
    if "zqkenv." not in trt:
        write("pkg/testrunner/test_runner_test.go", trt.replace('\t"github.com/zqk-os/zqk/pkg/zqkenv"\n', ""))
    sub_all("pkg/testrunner/shockwave_harness_integration_test.go", "\tt.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, \"1\")\n", "")
    sub_all(
        "cmd/zqk/test/discover_test.go",
        '\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)\n\nfunc init() {\n\t_ = os.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, "1")\n}\n',
        ")\n",
    )
    sub_all(
        "pkg/testdiscovery/discovery_test.go",
        '\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)\n\nfunc init() {\n\t_ = os.Setenv(zqkenv.ZQKAllowForegroundGoTest().Key, "1")\n}\n',
        ")\n",
    )
    sub_all("internal/bootstrap/polyglot_greenfield_test.go", 'cmd.Env = append(os.Environ(), "ZQK_ALLOW_FOREGROUND_GO_TEST=1")', 'cmd.Env = append(os.Environ(), "CGO_ENABLED=0")')

    sub_all("pkg/observer/semantic.go", 'zqkenv.Get("ZQK_OBSERVER_INCLUDE_TESTS")', "zqkenv.ObserverIncludeTests()")
    sub_all("pkg/observer/semantic.go", 'zqkenv.Get("ZQK_OBSERVER_CONCURRENCY")', "zqkenv.ObserverConcurrency()")
    sub_all("pkg/observer/semantic.go", 'zqkenv.Get("ZQK_OBSERVER_SKIP_INTENT")', "zqkenv.ObserverSkipIntent()")
    sub_all("pkg/llm/client.go", 'zqkenv.Get("ZQK_LLM_EMBED_MODEL")', "zqkenv.LLMEmbedModel()")
    sub_all("pkg/llm/client.go", 'zqkenv.Get("ZQK_DISABLE_LOCAL_OLLAMA")', "zqkenv.DisableLocalOllama()")
    sub_all("pkg/llm/client.go", 'zqkenv.Get("ZQK_FORCE_LOCAL_OLLAMA")', "zqkenv.ForceLocalOllama()")
    sub_all("pkg/coordination/coordinator.go", 'zqkenv.Get("ZQK_INFO_OPERATIONS")', "zqkenv.InfoOperations()")
    sub_all("pkg/coordination/coordinator.go", 'zqkenv.Get("ZQK_DEBUG_OPERATIONS")', "zqkenv.DebugOperations()")
    sub_all("cmd/zqk/agent/seat_worker.go", 'withEnvValue(out, "ZQK_LLM_TIMEOUT", timeout)', "withEnvValue(out, zqkenv.LLMTimeout().Name(), timeout)")

    for suffix, constn in [
        ("ENABLED", "graphEnvSuffixEnabled"),
        ("HOST", "graphEnvSuffixHost"),
        ("PORT", "graphEnvSuffixPort"),
        ("USERNAME", "graphEnvSuffixUsername"),
        ("PASSWORD", "graphEnvSuffixPassword"),
        ("DATABASE", "graphEnvSuffixDatabase"),
        ("POOL_SIZE", "graphEnvSuffixPoolSize"),
    ]:
        sub_all("pkg/mcp/graph_connection.go", f'"ZQK_GRAPH_{suffix}"', f'brand.DefaultEnvPrefix+"_"+{constn}')

    pprof = read("pkg/diagnostics/pprof_server.go")
    if "legacyZQKPprof" in pprof:
        if '"github.com/zqk-os/zqk/pkg/brand"' not in pprof:
            pprof = pprof.replace('\t"github.com/zqk-os/zqk/pkg/zqkenv"\n', '\t"github.com/zqk-os/zqk/pkg/brand"\n\t"github.com/zqk-os/zqk/pkg/zqkenv"\n')
        pprof = pprof.replace('legacyZQKPprof                = "ZQK_PPROF"\n\tlegacyZQKPprofPort            = "ZQK_PPROF_PORT"\n\tfallbackSchedulerDaemonPprof  = "ZQK_SCHEDULER_PPROF"\n\tfallbackSchedulerDaemonPort   = "ZQK_SCHEDULER_PPROF_PORT"\n\tfallbackStableBinaryPprof     = "ZQK_STABLE_PPROF"\n\tfallbackStableBinaryPprofPort = "ZQK_STABLE_PPROF_PORT"\n)', ')\n\nvar (\n\tlegacyDefaultPprof            = zqkenv.DefaultBrandKey("PPROF")\n\tlegacyDefaultPprofPort        = zqkenv.DefaultBrandKey("PPROF_PORT")\n\tfallbackSchedulerDaemonPprof  = brand.EnvPrefixForExecutable(brand.CanonicalExecutableToken+"-scheduler") + "_PPROF"\n\tfallbackSchedulerDaemonPort   = brand.EnvPrefixForExecutable(brand.CanonicalExecutableToken+"-scheduler") + "_PPROF_PORT"\n\tfallbackStableBinaryPprof     = zqkenv.DefaultBrandKey("STABLE_PPROF")\n\tfallbackStableBinaryPprofPort = zqkenv.DefaultBrandKey("STABLE_PPROF_PORT")\n)')
        pprof = pprof.replace("legacyZQKPprof,", "legacyDefaultPprof,")
        pprof = pprof.replace("legacyZQKPprofPort,", "legacyDefaultPprofPort,")
        write("pkg/diagnostics/pprof_server.go", pprof)

    deliver = read("pkg/agentfeed/deliver.go")
    if 'SessionEnvKey        = "ZQK_SESSION"' in deliver:
        if '"github.com/zqk-os/zqk/pkg/zqkenv"' not in deliver:
            deliver = deliver.replace('\t"github.com/zqk-os/zqk/pkg/idebridge"\n)', '\t"github.com/zqk-os/zqk/pkg/idebridge"\n\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)')
        deliver = deliver.replace('const (\n\tSessionEnvKey        = "ZQK_SESSION"\n\tFromAgentID          = "primary"\n\tskWakeAuthorExcluded = "author_excluded"\n)', 'const (\n\tFromAgentID          = "primary"\n\tskWakeAuthorExcluded = "author_excluded"\n)\n\nvar SessionEnvKey = zqkenv.Session().Name()')
        deliver = deliver.replace("val := strings.TrimSpace(os.Getenv(SessionEnvKey))", "val := strings.TrimSpace(zqkenv.Session().Get())")
        write("pkg/agentfeed/deliver.go", deliver)

    seating = read("pkg/authcred/seating.go")
    seating = seating.replace('strings.HasPrefix(e, "ZQK_PROJECT_ROOT=")', 'strings.HasPrefix(e, zqkenv.ProjectRoot().Name()+"=")')
    seating = seating.replace('strings.HasPrefix(e, "ZQK_")', "zqkenv.IsProductPrefixed(e)")
    seating = seating.replace('"ZQK_PROJECT_ROOT="+projectRootEnv', 'zqkenv.ProjectRoot().Name()+"="+projectRootEnv')
    write("pkg/authcred/seating.go", seating)

    mesh = read("pkg/scheduler/handlers_mesh_lease_supervision.go")
    if '"github.com/zqk-os/zqk/pkg/zqkenv"' not in mesh:
        mesh = mesh.replace('\tfileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"\n)', '\tfileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"\n\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)')
        mesh = mesh.replace('\t"github.com/zqk-os/zqk/pkg/utils/fileutil"\n)', '\t"github.com/zqk-os/zqk/pkg/utils/fileutil"\n\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)')
    mesh = mesh.replace('"ZQK_PROJECT_ROOT": consumerProjectRoot,', "zqkenv.ProjectRoot().Name(): consumerProjectRoot,")
    mesh = mesh.replace('"ZQK_PROFILE": "system",', 'zqkenv.Profile().Name(): "system",')
    write("pkg/scheduler/handlers_mesh_lease_supervision.go", mesh)

    cap = read("pkg/scheduler/handlers_cap_orchestrator.go")
    cap = cap.replace('\tconst apiKey = "ZQK_API_KEY" //nolint:gosec\n', "\tapiKey := zqkenv.APIKey().Name()\n")
    cap = cap.replace('strings.HasPrefix(e, "ZQK_GRAPH_ENABLED=") || strings.HasPrefix(e, "ZQK_ADMIN_GRAPH_ENABLED=")', 'zqkenv.HasDefaultAssignment(e, "GRAPH_ENABLED") || zqkenv.HasDefaultAssignment(e, "ADMIN_GRAPH_ENABLED")')
    write("pkg/scheduler/handlers_cap_orchestrator.go", cap)

    conv = read("pkg/scheduler/convergence_engine.go")
    if '"github.com/zqk-os/zqk/pkg/zqkenv"' not in conv:
        conv = conv.replace('\t"github.com/zqk-os/zqk/pkg/objects"\n)', '\t"github.com/zqk-os/zqk/pkg/objects"\n\t"github.com/zqk-os/zqk/pkg/zqkenv"\n)')
    conv = conv.replace('env := append(os.Environ(), "ZQK_PROJECT_ROOT="+s.projectRoot)', 'env := append(os.Environ(), zqkenv.ProjectRoot().Name()+"="+s.projectRoot)')
    write("pkg/scheduler/convergence_engine.go", conv)

    shim = read("cmd/zqk-shim/main.go")
    shim = shim.replace('getenv("ZQK_BREAK_GLASS_REASON")', 'getenv(zqkenv.DefaultBrandKey("BREAK_GLASS_REASON"))')
    shim = shim.replace('getenv("ZQK_SHIM_BYPASS_POLCODE009")', 'getenv(zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace('getenv("GIT_ZQK_SHIM_BYPASS_POLCODE009")', 'getenv("GIT_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace('getenv("GH_ZQK_SHIM_BYPASS_POLCODE009")', 'getenv("GH_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace('getenv(zqkenv.DefaultBrandKey("SHIM_BYPASS_POLCODE009"))', 'getenv(zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace('getenv("GIT_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_POLCODE009"))', 'getenv("GIT_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace('getenv("GH_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_POLCODE009"))', 'getenv("GH_"+zqkenv.DefaultBrandKey("SHIM_BYPASS_TRACEABILITY"))')
    shim = shim.replace("validatePOLCODE009", "validateCommitTraceability")
    shim = shim.replace("ZqkShimBypassPolCode009", "ZqkShimBypassTraceability")
    shim = shim.replace("POL-CODE-009", "commit traceability")
    shim = shim.replace("POLCODE009", "TRACEABILITY")
    shim = shim.replace('getenv("ZQK_AGENT_PRIVATE_KEY")', 'getenv(zqkenv.DefaultBrandKey("AGENT_PRIVATE_KEY"))')
    write("cmd/zqk-shim/main.go", shim)

    subp = read("pkg/zqkenv/subprocess_environ.go")
    subp = subp.replace("subprocessChildTestRootEnvKey = brand.DefaultEnvPrefix + \"_TEST_ROOT\"", 'subprocessChildTestRootEnvKey = DefaultBrandKey("TEST_ROOT")')
    subp = subp.replace("subprocessChildZQKProjectRootEnvKey = brand.DefaultEnvPrefix + \"_PROJECT_ROOT\"", 'subprocessChildZQKProjectRootEnvKey = DefaultBrandKey("PROJECT_ROOT")')
    subp = subp.replace("subprocessChildZQKTestDataDirEnvKey = brand.DefaultEnvPrefix + \"_TEST_DATA_DIR\"", 'subprocessChildZQKTestDataDirEnvKey = DefaultBrandKey("TEST_DATA_DIR")')
    subp = subp.replace('if strings.HasPrefix(e, "ZQK_SESSION=") || strings.HasPrefix(e, "ZQK_TEST_SESSION=")', 'if HasDefaultAssignment(e, "SESSION") || HasDefaultAssignment(e, "TEST_SESSION")')
    for old, new in [
        ('out = append(out, "ZQK_TEST_ROOT="+testRoot)', 'out = append(out, DefaultAssignment("TEST_ROOT", testRoot))'),
        ('out = append(out, "ZQK_ADMIN_TEST_ROOT="+testRoot)', 'out = append(out, AdminAssignment("TEST_ROOT", testRoot))'),
        ('out = append(out, "ZQK_TEST_ALLOW_CAS_FALLTHROUGH=1")', 'out = append(out, DefaultAssignment("TEST_ALLOW_CAS_FALLTHROUGH", "1"))'),
        ('out = append(out, "ZQK_ADMIN_TEST_ALLOW_CAS_FALLTHROUGH=1")', 'out = append(out, AdminAssignment("TEST_ALLOW_CAS_FALLTHROUGH", "1"))'),
        ('out = append(out, "ZQK_TEST_BYPASS_AUTH=1")', 'out = append(out, DefaultAssignment("TEST_BYPASS_AUTH", "1"))'),
        ('out = append(out, "ZQK_PRIVILEGED_WRITER_SOCKET="+unreachableSocket)', 'out = append(out, DefaultAssignment("PRIVILEGED_WRITER_SOCKET", unreachableSocket))'),
        ('out = append(out, "ZQK_ADMIN_PRIVILEGED_WRITER_SOCKET="+unreachableSocket)', 'out = append(out, AdminAssignment("PRIVILEGED_WRITER_SOCKET", unreachableSocket))'),
        ('out = append(out, "ZQK_MOCK_GRAPH=false")', 'out = append(out, DefaultAssignment("MOCK_GRAPH", "false"))'),
        ('out = append(out, "ZQK_ADMIN_MOCK_GRAPH=false")', 'out = append(out, AdminAssignment("MOCK_GRAPH", "false"))'),
        ('out = append(out, "ZQK_GRAPH_ENABLED=false")', 'out = append(out, DefaultAssignment("GRAPH_ENABLED", "false"))'),
        ('out = append(out, "ZQK_ADMIN_GRAPH_ENABLED=false")', 'out = append(out, AdminAssignment("GRAPH_ENABLED", "false"))'),
        ('if tr != "ZQK_TEST_ROOT=" && tr != "ZQK_ADMIN_TEST_ROOT=" {', 'if tr != DefaultBrandKey("TEST_ROOT")+"=" && tr != AdminBrandKey("TEST_ROOT")+"=" {'),
    ]:
        subp = subp.replace(old, new)
    write("pkg/zqkenv/subprocess_environ.go", subp)

    print("done")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
