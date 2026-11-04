package brand

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func createDirHelper(path string, perm os.FileMode) error {
	mkdirAllFn := os.MkdirAll
	return mkdirAllFn(path, perm)
}

func writeFileHelper(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, writeErr := f.Write(data)
	return writeErr
}

func TestRuntimeBrandDiscovery_ExecutableInspection(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "empty os.Args slice",
			args:     []string{},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "empty os.Args[0] string",
			args:     []string{""},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "windows zqk executable with uppercase extension",
			args:     []string{"/usr/local/bin/zqk.EXE"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "canonical zqk-stable executable",
			args:     []string{"/usr/local/bin/zqk-stable"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "canonical zqk-stable prefixed executable",
			args:     []string{"/usr/local/bin/zqk-stable-20261024"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "daemon role executable zqk-pw",
			args:     []string{"/opt/zqk/bin/zqk-pw"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "daemon role executable zqk-amb",
			args:     []string{"/opt/zqk/bin/zqk-amb"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "daemon role executable zqk-sched",
			args:     []string{"/opt/zqk/bin/zqk-sched"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "daemon role executable zqk-overseer",
			args:     []string{"/opt/zqk/bin/zqk-overseer"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "daemon role executable with windows exe extension",
			args:     []string{"/opt/zqk/bin/zqk-pw.exe"},
			expected: defaultExecutableNameValue,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			got := defaultExecutableName()
			if got != tc.expected {
				t.Fatalf("defaultExecutableName() for %v = %q, want %q", tc.args, got, tc.expected)
			}
		})
	}
}

func TestRuntimeBrandDiscovery_GoHarnessAndDynamicOverrides(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "go run binary name main",
			args:     []string{"/tmp/go-runner/main"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "go run windows binary main.exe",
			args:     []string{"/tmp/go-runner/main.exe"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "go test binary suffix",
			args:     []string{"/tmp/pkg.test"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "go-build temporary path pattern",
			args:     []string{"/tmp/go-build12345/b001/exe/arbitrary"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "jetbrains go build temporary path pattern",
			args:     []string{"/tmp/___go_build_app_go"},
			expected: defaultExecutableNameValue,
		},
		{
			name:     "custom dynamic brand with role suffix sched",
			args:     []string{"/usr/local/bin/enterprise-sched"},
			expected: "enterprise",
		},
		{
			name:     "custom dynamic brand with role suffix amb and exe extension",
			args:     []string{"/usr/local/bin/acme-amb.exe"},
			expected: "acme",
		},
		{
			name:     "custom standalone executable name",
			args:     []string{"/usr/local/bin/customctl"},
			expected: "customctl",
		},
		{
			name:     "custom standalone windows executable",
			args:     []string{"/usr/local/bin/customctl.exe"},
			expected: "customctl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			got := defaultExecutableName()
			if got != tc.expected {
				t.Fatalf("defaultExecutableName() for %v = %q, want %q", tc.args, got, tc.expected)
			}
		})
	}
}

func TestExecutableName_FallbackAndSetterNormalization(t *testing.T) {
	origExe := ExecutableName()
	t.Cleanup(func() { SetExecutableName(origExe) })

	// Empty string should be ignored by SetExecutableName
	SetExecutableName("acme")
	SetExecutableName("")
	if got := ExecutableName(); got != "acme" {
		t.Fatalf("ExecutableName() after SetExecutableName('') = %q, want acme", got)
	}

	// Normalizing zqk-stable and zqk-stable- prefixes
	SetExecutableName(ZqkStableName)
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName() after SetExecutableName(ZqkStableName) = %q, want %q", got, defaultExecutableNameValue)
	}

	SetExecutableName("zqk-stable-202610")
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName() after SetExecutableName(prefix) = %q, want %q", got, defaultExecutableNameValue)
	}

	// Normalizing go test and go run names
	SetExecutableName("subsystem.test")
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName() after SetExecutableName('*.test') = %q, want %q", got, defaultExecutableNameValue)
	}

	SetExecutableName("main")
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName() after SetExecutableName('main') = %q, want %q", got, defaultExecutableNameValue)
	}

	SetExecutableName("main.exe")
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName() after SetExecutableName('main.exe') = %q, want %q", got, defaultExecutableNameValue)
	}

	// Direct atomic store of empty string tests the fallback branch in ExecutableName()
	executableName.Store("")
	if got := ExecutableName(); got != defaultExecutableName() {
		t.Fatalf("ExecutableName() with empty atomic store = %q, want fallback %q", got, defaultExecutableName())
	}
}

