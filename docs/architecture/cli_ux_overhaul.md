# CLI UX Overhaul Plan ([REDACTED-ID])

**Last Verified:** 2026-08-31


## 1. Executive Summary
The ZQK CLI currently offers a functional but basic developer experience. Output relies on standard streams and simple terminal styling (using `fatih/color`). To transition ZQK into a world-class tool for AI+human hybrid teams, we need to introduce modern CLI UX paradigms: richer terminal colors, interactive prompts, progress indicators (spinners), and advanced table formatting.

This document outlines the architectural strategy for integrating these enhancements seamlessly across the `pkg/cli` and `cmd/zqk` packages.

## 2. Current State Assessment
- **Command Framework:** Cobra/Viper is used for command routing and flag parsing.
- **Color/Styling:** Basic styling via `fatih/color` (e.g., bold headers, dim separators).
- **Table Formatting:** A homegrown `table_formatter.go` handles truncation, column widths, and spacing. It lacks advanced features like dynamic resizing, borders, and multi-line cell support.
- **Interactivity:** Limited. Most inputs require explicit flags or standard input streams.
- **Feedback Mechanisms:** Slow operations do not consistently provide visual feedback (spinners or progress bars).

## 3. Proposed Enhancements & Technologies

### 3.1. Richer Terminal Colors & Theming
**Goal:** Establish a consistent, semantic color palette (Success, Info, Warning, Error, Highlight) and support themes (dark/light terminal profiles).
- **Library Choice:** Continue using `github.com/fatih/color` for simple text, or migrate to `github.com/charmbracelet/lipgloss` for advanced, composable styling.
- **Implementation:** 
  - Create a central `pkg/cli/theme` package.
  - Define semantic printers (e.g., `theme.Success()`, `theme.Warning()`).
  - Deprecate raw color calls scattered across the codebase.

### 3.2. Interactive Prompts
**Goal:** Allow users to seamlessly fallback to interactive mode if required flags are missing, or when performing complex data entry (e.g., creating objects without the UI).
- **Library Choice:** `github.com/charmbracelet/huh` or `github.com/AlecAivazis/survey/v2`.
- **Implementation:**
  - Introduce `pkg/cli/prompt` to abstract the underlying library.
  - Implement standard prompt types: `Confirm`, `Select`, `MultiSelect`, `Input`, and `Password`.
  - Integrate with `command_builder.go`: If a required field is missing from flags, invoke the interactive prompt to elicit it from the user before executing the command.

### 3.3. Spinner and Progress Abstractions
**Goal:** Provide clear visual feedback for long-running operations (e.g., graph syncs, network requests, AI processing).
- **Library Choice:** `github.com/briandowns/spinner` or `github.com/charmbracelet/bubbles/spinner`.
- **Implementation:**
  - Create `pkg/cli/ux` with `StartSpinner(message string)` and `StopSpinner(success bool, finalMessage string)`.
  - Spinners must automatically disable themselves if stdout is not a TTY (for CI/CD compatibility).
  - Wrap high-latency operations in the orchestration layer with the spinner context.

### 3.4. Advanced Table Formatting
**Goal:** Replace the homegrown `table_formatter.go` with a robust solution capable of handling complex terminal output cleanly.
- **Library Choice:** `github.com/jedib0t/go-pretty/v6/table` or `github.com/pterm/pterm`.
- **Implementation:**
  - Update `pkg/cli/table_formatter.go` to use the new engine under the hood.
  - Support features: Custom borders, auto-resizing columns based on terminal width, text wrapping, and right/left alignment.
  - Ensure compatibility with the existing `--columns` flag parsing logic to avoid breaking changes.

## 4. Implementation Strategy

### Phase 1: Foundation & Theming
- Introduce `pkg/cli/theme`.
- Replace existing `fatih/color` usages with the centralized theme definitions.

### Phase 2: Output Upgrades (Tables & Spinners)
- Refactor `table_formatter.go` to use `go-pretty/table`.
- Introduce `pkg/cli/ux` for spinner management and apply it to at least three known slow operations.

### Phase 3: Interactivity (Prompts)
- Introduce `pkg/cli/prompt`.
- Update `command_builder.go` to support an interactive fallback mode for missing required parameters.

## 5. Backward Compatibility & CI/CD
- **TTY Detection:** All UX enhancements must strictly check if the environment is a TTY (`isatty`). If not, gracefully degrade to standard log/text output to avoid corrupting CI/CD pipelines.
- **Flags:** Retain `--columns` and existing formatting flags to ensure scripts do not break. Add a `--no-color` or `--plain` flag globally to disable rich UX explicitly.
