package zqkenv_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestCycle245_AgentGuardExecution exercises EnforceForegroundGoTestGuard, IsGoTestBinary, and panic messages.
func TestCycle245_AgentGuardExecution(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})

	t.Run("NonTestBinaryReturnsCleanly", func(t *testing.T) {
		os.Args = []string{"/bin/zqk", "system", "check"}
		assert.NotPanics(t, func() {
			zqkenv.EnforceForegroundGoTestGuard()
		})
	})

	t.Run("AllowedBypassViaEnvReturnsCleanly", func(t *testing.T) {
		os.Args = []string{"/tmp/my_test.test"}
		t.Setenv(zqkenv.AllowForegroundGoTest().Name(), "1")
		assert.NotPanics(t, func() {
			zqkenv.EnforceForegroundGoTestGuard()
		})
	})

	t.Run("AllowedBypassViaJobIDReturnsCleanly", func(t *testing.T) {
		os.Args = []string{"/tmp/my_test.test"}
		t.Setenv(zqkenv.AllowForegroundGoTest().Name(), "")
		t.Setenv(zqkenv.JobID().Name(), "JOB-12345")
		assert.NotPanics(t, func() {
			zqkenv.EnforceForegroundGoTestGuard()
		})
	})

	t.Run("DisallowedForegroundTestPanics", func(t *testing.T) {
		os.Args = []string{"/tmp/sub/_test/binary"}
		for _, k := range []string{
			zqkenv.JobID().Name(), "ZQK_JOB_ID", "ZQK_SCHEDULER_JOB_ID",
			zqkenv.AllowForegroundGoTest().Name(), "ZQK_ALLOW_FOREGROUND_GO_TEST",
			zqkenv.TestRoot().Name(), "ZQK_TEST_ROOT",
			zqkenv.InTest().Name(), zqkenv.UpdateHelpGolden().Name(), "ZQKCLI_TEST_UPDATE_HELP_GOLDEN",
			"MAKEFLAGS", "MAKELEVEL", "CI", "GITHUB_ACTIONS", "GITLAB_CI",
		} {
			if k != "" {
				t.Setenv(k, "")
			}
		}

		assert.Panics(t, func() {
			zqkenv.EnforceForegroundGoTestGuard()
		})
	})
}

// TestCycle245_BrandKeysAndAccessors tests brand key helpers, prefix matching, and env getters.
func TestCycle245_BrandKeysAndAccessors(t *testing.T) {
	t.Run("IsProductPrefixedVariations", func(t *testing.T) {
		curPrefix := brand.EnvPrefix() + "_"
		assert.True(t, zqkenv.IsProductPrefixed(curPrefix+"TEST_VAR=1"))
		assert.False(t, zqkenv.IsProductPrefixed("PATH=/usr/bin"))
		assert.False(t, zqkenv.IsProductPrefixed("RANDOM_VARIABLE=true"))
	})

	t.Run("BrandKeyConstructors", func(t *testing.T) {
		assert.Contains(t, zqkenv.DefaultBrandKey("FOO"), "FOO")
		assert.Contains(t, zqkenv.AdminBrandKey("BAR"), "ADMIN_BAR")
		assert.Contains(t, zqkenv.DefaultAssignment("KEY", "VAL"), "KEY=VAL")
		assert.Contains(t, zqkenv.AdminAssignment("KEY", "VAL"), "ADMIN_KEY=VAL")
		assert.True(t, zqkenv.HasDefaultAssignment(zqkenv.DefaultAssignment("TEST", "1"), "TEST"))
		assert.False(t, zqkenv.HasDefaultAssignment("OTHER=1", "TEST"))
	})

	t.Run("BrandKeyEnvVarGetters", func(t *testing.T) {
		assert.NotEmpty(t, zqkenv.Airgap().Name())
		assert.NotEmpty(t, zqkenv.AllowDegraded().Name())
		assert.NotEmpty(t, zqkenv.BypassHandslapper().Name())
		assert.NotEmpty(t, zqkenv.Codegen().Name())
		assert.NotEmpty(t, zqkenv.ContextProfile().Name())
		assert.NotEmpty(t, zqkenv.DebugOperations().Name())
		assert.NotEmpty(t, zqkenv.DevCodegen().Name())
		assert.NotEmpty(t, zqkenv.DisableLocalOllama().Name())
		assert.NotEmpty(t, zqkenv.DistDir().Name())
		assert.NotEmpty(t, zqkenv.ForceLocalOllama().Name())
		assert.NotEmpty(t, zqkenv.InfoOperations().Name())
		assert.NotEmpty(t, zqkenv.LogLevel().Name())
		assert.NotEmpty(t, zqkenv.NonInteractive().Name())
		assert.NotEmpty(t, zqkenv.ObserverConcurrency().Name())
		assert.NotEmpty(t, zqkenv.ObserverIncludeTests().Name())
		assert.NotEmpty(t, zqkenv.ObserverSkipIntent().Name())
		assert.NotEmpty(t, zqkenv.Profile().Name())
		assert.NotEmpty(t, zqkenv.TaskID().Name())
		assert.NotEmpty(t, zqkenv.Timeout().Name())
		assert.NotEmpty(t, zqkenv.Env().Name())
	})
}