func TestStableExecutableAndPrefixNaming_Variations(t *testing.T) {
	origExe := ExecutableName()
	t.Cleanup(func() { SetExecutableName(origExe) })

	// When executable is zcom
	SetExecutableName("zcom")
	if got := StableExecutableName(); got != "zcom" {
		t.Fatalf("StableExecutableName() for zcom = %q, want zcom", got)
	}
	if got := StablePrefix(); got != "zcom-" {
		t.Fatalf("StablePrefix() for zcom = %q, want zcom-", got)
	}

	// When executable is default zqk
	SetExecutableName("zqk")
	if got := StableExecutableName(); got != ZqkStableName {
		t.Fatalf("StableExecutableName() for zqk = %q, want %q", got, ZqkStableName)
	}
	if got := StablePrefix(); got != ZqkStablePrefix {
		t.Fatalf("StablePrefix() for zqk = %q, want %q", got, ZqkStablePrefix)
	}

	// When executable is empty string in atomic value
	executableName.Store("")
	if got := StableExecutableName(); got != ZqkStableName {
		t.Fatalf("StableExecutableName() for empty = %q, want %q", got, ZqkStableName)
	}
	if got := StablePrefix(); got != ZqkStablePrefix {
		t.Fatalf("StablePrefix() for empty = %q, want %q", got, ZqkStablePrefix)
	}

	// When executable is custom brand
	SetExecutableName("enterprise")
	if got := StableExecutableName(); got != "enterprise-stable" {
		t.Fatalf("StableExecutableName() for enterprise = %q, want enterprise-stable", got)
	}
	if got := StablePrefix(); got != "enterprise-stable-" {
		t.Fatalf("StablePrefix() for enterprise = %q, want enterprise-stable-", got)
	}
}

func TestDaemonExecutableNames_DefaultAndCustomBrand(t *testing.T) {
	origExe := ExecutableName()
	t.Cleanup(func() { SetExecutableName(origExe) })

	// Test default brand
	SetExecutableName("zqk")
	if got := MCPExecutableName(); got != "zqk-mcp" {
		t.Fatalf("MCPExecutableName() = %q, want zqk-mcp", got)
	}
	if got := MCPDaemonExecutableName(); got != "zqk-mcp-daemon" {
		t.Fatalf("MCPDaemonExecutableName() = %q, want zqk-mcp-daemon", got)
	}
	if got := MCPIdeAdapterExecutableName(); got != "zqk-mcp-ide-adapter" {
		t.Fatalf("MCPIdeAdapterExecutableName() = %q, want zqk-mcp-ide-adapter", got)
	}

	// Test empty brand fallback
	executableName.Store("")
	if got := MCPExecutableName(); got != "zqk-mcp" {
		t.Fatalf("MCPExecutableName() empty = %q, want zqk-mcp", got)
	}
	if got := MCPDaemonExecutableName(); got != "zqk-mcp-daemon" {
		t.Fatalf("MCPDaemonExecutableName() empty = %q, want zqk-mcp-daemon", got)
	}
	if got := MCPIdeAdapterExecutableName(); got != "zqk-mcp-ide-adapter" {
		t.Fatalf("MCPIdeAdapterExecutableName() empty = %q, want zqk-mcp-ide-adapter", got)
	}
	if got := SchedExecutableName(); got != "zqk-sched" {
		t.Fatalf("SchedExecutableName() empty = %q, want zqk-sched", got)
	}
	if got := AmbExecutableName(); got != "zqk-amb" {
		t.Fatalf("AmbExecutableName() empty = %q, want zqk-amb", got)
	}
	if got := PWExecutableName(); got != "zqk-pw" {
		t.Fatalf("PWExecutableName() empty = %q, want zqk-pw", got)
	}
	if got := OverseerExecutableName(); got != "zqk-overseer" {
		t.Fatalf("OverseerExecutableName() empty = %q, want zqk-overseer", got)
	}

	// Test custom brand
	SetExecutableName("acmecorp")
	if got := MCPExecutableName(); got != "acmecorp-mcp" {
		t.Fatalf("MCPExecutableName() = %q, want acmecorp-mcp", got)
	}
	if got := MCPDaemonExecutableName(); got != "acmecorp-mcp-daemon" {
		t.Fatalf("MCPDaemonExecutableName() = %q, want acmecorp-mcp-daemon", got)
	}
	if got := MCPIdeAdapterExecutableName(); got != "acmecorp-mcp-ide-adapter" {
		t.Fatalf("MCPIdeAdapterExecutableName() = %q, want acmecorp-mcp-ide-adapter", got)
	}
}

