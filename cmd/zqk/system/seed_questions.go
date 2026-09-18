package system

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Seed questions for project discovery per docs/architecture/initialization-seed-questions-v1.0.md (BLI-774).

const seedQuestionsEssentialFile = "essential.yaml"

// defaultEssentialSeedQuestions returns the built-in essential seed questions (SEED-001 through SEED-005).
func defaultEssentialSeedQuestions() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyID:           "SEED-001",
			objects.FieldKeyKind:         objects.KindQuestion,
			objects.FieldKeyQuestionText: "What is the project type?",
			objects.FieldKeyCategory:     "essential",
			"required":                   true,
			"question_type":              "single_choice",
			"options": []string{
				"Individual project (single developer)",
				"Team project (small team, 2-10 people)",
				"Startup project (growing team, 10-50 people)",
				"Enterprise project (large organization, 50+ people)",
				"Partnership project (multiple organizations)",
			},
			"interpretation": map[string]any{
				"individual":            map[string]any{"customer_type": "individual", "complexity": "minimal"},
				objects.KindTeam:        map[string]any{"customer_type": "small_team", "complexity": "standard"},
				"startup":               map[string]any{"customer_type": "startup", "complexity": "standard"},
				"enterprise":            map[string]any{"customer_type": "enterprise", "complexity": "enterprise"},
				objects.KindPartnership: map[string]any{"customer_type": objects.KindPartnership, "complexity": "enterprise"},
			},
		},
		{
			objects.FieldKeyID:           "SEED-002",
			objects.FieldKeyKind:         objects.KindQuestion,
			objects.FieldKeyQuestionText: "Who owns the code repository?",
			objects.FieldKeyCategory:     "essential",
			"required":                   true,
			"question_type":              "text",
			"interpretation": map[string]any{
				objects.FieldKeyField: "code_owner",
				"usage":               "Determines primary authority and ownership",
			},
		},
		{
			objects.FieldKeyID:           "SEED-003",
			objects.FieldKeyKind:         objects.KindQuestion,
			objects.FieldKeyQuestionText: "Who are the primary stakeholders?",
			objects.FieldKeyCategory:     "essential",
			"required":                   true,
			"question_type":              "multi_text",
			"interpretation": map[string]any{
				objects.FieldKeyField: "primary_stakeholders",
				"usage":               "Creates stakeholder profiles",
			},
		},
		{
			objects.FieldKeyID:           "SEED-004",
			objects.FieldKeyKind:         objects.KindQuestion,
			objects.FieldKeyQuestionText: "What are the initial project goals?",
			objects.FieldKeyCategory:     "essential",
			"required":                   true,
			"question_type":              "multi_text",
			"interpretation": map[string]any{
				objects.FieldKeyField: "initial_goals",
				"usage":               "Creates initial goal objects",
			},
		},
		{
			objects.FieldKeyID:           "SEED-005",
			objects.FieldKeyKind:         objects.KindQuestion,
			objects.FieldKeyQuestionText: "What is the project's primary domain or industry?",
			objects.FieldKeyCategory:     "essential",
			"required":                   true,
			"question_type":              "text",
			"interpretation": map[string]any{
				objects.FieldKeyField: "domain",
				"usage":               "Configures domain-specific policies and templates",
			},
		},
	}
}

// NewSeedQuestionsCmd creates the system seed-questions command (BLI-774).
func NewSeedQuestionsCmd() *cobra.Command {
	var generate bool

	helpBuilder := clipkg.DynamicHelpBuilder(
		"List or generate seed questions for project discovery",
		"Lists the default essential seed questions, or writes them to .zqk/seed/questions/ for use during init or discovery.",
		"",
		"Use --generate to create .zqk/seed/questions/essential.yaml in the project root.",
	).
		AddExample("List essential seed questions", "%s system seed-questions").
		AddExample("Generate question files", "%s system seed-questions --generate").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSeedQuestionsCommandBuilder(), &cobra.Command{
		Use: "seed-questions",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runSeedQuestions(cmd, generate)
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)
	cmd.Flags().BoolVar(&generate, "generate", false, "Write .zqk/seed/questions/essential.yaml with default essential questions")

	return cmd
}

func runSeedQuestions(cmd *cobra.Command, generate bool) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	questions := defaultEssentialSeedQuestions()
	result := map[string]any{objects.FieldKeyQuestions: questions, "count": len(questions)}

	if generate {
		if projectRoot == emptyValue {
			return errfmt.Errorf("not a ZQK project (no project root found); run from project root or set ZQK_PROJECT_ROOT")
		}
		questionsDir := filepath.Join(projectRoot, paths.SeedQuestionsDir)
		if err := fileutil.MkdirAll(questionsDir, paths.DirPerm755); err != nil {
			return errfmt.Newf("create seed questions dir").Wrap(err)
		}
		outPath := filepath.Join(questionsDir, seedQuestionsEssentialFile)
		payload := map[string]any{objects.FieldKeyQuestions: questions}
		data, err := yaml.Marshal(payload)
		if err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("seed-questions YAML marshal", err).Log()
			return err
		}
		if err := fileutil.WriteFile(outPath, data, paths.FilePerm600); err != nil {
			return errfmt.Errorf("write %s: %w", outPath, err)
		}
		result["generated"] = outPath
		result["message"] = fmt.Sprintf("Wrote %d essential seed questions to %s", len(questions), outPath)
	}

	format := cli.GetFormat(cmd)
	if format == cli.FormatTable {
		return outputSeedQuestionsTable(cmd, result)
	}
	// JSON/YAML via shared format handler (CLI_ARCHITECTURE.md)
	if err := cli.FormatOutput(cmd, result); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("seed-questions format output", err).Log()
		return err
	}
	return nil
}

func outputSeedQuestionsTable(cmd *cobra.Command, result map[string]any) error {
	var buf strings.Builder
	questions, _ := result[objects.FieldKeyQuestions].([]map[string]any)
	count, _ := result["count"].(int)
	if msg, ok := result["message"].(string); ok && msg != emptyValue {
		buf.WriteString(msg + "\n\n")
	}
	buf.WriteString("Essential seed questions\n")
	buf.WriteString("------------------------\n")
	fmt.Fprintf(&buf, "  Count: %d\n\n", count)
	for _, q := range questions {
		id, _ := q[objects.FieldKeyID].(string)
		text, _ := q[objects.FieldKeyQuestionText].(string)
		qtype, _ := q["question_type"].(string)
		fmt.Fprintf(&buf, "  %s [%s]\n    %s\n", id, qtype, text)
	}
	return cli.WriteOutput(cmd, []byte(buf.String()))
}