// TestCycle245_PrivilegedWriterAndDaemonHelpers tests daemon role and command fragments.
func TestCycle245_PrivilegedWriterAndDaemonHelpers(t *testing.T) {
	t.Run("PrivilegedWriterDaemonCommandFragment", func(t *testing.T) {
		frag := zqkenv.PrivilegedWriterDaemonCommandFragment()
		assert.Equal(t, "object daemon", frag)
	})

	t.Run("PrivilegedWriterRequiredLaunchEnv", func(t *testing.T) {
		envMap := zqkenv.PrivilegedWriterRequiredLaunchEnv()
		assert.NotEmpty(t, envMap)
		assert.Equal(t, "1", envMap[zqkenv.IsDaemon().Name()])
	})

	t.Run("DaemonProcessMethods", func(t *testing.T) {
		t.Setenv(zqkenv.IsDaemon().Name(), "1")
		assert.True(t, zqkenv.DaemonProcess())

		t.Setenv(zqkenv.IsDaemon().Name(), "0")
		assert.False(t, zqkenv.DaemonProcess())

		t.Setenv("TEST_DAEMON_KEY", "custom_val")
		v := zqkenv.Get("TEST_DAEMON_KEY")
		assert.Equal(t, "custom_val", v.OrDefault("fallback"))

		t.Setenv("TEST_DAEMON_KEY", "42")
		v = zqkenv.Get("TEST_DAEMON_KEY")
		assert.Equal(t, 42, v.IntOrDefault(10))

		t.Setenv("TEST_DAEMON_KEY", "true")
		v = zqkenv.Get("TEST_DAEMON_KEY")
		assert.True(t, v.BoolOrDefault(false))

		// Defaults when unset
		t.Setenv("TEST_DAEMON_KEY", "")
		v = zqkenv.Get("TEST_DAEMON_KEY")
		assert.Equal(t, "default_val", v.OrDefault("default_val"))
		assert.Equal(t, 99, v.IntOrDefault(99))
		assert.False(t, v.BoolOrDefault(false))
	})

	t.Run("OSEnvSetter", func(t *testing.T) {
		zqkenv.OSEnvSetter("TEST_SETTER_KEY", "setter_val")
		assert.Equal(t, "setter_val", os.Getenv("TEST_SETTER_KEY"))
	})
}

// TestCycle245_EnvVarTypesAndStringMethod tests EnvVar.String() and uncommon vars.
func TestCycle245_EnvVarTypesAndStringMethod(t *testing.T) {
	t.Run("EnvVarStringMethod", func(t *testing.T) {
		v := zqkenv.Airgap()
		assert.Equal(t, v.Name(), v.String())
	})

	t.Run("UncommonVarsGetters", func(t *testing.T) {
		assert.NotEmpty(t, zqkenv.AmbientWatchDirs().Name())
		assert.NotEmpty(t, zqkenv.CellularMembraneModeB().Name())
		assert.NotEmpty(t, zqkenv.EnforceCellularMembrane().Name())
	})

	t.Run("EnvVarRequiredError", func(t *testing.T) {
		customVar := zqkenv.Airgap()
		t.Setenv(customVar.Name(), "")
		assert.Panics(t, func() {
			customVar.Required()
		})

		t.Setenv(customVar.Name(), "airgap-active")
		val := customVar.Required()
		assert.Equal(t, "airgap-active", val)
	})
}

