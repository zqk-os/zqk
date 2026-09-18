package scheduler

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// idePasteStepToken names the keystroke tokens allowed in ZQK_CURSOR_PASTE_PREFIX_STEPS
// (comma-separated). Steps run after IDE is activated and frontmost, before ⌘V and Return.
const (
	idePasteTokenBuiltin      = "builtin"
	idePasteTokenCmdY         = "cmd_y"
	idePasteCmdShiftE    = "cmd_shift_e"
	idePasteTokenCmdL         = "cmd_l"
	idePasteTokenEscape       = "escape"
	idePasteOptionCmdE   = "option_cmd_e"
	idePasteTokenDefaultAlias = "default"
)

// defaultIDEPastePrefixTokens is the default prefix when ZQK_CURSOR_PASTE_PREFIX_STEPS is unset:
// ⌘Y focuses the IDE agent chat reliably before ⌘V.
func defaultIDEPastePrefixTokens() []string {
	return []string{idePasteTokenCmdY}
}

// defaultIDEPasteApplicationName is the macOS process name for this studio's agent IDE.
// Override with ZQK_IDE_PASTE_APP (see zqkenv.IDEPasteApp). Do not emit an unquoted
// name — that compiles as two identifiers and osascript fails with -2740.
const defaultIDEPasteApplicationName = "Cursor"

func idePasteApplicationName() string {
	if v := strings.TrimSpace(zqkenv.IDEPasteApp().Get()); v != "" {
		return v
	}
	return defaultIDEPasteApplicationName
}

func appleScriptQuoted(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// loadIDEPasteAutomation resolves the AppleScript for --paste-ide and log summaries.
// When ZQK_CURSOR_PASTE_PREFIX_STEPS is unset or empty, the default token sequence is used (see defaultIDEPastePrefixTokens).
// When set to "builtin" or "default" (alone), same as default.
// Otherwise: comma-separated tokens (see parseIDEPastePrefixTokens).
func loadIDEPasteAutomation() (*idePasteAutomation, error) {
	raw := strings.TrimSpace(zqkenv.IDEPastePrefixSteps().Get())
	if raw == "" {
		return loadIDEPasteAutomationFromTokens(defaultIDEPastePrefixTokens(), true)
	}
	low := strings.ToLower(raw)
	if low == idePasteTokenBuiltin || low == idePasteTokenDefaultAlias {
		return loadIDEPasteAutomationFromTokens(defaultIDEPastePrefixTokens(), true)
	}
	tokens, err := parseIDEPastePrefixTokens(raw)
	if err != nil {
		return nil, err
	}
	return loadIDEPasteAutomationFromTokens(tokens, false)
}

func loadIDEPasteAutomationFromTokens(tokens []string, isDefault bool) (*idePasteAutomation, error) {
	s, err := appleScriptIDEPasteFromTokens(tokens)
	if err != nil {
		return nil, err
	}
	if err := validateBuiltIDEPasteScript(s, tokens); err != nil {
		return nil, err
	}
	var prefixStepsHumanShort, prefixStepsDocLine string
	if isDefault {
		prefixStepsHumanShort = humanShortIDEPasteTokens(tokens)
		prefixStepsDocLine = "default token sequence (set " + zqkenv.IDEPastePrefixSteps().Name() + " to customize; tokens: cmd_y, cmd_shift_e, escape, cmd_l, option_cmd_e)"
	} else {
		prefixStepsHumanShort = humanShortIDEPasteTokens(tokens)
		prefixStepsDocLine = "custom " + zqkenv.IDEPastePrefixSteps().Name() + " sequence"
	}
	return &idePasteAutomation{
		appleScript:           s,
		osascriptBeginDetail:  ideKeystrokeOsascriptBeginDetailFromTokens(tokens),
		keystrokesSummary:     ideKeystrokeLoggerKeystrokesSummaryFromTokens(tokens),
		prefixStepsHumanShort: prefixStepsHumanShort,
		prefixStepsDocLine:    prefixStepsDocLine,
	}, nil
}

type idePasteAutomation struct {
	appleScript string
	// osascriptBeginDetail is written to the agent keystroke TSV log (osascript_begin row).
	osascriptBeginDetail string
	// keystrokesSummary is the structured log field for planned keystrokes.
	keystrokesSummary string
	// prefixStepsHumanShort is a short markdown fragment for user-facing footers (emoji-style names).
	prefixStepsHumanShort string
	// prefixStepsDocLine describes the sequence for footer copy (default vs env).
	prefixStepsDocLine string
}

func normalizeIDEPasteToken(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case idePasteTokenCmdY, "focus_chat":
		return idePasteTokenCmdY, true
	case idePasteCmdShiftE, "explorer", "show_explorer":
		return idePasteCmdShiftE, true
	case idePasteTokenEscape:
		return idePasteTokenEscape, true
	case idePasteTokenCmdL:
		return idePasteTokenCmdL, true
	case idePasteOptionCmdE:
		return idePasteOptionCmdE, true
	default:
		return "", false
	}
}

