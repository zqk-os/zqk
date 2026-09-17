package system

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// objectDraftPlaneWarnTotal is an informational backlog signal (does not block "check green").
// TRACK: [REDACTED-ID]
const objectDraftPlaneWarnTotal = 50

// writeObjectDraftPlaneSummary reports objects that have not yet met the obligations to cross
// the CAS membrane, so they sit outside the Layer 0-3 rollup entirely (those layers describe
// in-membrane objects). Draft plane is a storage placement, not a lifecycle status value: the
// objects here hold kind-specific preliminary statuses such as draft, exploring, or identified.
// These IDs are omitted from normal object list/count; get-by-id still dual-reads.
func writeObjectDraftPlaneSummary(buf *strings.Builder, projectRoot string) {
	inv := storage.InventoryObjectDraftPlane(projectRoot)
	draftRoot := paths.ProjectDataDir + "/" + paths.ObjectDraftsDir
	headers := []string{"Kind", "Objects", "Sample Object IDs"}
	rows := make([][]string, 0, len(inv.ByKind))

	kinds := make([]string, 0, len(inv.ByKind))
	for kind := range inv.ByKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		// One id per line so the 45-char sample column cannot wrap mid-suffix
		// (copy/paste of a wrapped cell dropped the 'a' from 27599a81 / 3cf3884a).
		samples := strings.Join(inv.SampleIDsByKind[kind], "\n")
		remaining := inv.ByKind[kind] - len(inv.SampleIDsByKind[kind])
		if remaining > 0 {
			samples += fmt.Sprintf(" (+%d more)", remaining)
		}
		rows = append(rows, []string{formatColumnHeader(kind), fmt.Sprintf("%d", inv.ByKind[kind]), samples})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"None awaiting crossing.", "0", "-"})
	}

	buf.WriteString("\n=== System Check: Pre-Membrane (Object Draft Plane) ===\n\n")
	buf.WriteString(renderTableWithTitle(
		"Draft Plane: Preliminary Objects That Have Not Crossed the CAS Membrane",
		headers,
		rows,
	))
	buf.WriteString("\n")
	fmt.Fprintf(buf, "Stored under %s/ — outside CAS, so Layers 0-3 below do not cover these objects.\n", draftRoot)
	buf.WriteString("Get-by-ID works; normal list/count omit them. Promote once the crossing obligations are met (materializes into CAS), or delete when abandoned.\n")
	if inv.Total >= objectDraftPlaneWarnTotal {
		fmt.Fprintf(buf, "⚠️  %d object(s) awaiting crossing: backlog is growing.\n", inv.Total)
	}
	if len(inv.DualPlaneIDs) > 0 {
		fmt.Fprintf(buf, "❌ Dual-plane split-brain: %d id(s) exist on BOTH draft plane and CAS (Get reads draft; check validates CAS).\n", len(inv.DualPlaneIDs))
		fmt.Fprintf(buf, "   Fix: promote (materialize) or delete the draft shadow — never rm drafts blindly if CAS lacks the id.\n")
		maxShow := 8
		if len(inv.DualPlaneIDs) < maxShow {
			maxShow = len(inv.DualPlaneIDs)
		}
		for _, id := range inv.DualPlaneIDs[:maxShow] {
			fmt.Fprintf(buf, "      - %s\n", id)
		}
		if len(inv.DualPlaneIDs) > maxShow {
			fmt.Fprintf(buf, "      (+%d more)\n", len(inv.DualPlaneIDs)-maxShow)
		}
	}
	buf.WriteString("\n")
}
