package llmtranscript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	reExitCode     = regexp.MustCompile(`Command exited with code (\d+)`)
	reErrorPattern = regexp.MustCompile(`(?i)(syntax error|fatal:|error:|no such file|command not found|target not found|cannot find|failed with|permission denied|invalid option)`)
)

const maxScanTokenSize = 16 * 1024 * 1024 // 16MB buffer for large diffs

// ParseTranscript streams and parses lines from an io.Reader into TranscriptStep slices.
func ParseTranscript(r io.Reader) ([]TranscriptStep, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)

	var steps []TranscriptStep
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var step TranscriptStep
		if err := json.Unmarshal([]byte(line), &step); err != nil {
			return nil, fmt.Errorf("line %d: malformed json: %w", lineNum, err)
		}
		steps = append(steps, step)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return steps, nil
}

// ParseTranscriptFile reads a transcript file from disk.
func ParseTranscriptFile(filePath string) ([]TranscriptStep, error) {
	f, err := fileutil.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseTranscript(f)
}

// ExtractTurns walks through transcript steps and pairs model planner responses
// with the resulting tool executions, computing turn-level correctness and recovery metrics.
func ExtractTurns(steps []TranscriptStep, defaultModel string) []Turn {
	var turns []Turn
	var lastTurnFailed bool

	for i := 0; i < len(steps); i++ {
		step := steps[i]
		if step.Type != "PLANNER_RESPONSE" || step.Source != "MODEL" {
			continue
		}

		turnModel := strings.TrimSpace(defaultModel)
		if turnModel == "" {
			turnModel = UnlabeledModel
		}

		turn := Turn{
			StepIndex:       step.StepIndex,
			Model:           turnModel,
			InputTokens:     step.InputTokens,
			OutputTokens:    step.OutputTokens,
			CacheReadTokens: step.CacheReadTokens,
			ToolCalls:       step.ToolCalls,
		}

		// Find tool execution results that follow this planner response before the next planner response or user input.
		allExecutionsSuccess := true
		for j := i + 1; j < len(steps); j++ {
			nextStep := steps[j]
			if nextStep.Type == "PLANNER_RESPONSE" || nextStep.Type == "USER_INPUT" {
				break
			}

			// Tool responses typically have type "GENERIC" or "SYSTEM_MESSAGE" with content
			exec := evaluateToolExecution(nextStep, step.ToolCalls)
			if exec != nil {
				turn.Executions = append(turn.Executions, *exec)
				if !exec.Success {
					allExecutionsSuccess = false
				}
			}
		}

		if len(turn.Executions) > 0 {
			turn.IsSuccess = allExecutionsSuccess
		} else {
			// If no tools were called, check step status
			turn.IsSuccess = step.Status == "DONE"
		}

		if lastTurnFailed {
			turn.IsSelfCorrection = true
			if turn.IsSuccess {
				turn.CorrectedPrevious = true
			}
		}

		lastTurnFailed = !turn.IsSuccess
		turns = append(turns, turn)
	}

	return turns
}

// evaluateToolExecution assesses a single tool response step against known tool signatures.
func evaluateToolExecution(step TranscriptStep, activeToolCalls []ToolCall) *ToolExecution {
	if step.Content == "" && step.Error == "" && step.Status == "" {
		return nil
	}

	var toolName string
	var args map[string]any
	if len(activeToolCalls) > 0 {
		toolName = activeToolCalls[0].Name
		args = activeToolCalls[0].Args
	} else {
		toolName = "generic"
	}

	exec := &ToolExecution{
		ToolName:  toolName,
		Args:      args,
		StepIndex: step.StepIndex,
		Success:   true,
		ExitCode:  0,
	}

	if step.Status == "ERROR" {
		exec.Success = false
		exec.ErrorMessage = step.Error
		if exec.ErrorMessage == "" {
			exec.ErrorMessage = "step status marked ERROR"
		}
		return exec
	}

	// Parse Command exited with code N
	if matches := reExitCode.FindStringSubmatch(step.Content); len(matches) > 1 {
		code, err := strconv.Atoi(matches[1])
		if err == nil {
			exec.ExitCode = code
			if code != 0 {
				exec.Success = false
				exec.ErrorMessage = fmt.Sprintf("command exited with code %d", code)
			}
		}
	}

	// Detect typical runtime and tool failure patterns
	contentLower := strings.ToLower(step.Content)
	if strings.Contains(contentLower, "error:") ||
		strings.Contains(contentLower, "target not found") ||
		strings.Contains(contentLower, "syntax error") ||
		strings.Contains(contentLower, "permission denied") ||
		strings.Contains(contentLower, "fatal:") {
		exec.Success = false
		if exec.ErrorMessage == "" {
			lines := strings.Split(step.Content, "\n")
			for _, l := range lines {
				lTrim := strings.TrimSpace(l)
				if reErrorPattern.MatchString(lTrim) {
					exec.ErrorMessage = lTrim
					break
				}
			}
			if exec.ErrorMessage == "" && len(lines) > 0 {
				exec.ErrorMessage = strings.TrimSpace(lines[0])
			}
		}
	}

	return exec
}

// DetectModelFromSettings attempts to read the active model from the user's settings.json.
func DetectModelFromSettings(appDataDir string) string {
	settingsPath := filepath.Join(appDataDir, "settings.json")
	data, err := fileutil.ReadFile(settingsPath)
	if err != nil {
		return ""
	}

	var s struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &s); err == nil && s.Model != "" {
		return s.Model
	}
	return ""
}
