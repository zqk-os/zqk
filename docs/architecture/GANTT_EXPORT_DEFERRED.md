# Gantt export (PNG, PDF)

**Backlog:** BLI-213 — export pipeline for PNG and PDF from rendered Gantt SVG.

**In-tree helpers:** `pkg/gantt/export_rsvg.go` — `ExportPNGWithRsvgConvert` and `ExportPDFWithRsvgConvert` when **`rsvg-convert`** (librsvg) is on `PATH`. Tests skip if the binary is missing. Use `WriteMinimalInteractionSVGFile` for a deterministic sample.

**Strategic note:** Full integration with the historical Gantt renderer remains aligned with the graph-backend pivot (`docs/marketing/strategic-pivot/SVG_GANTT_WORK_PRESERVATION.md`). A future `zqk` subcommand can wrap the helpers above with golden-file CI where `rsvg-convert` is installed.

**Prerequisite:** Stable SVG output meeting `GANTT_SVG_INTERACTION_CONTRACT.md` (BLI-214 contract).