func TestRoleDifferentiatorNames_ComprehensiveRoles(t *testing.T) {
	origExe := ExecutableName()
	t.Cleanup(func() { SetExecutableName(origExe) })

	// Test empty / whitespace role
	if got := RoleDifferentiatorNames(""); got != nil {
		t.Fatalf("RoleDifferentiatorNames('') = %v, want nil", got)
	}
	if got := RoleDifferentiatorNames("   "); got != nil {
		t.Fatalf("RoleDifferentiatorNames('   ') = %v, want nil", got)
	}

	// Test with default brand "zqk"
	SetExecutableName("zqk")
	rolesToVerify := map[string][]string{
		"sched":            {"zqk-sched", "zqk-scheduler"},
		"scheduler":        {"zqk-sched", "zqk-scheduler"},
		"scheduler-daemon": {"zqk-sched", "zqk-scheduler"},
		"amb":              {"zqk-amb", "zqk-ambient"},
		"ambient":          {"zqk-amb", "zqk-ambient"},
		"ambient-daemon":   {"zqk-amb", "zqk-ambient"},
		"pw":               {"zqk-pw", "zqk-privileged-writer"},
		"writer":           {"zqk-pw", "zqk-privileged-writer"},
		"overseer":         {"zqk-overseer"},
		"mcp":              {"zqk-mcp-daemon", "zqk-mcp"},
		"mcp-daemon":       {"zqk-mcp-daemon", "zqk-mcp"},
		"ide-adapter":      {"zqk-mcp-ide-adapter"},
		"mcp-ide-adapter":  {"zqk-mcp-ide-adapter"},
		"worker":           {"zqk-worker"},
	}

	for role, expected := range rolesToVerify {
		got := RoleDifferentiatorNames(role)
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("RoleDifferentiatorNames(%q) for zqk = %v, want %v", role, got, expected)
		}
	}

	// Test with custom brand "acme"
	SetExecutableName("acme")
	customRolesToVerify := map[string][]string{
		"sched":           {"acme-sched", "acme-scheduler", "zqk-sched", "zqk-scheduler"},
		"amb":             {"acme-amb", "acme-ambient", "zqk-amb", "zqk-ambient"},
		"pw":              {"acme-pw", "acme-privileged-writer", "zqk-pw", "zqk-privileged-writer"},
		"overseer":        {"acme-overseer", "zqk-overseer"},
		"mcp":             {"acme-mcp-daemon", "acme-mcp", "zqk-mcp-daemon", "zqk-mcp"},
		"mcp-ide-adapter": {"acme-mcp-ide-adapter", "zqk-mcp-ide-adapter"},
		"worker":          {"acme-worker", "zqk-worker"},
	}

	for role, expected := range customRolesToVerify {
		got := RoleDifferentiatorNames(role)
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("RoleDifferentiatorNames(%q) for acme = %v, want %v", role, got, expected)
		}
	}
}

