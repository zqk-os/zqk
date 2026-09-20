package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/llm"
)

// GenerateDynamicTips reads from the system state and uses the LLM to generate 3 dynamic efficiency tips.
func GenerateDynamicTips(ctx context.Context, llmClient llm.Client, projectRoot string, systemStateSummary string) ([]string, error) {
	if llmClient == nil {
		llmClient = llm.NewClient(ctx, nil)
	}

	prompt := fmt.Sprintf(`You are the "ZQK Observer Coach", a real-time orchestrator monitoring the ZQK knowledge kernel and swarm architecture.
Your job is to read the current system state summary and provide EXACTLY 3 short, actionable, dynamic tips for the user/agent.
Focus on identifying any signs of policy violation, failure to orchestrate, maximizing swarm usage and multi-agent parallelism, or AST integrity.
Do not provide generic advice. Be specific to the current state.
Prefix each tip with "ZQK Observer Tip: ".

Current System State:
%s

Respond ONLY with a valid JSON array of 3 strings. Example:
[
  "ZQK Observer Tip: The test bundle for PRI-003 is blocked. Ensure 'zqk scheduler scan-tests' is running parallel jobs.",
  "ZQK Observer Tip: You have 3 backlog items in 'planned' but no active agents. Run 'zqk agent orchestrate' to parallelize the work."
]`, systemStateSummary)

	resp, err := llmClient.GenerateCompletion(ctx, prompt, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate tips: %w", err)
	}

	cleanedResp := cleanJSONResponse(resp)

	var tips []string
	if err := json.Unmarshal([]byte(cleanedResp), &tips); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tips: %w (raw: %s)", err, resp)
	}

	return tips, nil
}

func cleanJSONResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			cleaned = strings.Join(lines, "\n")
		}
	}
	return strings.TrimSpace(cleaned)
}

// ReadCachedTips reads the latest generated tips from the state file.
func ReadCachedTips(projectRoot string) []string {
	b, err := fileutil.ReadFile(paths.ObserverTipsPath(projectRoot))
	if err != nil {
		return nil
	}
	var tips []string
	if err := json.Unmarshal(b, &tips); err == nil && len(tips) > 0 {
		return tips
	}
	return nil
}

// FormatCachedTips returns the observer-tips block for prompts, or empty if none are cached.
func FormatCachedTips(projectRoot string) string {
	tips := ReadCachedTips(projectRoot)
	if len(tips) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n--- AST Observer Insights ---\n")
	for _, tip := range tips {
		sb.WriteString("- " + tip + "\n")
	}
	sb.WriteString("-----------------------------\n")
	return sb.String()
}

// WriteCachedTips writes the generated tips to the state file.
func WriteCachedTips(projectRoot string, tips []string) error {
	stateDir := paths.StateDirPath(projectRoot)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tips, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(paths.ObserverTipsPath(projectRoot), b)
}
