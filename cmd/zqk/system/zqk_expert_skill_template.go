package system

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// EnsureDefaultSkillsFiles verifies that the canonical zqk-expert skill pack exists in the project root
// under .zqk/skills/zqk-expert/SKILL.md and is linked/exposed to .agent/skills/zqk-expert for IDE agents.
func EnsureDefaultSkillsFiles(projectRoot string, logger logging.Logger) error {
	if projectRoot == emptyValue {
		return nil
	}

	zqkSkillsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "skills", "zqk-expert")
	if err := fileutil.EnsureDir(zqkSkillsDir); err != nil {
		return err
	}
	zqkSkillPath := filepath.Join(zqkSkillsDir, "SKILL.md")
	if !fileutil.Exists(zqkSkillPath) {
		if err := fileutil.WriteFile(zqkSkillPath, []byte(ZQKExpertSkillContent), paths.FilePerm644); err != nil {
			return err
		}
		if logger != nil {
			logging.Fluent(logger).Info("Installed canonical zqk-expert skill pack").String("path", zqkSkillPath).Log()
		}
	}

	agentSkillsDir := filepath.Join(projectRoot, ".agent", "skills")
	if err := fileutil.EnsureDir(agentSkillsDir); err != nil {
		return err
	}
	agentSkillLink := filepath.Join(agentSkillsDir, "zqk-expert")
	if !fileutil.Exists(agentSkillLink) {
		relTarget := filepath.Join("..", "..", paths.ProjectDataDir, "skills", "zqk-expert")
		if err := fileutil.Symlink(relTarget, agentSkillLink); err != nil {
			agentExpertDir := filepath.Join(agentSkillsDir, "zqk-expert")
			_ = fileutil.EnsureDir(agentExpertDir)
			_ = fileutil.WriteFile(filepath.Join(agentExpertDir, "SKILL.md"), []byte(ZQKExpertSkillContent), paths.FilePerm644)
		}
	}

	return nil
}