func TestIsProductExecutable_ExtensiveMatrix(t *testing.T) {
	origExe := ExecutableName()
	t.Cleanup(func() { SetExecutableName(origExe) })

	SetExecutableName("acme")

	positiveMatches := []string{
		"acme",
		"acme.exe",
		"acme-stable",
		"acme-mcp",
		"acme-mcp-daemon",
		"acme-mcp-ide-adapter",
		"acme-sched",
		"acme-amb",
		"acme-pw",
		"acme-overseer",
		// Canonical names
		"zqk",
		"zqk.exe",
		"zqk-stable",
		"zqk-mcp",
		"zqk-mcp-daemon",
		"zqk-mcp-ide-adapter",
		"zcom",
		"zqk-sched",
		"zqk-scheduler",
		"zqk-amb",
		"zqk-ambient",
		"zqk-pw",
		"zqk-privileged-writer",
		"zqk-overseer",
		// Prefixes
		"acme-stable-20261024",
		"zqk-stable-v1.0.0",
		// Dynamic stripped role suffixes
		"acme-sched",
		"zcom-overseer",
	}

	for _, name := range positiveMatches {
		if !IsProductExecutable(name) {
			t.Errorf("IsProductExecutable(%q) = false, want true", name)
		}
	}

	negativeMatches := []string{
		"bash",
		"python.exe",
		"unrelated-tool",
		"acme-unrecognized-suffix",
		"random_bin",
	}

	for _, name := range negativeMatches {
		if IsProductExecutable(name) {
			t.Errorf("IsProductExecutable(%q) = true, want false", name)
		}
	}
}

func TestProductNameAndNamespacePrefix_StorageFallback(t *testing.T) {
	origProd := ProductName()
	origNS := NamespacePrefix()
	t.Cleanup(func() {
		SetProductName(origProd)
		SetNamespacePrefix(origNS)
	})

	productName.Store("")
	if got := ProductName(); got != defaultProductNameValue {
		t.Fatalf("ProductName() fallback = %q, want %q", got, defaultProductNameValue)
	}

	namespacePrefix.Store("")
	if got := NamespacePrefix(); got != defaultNamespacePrefixValue {
		t.Fatalf("NamespacePrefix() fallback = %q, want %q", got, defaultNamespacePrefixValue)
	}

	// SetNamespacePrefix with empty string should be ignored
	SetNamespacePrefix("valid-prefix")
	SetNamespacePrefix("")
	if got := NamespacePrefix(); got != "valid-prefix" {
		t.Fatalf("NamespacePrefix() after empty SetNamespacePrefix = %q, want valid-prefix", got)
	}
}