// TestCycle245_MaskingAndRedactionPipeline tests sensitive key detection and environment sanitization.
func TestCycle245_MaskingAndRedactionPipeline(t *testing.T) {
	t.Run("IsSensitiveKeyDetection", func(t *testing.T) {
		assert.True(t, zqkenv.IsSensitiveKey("API_KEY"))
		assert.True(t, zqkenv.IsSensitiveKey("user_token"))
		assert.True(t, zqkenv.IsSensitiveKey("DATABASE_PASSWORD"))
		assert.True(t, zqkenv.IsSensitiveKey("JWT_SECRET"))
		assert.True(t, zqkenv.IsSensitiveKey("AUTHORIZATION_HEADER"))
		assert.True(t, zqkenv.IsSensitiveKey("SSH_PRIVATE_KEY"))
		assert.False(t, zqkenv.IsSensitiveKey("LOG_LEVEL"))
		assert.False(t, zqkenv.IsSensitiveKey("PROJECT_ROOT"))
	})

	t.Run("MaskSensitiveValueRules", func(t *testing.T) {
		assert.Equal(t, "", zqkenv.MaskSensitiveValue("KEY", ""))
		assert.Equal(t, "******", zqkenv.MaskSensitiveValue("SECRET_KEY", "sensitive-pass"))
		assert.Equal(t, "Bearer ******", zqkenv.MaskSensitiveValue("AUTH", "Bearer xyz123456"))
		assert.Equal(t, "plain_val", zqkenv.MaskSensitiveValue("SAFE_CONFIG", "plain_val"))
	})

	t.Run("SanitizeFieldsMap", func(t *testing.T) {
		raw := map[string]any{
			"api_token": "secret12345",
			"log_level": "info",
			"nested": map[string]any{
				"pass": "hiddenpass",
			},
		}
		sanitized := zqkenv.SanitizeFields(raw)
		assert.NotEqual(t, "secret12345", sanitized["api_token"])
		assert.Equal(t, "info", sanitized["log_level"])
	})

	t.Run("SanitizeEnvironmentSlice", func(t *testing.T) {
		environ := []string{
			"ZQK_LOG_LEVEL=debug",
			"OPENAI_API_KEY=sk-1234567890abcdef",
			"INVALID_ENTRY_WITHOUT_EQUALS",
		}
		sanitized := zqkenv.SanitizeEnvironment(environ)
		require.Len(t, sanitized, 3)
		assert.Contains(t, sanitized[0], "debug")
		assert.NotContains(t, sanitized[1], "sk-1234567890abcdef")
		assert.Contains(t, sanitized[1], "***")
		assert.Equal(t, "INVALID_ENTRY_WITHOUT_EQUALS", sanitized[2])
	})
}

// TestCycle245_SubprocessEnvironAndDaemonIsolation tests process wiring, scrub, and prefix logic.
func TestCycle245_SubprocessEnvironAndDaemonIsolation(t *testing.T) {
	t.Run("WireExecForIsolatedProjectWithNilCmd", func(t *testing.T) {
		assert.NotPanics(t, func() {
			zqkenv.WireExecForIsolatedProject(nil, "/tmp/project")
		})
	})

	t.Run("WireExecForIsolatedProjectValidCmd", func(t *testing.T) {
		cmd := &exec.Cmd{Path: "/bin/echo", Args: []string{"echo", "hello"}}
		zqkenv.WireExecForIsolatedProject(cmd, "/tmp/test-project-dir")
		assert.Equal(t, "/tmp/test-project-dir", cmd.Dir)
		assert.NotEmpty(t, cmd.Env)
	})

	t.Run("SubprocessEnvironWithTestRootAndExtras", func(t *testing.T) {
		env := zqkenv.SubprocessEnvironWithTestRootAndExtras(
			"/tmp/testroot",
			"CUSTOM_EXTRA_1=initial",
			"INVALID_NO_EQUALS",
			"=NO_KEY",
			"CUSTOM_EXTRA_1=overridden",
			"CUSTOM_EXTRA_2=second",
		)
		assert.NotEmpty(t, env)
		foundCount := 0
		for _, item := range env {
			if strings.HasPrefix(item, "CUSTOM_EXTRA_1=") {
				foundCount++
				assert.Equal(t, "CUSTOM_EXTRA_1=overridden", item)
			}
		}
		assert.Equal(t, 1, foundCount)
	})

	t.Run("ScrubDaemonProcessEnv", func(t *testing.T) {
		t.Setenv(zqkenv.ParentPID().Name(), "12345")
		t.Setenv(zqkenv.MCPAccountID().Name(), "acct_999")
		zqkenv.ScrubDaemonProcessEnv()
		assert.Empty(t, os.Getenv(zqkenv.ParentPID().Name()))
		assert.Empty(t, os.Getenv(zqkenv.MCPAccountID().Name()))
		assert.Equal(t, "1", os.Getenv(zqkenv.IsParentZqk().Name()))
	})

	t.Run("MCPEnvKeyPrefix", func(t *testing.T) {
		prefix := zqkenv.MCPEnvKeyPrefix()
		assert.True(t, strings.HasSuffix(prefix, "MCP_"))
	})

	t.Run("IsInTestEnvOverride", func(t *testing.T) {
		t.Setenv(brand.EnvVar("IN_TEST"), "true")
		assert.True(t, zqkenv.IsInTest())
	})
}
