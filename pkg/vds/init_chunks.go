package vds

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func exampleChunksYAML() string {
	evalJSON := paths.CLIUsage("workflow", "vds", "evaluate", "--format", "json")
	evalBrief := paths.CLIUsage("workflow", "vds", "evaluate", "--format", "agent-prompt")
	return `# Verifiable Decomposition Spine — working chunks
# Policy: POL-WORKFLOW-VDS
# Evaluate: ` + evalJSON + `
# Brief:    ` + evalBrief + `
#
# Each chunk must stand alone. Chat is never evidence.

schema: zqk_vds_chunks_v1
chunks:
  - chunk_id: vds-replace-me
    stage: design
    claim: "Replace with one independently verifiable claim"
    work_object_ref: OBJ-REPLACE-ME
    rubric_ref: POL-WORKFLOW-VDS#chunk-contract
    dsl_checks:
      - object_exists:OBJ-REPLACE-ME
    evidence_refs:
      - OBJ-REPLACE-ME
    gate_design: yes
    independent_verify: pending
`
}

// InitChunksFile writes the default chunks scaffold if missing (or force).
func InitChunksFile(projectRoot string, force bool) (path string, created bool, err error) {
	path = filepath.Join(projectRoot, DefaultChunksRel)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return path, false, errfmt.Errorf("vds: mkdir: %w", err)
	}
	if !force {
		if _, err := fileutil.Stat(path); err == nil {
			return path, false, nil
		}
	}
	if err := fileutil.WriteFile(path, []byte(exampleChunksYAML()), paths.FilePerm644); err != nil {
		return path, false, errfmt.Errorf("vds: write chunks: %w", err)
	}
	return path, true, nil
}