// ZQKExpertSkillContent contains the canonical, sealed SKILL.md for the zqk-expert skill.
const ZQKExpertSkillContent = `---
name: zqk-expert
description: >-
  Expert guidance for ZQK Knowledge Kernel operations: CLI-only process data, VDS done-gates,
  scheduler test bundles, agent feed/MCP mesh, TPM Gantt matrix (roadmap/workstream/plan),
  plan-scoped branch collaboration, CLI command DNA, and convergence discipline.
  Use when operating zqk CLI, claiming work done, orchestrating swarm seats, touching .zqk/process,
  or aligning agent workflows with current system standards.
seal_hash: 53d963ff3c8a861178125d60a75a428378c0b59951d265cc8877b1c65b5674d9
seal_version: 1.0.0
seal_issuer: system:cli
seal_date: 2026-09-29T00:35:33-07:00
---

# ZQK Expert

Mandatory operating skill for swarm and IDE agents. Prefer **re-runnable evidence** over narrative.
When this file disagrees with live CLI help or an active ` + "`POL-*`" + ` object, **trust the kernel + CLI**.

**Canonical path:** ` + "`.zqk/skills/zqk-expert/` (keep `skills/zqk-expert/` in sync)." + `

## 1. Orchestration Hygiene & Critical Sections

You must execute all operations within a strict critical section to prevent race conditions.
Structure your execution as a ` + "`try-catch-finally`" + ` block guaranteeing lock release on all paths.

- **TRY:**
  - Execute: ` + "`zqk agent claim <task_id>`" + `
  - IF exit_code != 0 THEN HALT and do not proceed.
  - Append ` + "`--atomic --skip-parallel`" + ` to EVERY state-altering command you execute.
- **CATCH (Exception/Failure):**
  - IF any step fails, force lock release AND propagate failure.
- **FINALLY:**
  - ALWAYS execute lock release: ` + "`zqk agent release <task_id>`" + `

## 2. Fail-Closed Compliance

You must enforce strict fail-closed operations.
- IF any command returns a non-zero exit code, "command not found", "timeout", or "network unreachable", THEN emit an explicit JSON failure payload to stderr AND HALT.
- Standardize the error payload across all failure branches: ` + "`{\"status\": \"FAIL_CLOSED\", \"cause\": \"<specific_cause>\", \"context\": \"<command_executed>\"}`" + `.
- Narrative fallbacks are strictly FORBIDDEN. Never guess, assume completion, or hallucinate recovery logic for schema/validation failures.

## 3. Kernel Traceability & Deterministic Checks

- **Deterministic Evaluation**: Before claiming success on any kernel object mutation, you MUST run deterministic checks. For example, execute ` + "`zqk test run --all`" + ` and ensure the exit code is 0.
- **Event-Driven Traceability**: Shift from in-place mutation to event-driven logging where applicable. Update kernel objects via the CLI and ALWAYS use ` + "`--version <current>+1`" + ` to prevent race conditions. Ensure any changes trigger the appropriate CAS updates (` + "`.zqk/cas-index.json`" + `).

## Session boot (do this first)

` + "```bash\n" +
	`zqk workflow whats-next --format json          # mission / PRI / cues; add --agent-id <seat>
zqk system policy-interrupts list              # before high-stakes work
` + "```\n" + `

Infer next work from **whats-next JSON**, not from chat memory.

## Hard rules (non-negotiable)

| Rule | Do | Do not |
|------|----|--------|
| Process data | ` + "`zqk object create|update|promote|bulk …`" + ` | Edit instance YAML under ` + "`.zqk/process/`" + ` by hand |
| Done claims | ` + "`zqk workflow vds evaluate`" + ` (+ evidence) or ` + "`zqk do <id> --verify`" + ` | Mark BLI/CVS/CAP done from chat alone |
| Tests | ` + "`zqk test run --all`" + ` | Foreground ` + "`go test ./…`" + ` / multi-minute waits |
| Single-Command Loop | ` + "`zqk do <id>`" + ` | Raw hand-crafted state mutation sequences |
| Code search | ` + "`zqk grep`" + ` (in-process AST & trigram queries) | External slow shell binaries / brittle regex scans |

## Verifiable Decomposition Spine (VDS)

Policy: **POL-WORKFLOW-VDS**. Culture + DSL gate for "done."

` + "```bash\n" +
	`zqk workflow vds checklist --format json
zqk workflow vds evaluate --format json          # exit non-zero on FAIL
zqk workflow vds evaluate --format agent-prompt
zqk workflow vds evaluate --apply-verify         # persist independent_verify=yes (surgical)
` + "```\n" + `

Before claiming stage progress: name ` + "`stage` + `chunk_id`" + `, show ` + "`rubric_ref` / `dsl_checks` / `evidence_refs` / `independent_verify`" + `.

## TPM plane: Gantt matrix + plan branch collaboration

| Axis | Object | Meaning |
|------|--------|---------|
| Frame | ` + "`roadmap`" + ` | Product/system viewport around the Gantt |
| Rows (lanes) | ` + "`workstream`" + ` | Like/complementary activities; long-lived reporting lanes |
| Columns | ` + "`priority_plan`" + ` | Feature/functional bundles; may span multiple workstreams |
| Time anchors | ` + "`milestone`" + ` | Make the timeline honest |
| Cells | BLI → ATK | Work under a **locked** plan |

**Branch collaboration:**
1. After any merge to ` + "`main`" + `, fetch ` + "`origin/main`" + `. TPM provisions **one collaboration branch per plan** from **that tip** (e.g. ` + "`integration/pri-…`" + `). Never mint from a feature HEAD or stale local ` + "`main`" + `.
2. Agents work in isolated **git worktrees** off that branch — never ` + "`git checkout`" + ` between seats.
3. Merge up into the plan branch after pedantic/build gates.
4. TPM opens the **single PR** to ` + "`main`" + `.
`
