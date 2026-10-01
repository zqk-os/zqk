package agentonboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// AgentPacksRelDir holds regenerable per-vendor boot packs (≤100 top-level vendor dirs).
var AgentPacksRelDir = paths.AgentPacksDir

// BootPayloadFor returns shared kernel boot text plus a thin vendor-specific addendum.
func BootPayloadFor(v Vendor) string {
	return BootPayload() + "\n" + vendorAddendum(v)
}

func vendorAddendum(v Vendor) string {
	exe := brand.ExecutableName()
	switch v.ID {
	case VendorIDE:
		return fmt.Sprintf(`## Vendor: IDE
- Prefer MCP via `+"`"+`%s mcp serve`+"`"+` over inventing parallel tool bridges.
- Keep orchestration coordinated via the kernel (feed/scheduler/orchestrate), ensuring proper initialization context is provided.
`, exe)
	case VendorClaudeCode:
		return `## Vendor: Claude Code
- Treat CLAUDE.md as regenerable from agent-onboard; durable state stays in kernel objects.
`
	case VendorCline:
		return `## Vendor: Cline
- Regenerable .clinerules must not become a second source of truth outside the kernel.
`
	case VendorWindsurf:
		return `## Vendor: Windsurf
- Regenerable .windsurfrules must not become a second source of truth outside the kernel.
`
	case VendorGemini:
		return `## Vendor: Gemini CLI
- Do not store loops or mission state under .gemini/; use kernel objects + whats-next.
`
	case VendorAgent:
		return `## Vendor: Agent
- Prefer kernel seating + feed notify for multi-agent wake over vendor-only chat paste.
`
	case VendorOpenClaw:
		return `## Vendor: OpenClaw
- Prefer kernel seating + feed notify for multi-agent wake over vendor-only chat paste.
`
	case VendorAgentsMD:
		return `## Universal AGENTS.md
- This file is the headless-safe directive surface (Vector B). IDE rule forests are optional packs.
`
	default:
		return fmt.Sprintf("## Vendor: %s\n- Keep directives regenerable via %s system agent-onboard.\n", v.DisplayName, exe)
	}
}

// WriteVendorPacks writes BootPayloadFor each vendor under .zqk/agent_packs/<id>/BOOT.md.
func WriteVendorPacks(projectRoot string, vendors []Vendor, dryRun, force bool) (written []string, err error) {
	for _, v := range vendors {
		rel := filepath.ToSlash(filepath.Join(AgentPacksRelDir, string(v.ID), "BOOT.md"))
		abs := filepath.Join(projectRoot, filepath.FromSlash(rel))
		if pathExists(abs) && !force {
			continue
		}
		if dryRun {
			written = append(written, rel)
			continue
		}
		if err := fileutil.EnsureDir(filepath.Dir(abs)); err != nil {
			return written, errfmt.Newf("ensure agent pack dir for %s", v.ID).Wrap(err)
		}
		if err := fileutil.WriteSecureFile(abs, []byte(BootPayloadFor(v))); err != nil {
			return written, errfmt.Newf("write agent pack %s", rel).Wrap(err)
		}
		written = append(written, rel)
	}
	return written, nil
}

// WorkspaceFingerprint is a stable-ish lite identity for the sync report (not a CAS object id).
func WorkspaceFingerprint(projectRoot, vector string, detected []DetectedVendor) string {
	var b strings.Builder
	b.WriteString(filepath.Clean(projectRoot))
	b.WriteByte('|')
	b.WriteString(vector)
	b.WriteByte('|')
	for _, d := range detected {
		b.WriteString(string(d.ID))
		b.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}
