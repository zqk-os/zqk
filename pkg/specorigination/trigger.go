package specorigination

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const triggerOutputMaxRunes = 2048

const adminBinaryName = "zqk-admin"

// applyTriggerCommands runs idempotent zqk subprocesses for SPEC_ORIGIN_TRIGGER (see DATA_ORIGINATION_PIPELINE_VISION.md).
// It uses the sibling zqk-admin binary because trigger steps are code-generation
// and internal-materialization commands, while spec-origination itself is also
// exposed by the primary zqk binary.
func applyTriggerCommands(ctx context.Context, projectRoot, ontology string) error {
	if err := generateSpecBuilder(projectRoot, ontology); err != nil {
		return err
	}

	bin, err := resolveAdminTriggerBinary()
	if err != nil {
		return err
	}

	steps := []struct {
		name string
		args []string
	}{
		{
			name: "sync_glossary",
			args: []string{"system", "sync-glossary-from-specs", "--apply", "--dry-run=false", "--format", "json"},
		},
		{
			name: "generate_instance_builders",
			args: []string{"system", "generate-instance-builders", "--overwrite"},
		},
		{
			name: "path_cache",
			args: []string{"system", "path-cache"},
		},
	}

	for _, st := range steps {
		cmd := execwrap.CommandContext(ctx, bin, st.args...)
		cmd.Dir = projectRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(truncateRunes(string(out), triggerOutputMaxRunes))
			if msg != "" {
				return errfmt.Errorf("spec origination trigger step %s failed: %w\n%s", st.name, err, msg)
			}
			return errfmt.Errorf("spec origination trigger step %s failed: %w", st.name, err)
		}
	}
	return nil
}

func generateSpecBuilder(projectRoot, ontology string) error {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	specPath := filepath.Join(specsDir, ontology+".yaml")
	outputDir := filepath.Join(projectRoot, "pkg", "specbuilder", "builders")
	versionDir := filepath.Join(filepath.Dir(outputDir), builders.CurrentBuilderPackage)
	factory := builders.NewConstantsFactoryWithSpecLoader(
		builders.CurrentBuilderPackage,
		objects.NewSpecLoader(specsDir),
	)
	if err := factory.ReserveConstantsFromDir(versionDir, ontology); err != nil {
		return errfmt.Errorf("spec origination trigger step reserve_spec_constants failed: %w", err)
	}
	if err := builders.GenerateBuilderFromYAML(specPath, outputDir, "", factory); err != nil {
		return errfmt.Errorf("spec origination trigger step generate_spec_builder failed: %w", err)
	}
	return nil
}

func resolveAdminTriggerBinary() (string, error) {
	current, err := fileutil.Executable()
	if err != nil {
		return "", errfmt.Errorf("spec origination trigger: resolve executable: %w", err)
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", errfmt.Errorf("spec origination trigger: eval symlinks: %w", err)
	}

	for _, admin := range adminTriggerBinaryCandidates(current) {
		info, statErr := fileutil.Stat(admin)
		if statErr != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		return admin, nil
	}
	return "", errfmt.Errorf(
		"spec origination trigger requires %s in a stable sibling or repository bin directory",
		adminBinaryName,
	)
}

func adminTriggerBinaryPath(current string) string {
	base := filepath.Base(current)
	if base == adminBinaryName || base == adminBinaryName+".exe" {
		return current
	}
	return filepath.Join(filepath.Dir(current), adminBinaryName)
}

func adminTriggerBinaryCandidates(current string) []string {
	sibling := adminTriggerBinaryPath(current)
	candidates := []string{sibling}
	dir := filepath.Dir(current)
	if filepath.Base(dir) == paths.WorkshopBinDir &&
		filepath.Base(filepath.Dir(dir)) == paths.ProjectDataDir {
		projectRoot := filepath.Dir(filepath.Dir(dir))
		candidates = append(
			candidates,
			filepath.Join(projectRoot, paths.RepoBinDir, adminBinaryName),
		)
	}
	return candidates
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