func parseIDEPastePrefixTokens(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		norm, ok := normalizeIDEPasteToken(p)
		if !ok {
			return nil, errfmt.Errorf(
				"invalid %s token %q (use comma-separated: %s, %s, %s, %s, %s; aliases: explorer, show_explorer, focus_chat; or alone: %s)",
				zqkenv.IDEPastePrefixSteps(), p,
				idePasteTokenCmdY, idePasteCmdShiftE, idePasteTokenEscape, idePasteTokenCmdL, idePasteOptionCmdE,
				idePasteTokenBuiltin,
			)
		}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil, errfmt.Errorf("%s is empty after parsing (use %s alone for the default script)",
			zqkenv.IDEPastePrefixSteps(), idePasteTokenBuiltin)
	}
	return out, nil
}

func writeAppleScriptPreamble(b *strings.Builder, tokens []string) {
	b.WriteString("-- Generated by zqk ")
	if env := strings.TrimSpace(zqkenv.IDEPastePrefixSteps().Get()); env != "" {
		b.WriteString("from ")
		b.WriteString(env)
	} else {
		b.WriteString("(default token sequence; set ")
		b.WriteString(zqkenv.IDEPastePrefixSteps().Name())
		b.WriteString(" to customize)")
	}
	b.WriteString("\n-- Tokens: ")
	b.WriteString(strings.Join(tokens, ", "))
	b.WriteString("\n-- See: cmd/zqk/scheduler/ide_paste_automation.go\n--\n\n")
}

func appleScriptIDEPasteFromTokens(tokens []string) (string, error) {
	app := appleScriptQuoted(idePasteApplicationName())
	var b strings.Builder
	writeAppleScriptPreamble(&b, tokens)
	b.WriteString(`log "zqk agent_prompt: step=activate_ide"` + "\n")
	b.WriteString(`tell application ` + app + ` to activate` + "\n")
	b.WriteString(`delay 0.5` + "\n")
	b.WriteString(`tell application "System Events"` + "\n")
	b.WriteString(`	set zqkCP to missing value` + "\n")
	b.WriteString(`	try` + "\n")
	b.WriteString(`		set zqkCP to first application process whose name is ` + app + ` and frontmost is true` + "\n")
	b.WriteString(`	end try` + "\n")
	b.WriteString(`	if zqkCP is missing value then` + "\n")
	b.WriteString(`		try` + "\n")
	b.WriteString(`			set zqkCP to first application process whose name is ` + app + "\n")
	b.WriteString(`		end try` + "\n")
	b.WriteString(`	end if` + "\n")
	b.WriteString(`	if zqkCP is missing value then` + "\n")
	b.WriteString(`		error "zqk agent_prompt: no application process named exactly " & ` + app + ` & " (is the IDE running?)"` + "\n")
	b.WriteString(`	end if` + "\n")
	b.WriteString(`	try` + "\n")
	b.WriteString(`		log "zqk agent_prompt: step=resolved_process unix_id=" & (unix id of zqkCP)` + "\n")
	b.WriteString(`	end try` + "\n")
	b.WriteString(`	tell zqkCP` + "\n")
	b.WriteString(`		set frontmost to true` + "\n")
	b.WriteString(`		delay 0.25` + "\n")
	for _, tok := range tokens {
		switch tok {
		case idePasteCmdShiftE:
			b.WriteString(`		log "zqk agent_prompt: step=focus_explorer_cmd_shift_e"` + "\n")
			b.WriteString(`		keystroke "e" using {command down, shift down}` + "\n")
			b.WriteString(`		delay 0.4` + "\n")
		case idePasteTokenEscape:
			b.WriteString(`		log "zqk agent_prompt: step=escape_defocus_stealing_inputs"` + "\n")
			b.WriteString(`		key code 53` + "\n")
			b.WriteString(`		delay 0.35` + "\n")
		case idePasteTokenCmdL:
			b.WriteString(`		log "zqk agent_prompt: step=command_l_panel_toggle"` + "\n")
			b.WriteString(`		keystroke "l" using command down` + "\n")
			b.WriteString(`		delay 0.4` + "\n")
		case idePasteTokenCmdY:
			b.WriteString(`		log "zqk agent_prompt: step=focus_ide_chat_cmd_y"` + "\n")
			b.WriteString(`		keystroke "y" using command down` + "\n")
			b.WriteString(`		delay 0.4` + "\n")
		case idePasteOptionCmdE:
			b.WriteString(`		log "zqk agent_prompt: step=option_cmd_e_focus_agent_chat"` + "\n")
			b.WriteString(`		keystroke "e" using {command down, option down}` + "\n")
			b.WriteString(`		delay 0.5` + "\n")
		default:
			return "", errfmt.Errorf("internal: unknown token %q", tok)
		}
	}
	b.WriteString(`		log "zqk agent_prompt: step=keystroke_cmd_v_paste"` + "\n")
	b.WriteString(`		keystroke "v" using command down` + "\n")
	b.WriteString(`		delay 0.3` + "\n")
	b.WriteString(`		log "zqk agent_prompt: step=key_code_36_return_submit"` + "\n")
	b.WriteString(`		key code 36` + "\n")
	b.WriteString(`	end tell` + "\n")
	b.WriteString(`end tell` + "\n")
	b.WriteString(`log "zqk agent_prompt: step=sequence_complete"` + "\n")
	return b.String(), nil
}

