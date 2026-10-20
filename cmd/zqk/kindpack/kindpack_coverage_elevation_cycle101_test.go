package kindpack

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// helper to build a well-formed kindpack directory with specified name and kinds
func createCycle101SpecPack(t *testing.T, packName string, kinds []string) string {
	t.Helper()
	dir := t.TempDir()

	var kindsYaml strings.Builder
	for _, k := range kinds {
		kindsYaml.WriteString("  - " + k + "\n")
	}

	manifestContent := "name: " + packName + "\nkinds:\n" + kindsYaml.String()
	manifestPath := filepath.Join(dir, "kindpack.yaml")
	require.NoError(t, fileutil.WriteFile(manifestPath, []byte(manifestContent), paths.FilePerm644))

	specDir := filepath.Join(dir, "specs")
	require.NoError(t, fileutil.EnsureDir(specDir))

	lifecycleDir := filepath.Join(dir, "lifecycles")
	require.NoError(t, fileutil.EnsureDir(lifecycleDir))

	for _, k := range kinds {
		specContent := "ontology: " + k + "\nschema_version: 2.0.0\nfields:\n  - name: title\n    type: string\n"
		require.NoError(t, fileutil.WriteFile(filepath.Join(specDir, k+".yaml"), []byte(specContent), paths.FilePerm644))

		lifeContent := "name: " + k + "\nstatuses:\n  - name: originated\n  - name: completed\n"
		require.NoError(t, fileutil.WriteFile(filepath.Join(lifecycleDir, k+"_lifecycle.yaml"), []byte(lifeContent), paths.FilePerm644))
	}

	return dir
}

// TestCycle101_KindPackCmd_HelpAndStructure verifies command metadata and dynamic help rendering.
func TestCycle101_KindPackCmd_HelpAndStructure(t *testing.T) {
	cmd := NewKindPackCmd()
	require.NotNil(t, cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	out := buf.String()

	require.Contains(t, out, "install")
	require.Contains(t, out, "Record a verified spec pack as typed object kinds")

	// Subcommand help
	var subBuf bytes.Buffer
	subCmd := newInstallCmd()
	subCmd.SetOut(&subBuf)
	subCmd.SetErr(&subBuf)
	subCmd.SetArgs([]string{"--help"})
	require.NoError(t, subCmd.Execute())
	subOut := subBuf.String()
	require.Contains(t, subOut, "Verify an uploaded spec pack and record its formal specs into the kernel")
}

// TestCycle101_KindPack_InstallExecution verifies standard, multi-kind, and idempotent error paths.
func TestCycle101_KindPack_InstallExecution(t *testing.T) {
	testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Install multi-kind pack: widget and gadget
	packDir := createCycle101SpecPack(t, "kpk-demo-pack", []string{"widget", "gadget"})

	cmd := NewKindPackCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"install", packDir})

	require.NoError(t, cmd.Execute())
	require.Contains(t, outBuf.String(), "recorded kpk-demo-pack (widget,gadget)")

	// 2. Re-install attempt should fail with 'already recorded'
	cmdReinstall := NewKindPackCmd()
	cmdReinstall.SetArgs([]string{"install", packDir})
	err := cmdReinstall.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already recorded")

	// 3. Installing with wrong number of arguments
	cmdNoArgs := NewKindPackCmd()
	cmdNoArgs.SetArgs([]string{"install"})
	require.Error(t, cmdNoArgs.Execute())

	cmdExtraArgs := NewKindPackCmd()
	cmdExtraArgs.SetArgs([]string{"install", packDir, "extraneous-arg"})
	require.Error(t, cmdExtraArgs.Execute())
}

// TestCycle101_KindPack_ValidationFailures verifies pack verification errors bubble up through CLI execution.
func TestCycle101_KindPack_ValidationFailures(t *testing.T) {
	testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := t.TempDir()

	// 1. Pack directory missing kindpack.yaml
	missingManifestDir := filepath.Join(tmpDir, "missing-manifest")
	require.NoError(t, fileutil.EnsureDir(missingManifestDir))
	cmdMissing := NewKindPackCmd()
	cmdMissing.SetArgs([]string{"install", missingManifestDir})
	err := cmdMissing.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "kindpack.yaml")

	// 2. Pack declaring no kinds
	noKindsDir := filepath.Join(tmpDir, "no-kinds")
	require.NoError(t, fileutil.EnsureDir(noKindsDir))
	require.NoError(t, fileutil.WriteFile(filepath.Join(noKindsDir, "kindpack.yaml"), []byte("name: empty-kinds\nkinds: []\n"), paths.FilePerm644))
	cmdNoKinds := NewKindPackCmd()
	cmdNoKinds.SetArgs([]string{"install", noKindsDir})
	err = cmdNoKinds.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "declares no kinds")

	// 3. Spec ontology mismatch
	mismatchDir := filepath.Join(tmpDir, "mismatch-pack")
	require.NoError(t, fileutil.EnsureDir(filepath.Join(mismatchDir, "specs")))
	require.NoError(t, fileutil.EnsureDir(filepath.Join(mismatchDir, "lifecycles")))
	require.NoError(t, fileutil.WriteFile(filepath.Join(mismatchDir, "kindpack.yaml"), []byte("name: mismatch\nkinds:\n  - device\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(mismatchDir, "specs", "device.yaml"), []byte("ontology: different_device\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(mismatchDir, "lifecycles", "device_lifecycle.yaml"), []byte("name: device\nstatuses:\n  - name: init\n"), paths.FilePerm644))
	cmdMismatch := NewKindPackCmd()
	cmdMismatch.SetArgs([]string{"install", mismatchDir})
	err = cmdMismatch.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec ontology is")

	// 4. Duplicate kinds declared in manifest
	dupDir := filepath.Join(tmpDir, "duplicate-kinds")
	require.NoError(t, fileutil.EnsureDir(filepath.Join(dupDir, "specs")))
	require.NoError(t, fileutil.EnsureDir(filepath.Join(dupDir, "lifecycles")))
	require.NoError(t, fileutil.WriteFile(filepath.Join(dupDir, "kindpack.yaml"), []byte("name: dup-pack\nkinds:\n  - item\n  - item\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(dupDir, "specs", "item.yaml"), []byte("ontology: item\nschema_version: 2.0.0\n"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(dupDir, "lifecycles", "item_lifecycle.yaml"), []byte("name: item\nstatuses:\n  - name: init\n"), paths.FilePerm644))
	cmdDup := NewKindPackCmd()
	cmdDup.SetArgs([]string{"install", dupDir})
	err = cmdDup.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate kind")
}

// TestCycle101_JoinKinds_EdgeCases covers join formatting thoroughly.
func TestCycle101_JoinKinds_EdgeCases(t *testing.T) {
	require.Equal(t, "", joinKinds(nil))
	require.Equal(t, "", joinKinds([]string{}))
	require.Equal(t, "single", joinKinds([]string{"single"}))
	require.Equal(t, "one,two", joinKinds([]string{"one", "two"}))
	require.Equal(t, "a,b,c,d,e", joinKinds([]string{"a", "b", "c", "d", "e"}))
}
