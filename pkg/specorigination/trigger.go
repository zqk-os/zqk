package specorigination

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const triggerOutputMaxRunes = 2048

// applyTriggerCommands runs idempotent zqk subprocesses for SPEC_ORIGIN_TRIGGER (see DATA_ORIGINATION_PIPELINE_VISION.md).
// Uses [os.Executable] as the CLI binary and sets process working directory to projectRoot.
func applyTriggerCommands(ctx context.Context, projectRoot string) error {
	bin, err := os.Executable()
	if err != nil {
		return errfmt.Errorf("spec origination trigger: resolve executable: %w", err)
	}
	bin, err = filepath.EvalSymlinks(bin)
	if err != nil {
		return errfmt.Errorf("spec origination trigger: eval symlinks: %w", err)
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
		cmd := exec.CommandContext(ctx, bin, st.args...)
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
