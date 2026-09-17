# Gantt Chart Export Pipeline

## Purpose
This document describes the export pipeline for converting rendered Gantt chart SVG artifacts into PNG and PDF formats for enterprise users.

## Prerequisites
The export pipeline requires the `rsvg-convert` binary (part of `librsvg`) to be installed and available in your system's PATH. This dependency is required because it handles SVG to PNG/PDF conversion with high fidelity.

- **macOS**: `brew install librsvg`
- **Linux (Ubuntu/Debian)**: `sudo apt-get install librsvg2-bin`

## How to Invoke the Export
The repository provides an integration helper script to automate the export process from a rendered SVG. 

To run the export pipeline on a Gantt chart SVG:

```bash
go run scripts/export_gantt_artifact.go path/to/input.svg
```

Alternatively, you can specify specific outputs:
```bash
go run scripts/export_gantt_artifact.go path/to/input.svg custom_output.png custom_output.pdf
```

## Where Outputs Land
If no specific output paths are provided, the script will automatically generate `.png` and `.pdf` files in the exact same directory as the input SVG file, sharing the same base name.
For example, `go run scripts/export_gantt_artifact.go ./out/my_gantt.svg` will produce:
- `./out/my_gantt.png`
- `./out/my_gantt.pdf`

## Error Handling
If `rsvg-convert` is missing from the PATH, the tool fails gracefully with an actionable error message (`rsvg-convert not in PATH`). Tests similarly skip execution gracefully if the tooling is unavailable locally, though CI pipelines ensure its presence for deterministic generation.