func ideKeystrokeOsascriptBeginDetailFromTokens(tokens []string) string {
	var parts []string
	parts = append(parts, "steps=activate")
	for _, t := range tokens {
		switch t {
		case idePasteCmdShiftE:
			parts = append(parts, "Cmd+Shift+E(Explorer)")
		case idePasteTokenEscape:
			parts = append(parts, "Escape")
		case idePasteTokenCmdL:
			parts = append(parts, "Cmd+L")
		case idePasteOptionCmdE:
			parts = append(parts, "option_cmd_e")
		case idePasteTokenCmdY:
			parts = append(parts, "Cmd+Y")
		}
	}
	parts = append(parts, "Cmd+V", "Return")
	return strings.Join(parts, ";") + "; see " + zqkenv.IDEPastePrefixSteps().Name()
}

func ideKeystrokeLoggerKeystrokesSummaryFromTokens(tokens []string) string {
	var b strings.Builder
	b.WriteString("activate_ide")
	for _, t := range tokens {
		switch t {
		case idePasteCmdShiftE:
			b.WriteString(";Cmd+Shift+E(Explorer)")
		case idePasteTokenEscape:
			b.WriteString(";Escape(53)")
		case idePasteTokenCmdL:
			b.WriteString(";Cmd+L")
		case idePasteOptionCmdE:
			b.WriteString(";option_cmd_e")
		case idePasteTokenCmdY:
			b.WriteString(";Cmd+Y")
		}
	}
	b.WriteString(";Cmd+V;Return(36)")
	return b.String()
}

func humanShortIDEPasteTokens(tokens []string) string {
	var parts []string
	optCount := 0
	for _, t := range tokens {
		switch t {
		case idePasteCmdShiftE:
			parts = append(parts, "⌘⇧E (Explorer)")
		case idePasteTokenEscape:
			parts = append(parts, "Escape")
		case idePasteTokenCmdL:
			parts = append(parts, "⌘L")
		case idePasteTokenCmdY:
			parts = append(parts, "⌘Y")
		case idePasteOptionCmdE:
			optCount++
		}
	}
	out := strings.Join(parts, ", ")
	if optCount == 2 {
		if out != "" {
			out += ", "
		}
		out += "⌥⌘E×2"
	} else if optCount == 1 {
		if out != "" {
			out += ", "
		}
		out += "⌥⌘E"
	} else if optCount > 2 {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("⌥⌘E×%d", optCount)
	}
	return out
}

// validateBuiltIDEPasteScript checks the generated AppleScript matches the requested token keystrokes.
func validateBuiltIDEPasteScript(s string, tokens []string) error {
	want := map[string]int{
		idePasteTokenCmdY:       0,
		idePasteCmdShiftE:  0,
		idePasteTokenEscape:     0,
		idePasteTokenCmdL:       0,
		idePasteOptionCmdE: 0,
	}
	for _, t := range tokens {
		want[t]++
	}
	if got := strings.Count(s, `keystroke "y" using command down`); got != want[idePasteTokenCmdY] {
		return errfmt.Errorf("internal: cmd_y keystroke count mismatch: got %d want %d", got, want[idePasteTokenCmdY])
	}
	if got := strings.Count(s, `keystroke "e" using {command down, shift down}`); got != want[idePasteCmdShiftE] {
		return errfmt.Errorf("internal: cmd_shift_e keystroke count mismatch: got %d want %d", got, want[idePasteCmdShiftE])
	}
	if got := strings.Count(s, "key code 53"); got != want[idePasteTokenEscape] {
		return errfmt.Errorf("internal: escape key code count mismatch: got %d want %d", got, want[idePasteTokenEscape])
	}
	if got := strings.Count(s, `keystroke "l" using command down`); got != want[idePasteTokenCmdL] {
		return errfmt.Errorf("internal: cmd_l keystroke count mismatch: got %d want %d", got, want[idePasteTokenCmdL])
	}
	if got := strings.Count(s, `keystroke "e" using {command down, option down}`); got != want[idePasteOptionCmdE] {
		return errfmt.Errorf("internal: option_cmd_e keystroke count mismatch: got %d want %d", got, want[idePasteOptionCmdE])
	}
	return nil
}