func TestProductNamespacePrefix_ExtendedSuffixTrimming(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"enterprise-admin", "enterprise"},
		{"enterprise-admin-admin", "enterprise"},
		{"", defaultNamespacePrefixValue},
		{"mycorp-community", "mycorp"},
		{"mycorp-dev", "mycorp"},
		{"mycorp-rc", "mycorp"},
		{"mybrand-stable", "mybrand"},
	}

	for _, tc := range testCases {
		got := ProductNamespacePrefix(tc.input)
		if got != tc.expected {
			t.Errorf("ProductNamespacePrefix(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestLoadFromProject_MalformedAndPartialConfigs(t *testing.T) {
	tempDir := t.TempDir()
	cfgDir := filepath.Join(tempDir, "config")
	if err := createDirHelper(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config failed: %v", err)
	}

	// 1. Non-existent root returns defaults
	defCfg := LoadFromProject(filepath.Join(tempDir, "does-not-exist"))
	if defCfg.ExecutableName != defaultExecutableNameValue ||
		defCfg.ProductName != defaultProductNameValue ||
		defCfg.NamespacePrefix != defaultNamespacePrefixValue {
		t.Fatalf("LoadFromProject non-existent root returned unexpected defaults: %+v", defCfg)
	}

	// 2. Corrupt YAML file in zqk.yaml should be safely skipped
	corruptYAML := []byte(`
brand:
  executable_name: [invalid yaml struct
`)
	if err := writeFileHelper(filepath.Join(cfgDir, "zqk.yaml"), corruptYAML); err != nil {
		t.Fatalf("write corrupt zqk.yaml: %v", err)
	}
	corruptCfg := LoadFromProject(tempDir)
	if corruptCfg.ExecutableName != defaultExecutableNameValue {
		t.Fatalf("LoadFromProject with corrupt YAML returned %q, want default %q", corruptCfg.ExecutableName, defaultExecutableNameValue)
	}

	// 3. Partial YAML in zqk.yaml followed by local override
	partialYAML := []byte(`
brand:
  product_name: "PartialProduct"
`)
	if err := writeFileHelper(filepath.Join(cfgDir, "zqk.yaml"), partialYAML); err != nil {
		t.Fatalf("write partial zqk.yaml: %v", err)
	}

	localYAML := []byte(`
brand:
  namespace_prefix: "localns"
`)
	if err := writeFileHelper(filepath.Join(cfgDir, "zqk-local.yaml"), localYAML); err != nil {
		t.Fatalf("write local zqk-local.yaml: %v", err)
	}

	combinedCfg := LoadFromProject(tempDir)
	if combinedCfg.ProductName != "PartialProduct" {
		t.Errorf("expected ProductName PartialProduct, got %q", combinedCfg.ProductName)
	}
	if combinedCfg.NamespacePrefix != "localns" {
		t.Errorf("expected NamespacePrefix localns, got %q", combinedCfg.NamespacePrefix)
	}
	if combinedCfg.ExecutableName != defaultExecutableNameValue {
		t.Errorf("expected ExecutableName %q, got %q", defaultExecutableNameValue, combinedCfg.ExecutableName)
	}
}

func TestEnvPrefix_ComprehensiveSuffixesAndCollapsing(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string falls back to default", "", "ZQK"},
		{"mcp proxy suffix", "zqk-mcp-proxy", "ZQK"},
		{"mcp daemon suffix", "zqk-mcp-daemon", "ZQK"},
		{"mcp ide adapter suffix", "zqk-mcp-ide-adapter", "ZQK"},
		{"privileged writer suffix", "acme-privileged-writer", "ACME"},
		{"ambient daemon suffix", "acme-ambient-daemon", "ACME"},
		{"ambient short suffix", "acme-ambient", "ACME"},
		{"scheduler daemon suffix", "corp-scheduler-daemon", "CORP"},
		{"scheduler suffix", "corp-scheduler", "CORP"},
		{"beta suffix", "tool-beta", "TOOL"},
		{"alpha suffix", "tool-alpha", "TOOL"},
		{"rc suffix", "tool-rc", "TOOL"},
		{"all non-alphanumeric runes", "---___---", "ZQK"},
		{"repeating underscores and hyphens collapsed", "foo---bar___baz", "FOO_BAR_BAZ"},
		{"alphanumeric with digits", "app2-v3-release", "APP2_V3_RELEASE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := EnvPrefixForExecutable(tc.input)
			if got != tc.expected {
				t.Errorf("EnvPrefixForExecutable(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}

	// Test EnvVar with and without leading underscore
	if got := EnvVar("DATABASE_URL"); got != EnvPrefix()+"_DATABASE_URL" {
		t.Errorf("EnvVar('DATABASE_URL') = %q, want %q", got, EnvPrefix()+"_DATABASE_URL")
	}
	if got := EnvVar("_DATABASE_URL"); got != EnvPrefix()+"_DATABASE_URL" {
		t.Errorf("EnvVar('_DATABASE_URL') = %q, want %q", got, EnvPrefix()+"_DATABASE_URL")
	}
}
