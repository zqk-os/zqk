package intake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	kernelintake "github.com/zqk-os/zqk/pkg/kernel/intake"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	useIntakeCommand        = "intake [context...]"
	shortIntakeCommand      = "Semantic ingestion pipeline (Intent Capture)"
	errFailedToReadStdin    = "failed to read from stdin"
	errNoContextProvided    = "no intent context provided. Please provide arguments or pipe content via stdin."
	statusSynthesizing      = "Synthesizing objects from input context via ambient Semantic Engine..."
	statusApplyingShockwave = "Applying Policy Shockwave Validation and saving..."
	msgNoObjectsExtracted   = "No objects were extracted from the provided context."
	msgIntakeCancelled      = "Intake cancelled."
	msgMissingDescription   = "description is required on base_object: fail-closed CAS intake gate"
)

// IntakeObject is a single object synthesized by the semantic engine
// during intake. Description is REQUIRED (base_object description gate):
// objects without a non-empty description are rejected fail-closed.
type IntakeObject struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// IntakeResponse is the semantic engine's JSON output: either a
// clarification question or a batch of objects.
type IntakeResponse struct {
	ClarificationQuestion string         `json:"clarification_question,omitempty"`
	Objects               []IntakeObject `json:"objects,omitempty"`
}

// ValidateIntakeObject enforces the base_object description gate on intake
// payloads. It is fail-closed: any missing/blank field rejects the object.
// Intake must never persist an object that would violate the base_object
// description requirement downstream.
func ValidateIntakeObject(obj IntakeObject) error {
	if strings.TrimSpace(obj.Kind) == "" {
		return errfmt.Errorf("missing kind")
	}
	if strings.TrimSpace(obj.Title) == "" {
		return errfmt.Errorf("missing title")
	}
	if strings.TrimSpace(obj.Description) == "" {
		return errfmt.Errorf("%s (title: %q)", msgMissingDescription, obj.Title)
	}
	return nil
}

// NewIntakeCmd returns the intake command
func NewIntakeCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewIntakeCommandBuilder(), &cobra.Command{
		Use:   useIntakeCommand,
		Short: shortIntakeCommand,
		Long: paths.RewriteCanonicalCLIInvocations(`Intake parses raw contextual text or key-value pairs into structured ZQK objects
using the configured ambient semantic engine (LLM). It natively supports bulk operations.
Every synthesized object must carry a non-empty description (base_object
description gate) — objects without one are rejected fail-closed and the
intake run aborts rather than persisting incomplete objects.

Examples:
  zqk intake "We need a new sqlite connection and a metrics exporter"
  cat meeting_notes.md | zqk intake`),
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			ctx := proc.OperationContext()
			log := proc.Logger()
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			var inputContext string

			if len(args) > 0 {
				inputContext = strings.Join(args, " ")
			} else {
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & fileutil.ModeCharDevice) == 0 {
					bytes, err := io.ReadAll(os.Stdin)
					if err != nil {
						return errfmt.Newf(errFailedToReadStdin).Wrap(err)
					}
					inputContext = string(bytes)
				}
			}

			inputContext = strings.TrimSpace(inputContext)
			if inputContext == "" {
				return errfmt.Errorf(errNoContextProvided)
			}

			client := llm.NewClient(ctx, llm.DefaultConfig(ctx))

			systemPrompt := `You are the ZQK Semantic Ingestion Engine. 
Your task is to parse raw context into discrete, strictly-typed ZQK system objects.
If the provided context is too vague, ambiguous, or lacks enough detail to create meaningful objects, you MUST ask a clarification question.
When asking a clarification question, be specific about what information is missing. You may also provide partial objects if some parts are clear but others are not.
EVERY object you emit MUST include a non-empty "description" field. Objects without a description are rejected by the fail-closed CAS intake gate.
Return ONLY a valid JSON object with the following structure. Do not include markdown formatting or backticks.
{
  "clarification_question": "Ask a specific question here if the intent is too vague or ambiguous (optional)",
  "objects": [
    {
      "kind": "requirement",
      "title": "Clear title",
      "description": "Clear description"
    }
  ]
}
Valid kinds usually include: requirement, workstream, goal, technical_spec, strategic_plan.`

			var parsedObjects []IntakeObject

			// Interactive Clarification Loop (In-Session)
			for {
				fmt.Fprintln(errOut, statusSynthesizing)
				prompt := fmt.Sprintf("Extract ZQK objects from the following context:\n\n%s", inputContext)

				resp, err := client.GenerateCompletion(ctx, prompt, systemPrompt)
				if err != nil {
					return errfmt.Newf("semantic engine failed to synthesize intent").Wrap(err)
				}

				// Clean up potential markdown formatting
				resp = strings.TrimSpace(resp)
				resp = strings.TrimPrefix(resp, "```json")
				resp = strings.TrimPrefix(resp, "```")
				resp = strings.TrimSuffix(resp, "```")
				resp = strings.TrimSpace(resp)

				var intakeResp IntakeResponse
				if err := json.Unmarshal([]byte(resp), &intakeResp); err != nil {
					logging.FluentEvent(log).Error("Failed to parse semantic engine output", err).Log()
					fmt.Fprintf(errOut, "Raw output was:\n%s\n", resp)
					return errfmt.Newf("failed to unmarshal JSON from semantic engine").Wrap(err)
				}

				// If the semantic engine needs clarification, prompt the user
				if intakeResp.ClarificationQuestion != "" {
					if len(intakeResp.Objects) > 0 {
						fmt.Fprintf(out, "\nI parsed %d object(s), but need more information.\n", len(intakeResp.Objects))
					}
					fmt.Fprintf(out, "\nClarification needed: %s\n> ", intakeResp.ClarificationQuestion)

					scanner := bufio.NewScanner(os.Stdin)
					scanner.Scan()
					userInput := strings.TrimSpace(scanner.Text())

					if userInput == "" || strings.ToLower(userInput) == "cancel" {
						fmt.Fprintln(out, msgIntakeCancelled)
						return nil
					}

					// Append the clarification context
					inputContext = fmt.Sprintf("%s\n\nClarification: %s -> %s", inputContext, intakeResp.ClarificationQuestion, userInput)
					continue
				}

				parsedObjects = intakeResp.Objects
				break
			}

			if len(parsedObjects) == 0 {
				fmt.Fprintln(out, msgNoObjectsExtracted)
				return nil
			}

			// Fail-closed CAS intake gate: pre-validate the entire batch against
			// the base_object description gate before any object is persisted.
			// A single rejected object aborts the whole run.
			if gateErr := ValidateAllIntakeObjects(parsedObjects); gateErr != nil {
				fmt.Fprintln(out, "Intake rejected (fail-closed CAS gate). No objects were persisted.")
				return gateErr
			}

			clusterFlag, _ := cmd.Flags().GetBool("cluster")
			concurrencyFlag, _ := cmd.Flags().GetBool("concurrency")
			strictAntiChain, _ := cmd.Flags().GetBool("strict-anti-chain")

			if clusterFlag || concurrencyFlag || len(parsedObjects) >= 2 {
				synthesis, synthErr := SynthesizeIntakeMembrane(parsedObjects, strictAntiChain)
				if synthErr != nil {
					return errfmt.Errorf("intake membrane processing failed: %w", synthErr)
				}

				if clusterFlag || concurrencyFlag {
					fmt.Fprintf(out, "=== Intake Membrane Synthesis ===\n")
					fmt.Fprintf(out, "Total Inputs: %d | Clusters: %d | Deduplicated/Merged: %d\n\n",
						synthesis.TotalInputCount, len(synthesis.Clusters), synthesis.DeduplicatedCount)

					for i, c := range synthesis.Clusters {
						topo := synthesis.Topologies[i]
						fmt.Fprintf(out, "Cluster %d [%s]: %s (Domain: %s, Mode: %s)\n",
							i+1, c.ClusterID, c.Title, c.DomainCategory, topo.Mode)
						if concurrencyFlag {
							fmt.Fprintf(out, "  Batches: %v\n", topo.Batches)
							if len(topo.Dependencies) > 0 {
								fmt.Fprintf(out, "  Dependencies: %v\n", topo.Dependencies)
							}
						}
					}
					fmt.Fprintln(out)
				}
			}

			fmt.Fprintf(out, "\nSynthesized %d objects. %s\n\n", len(parsedObjects), statusApplyingShockwave)

			// Instantiate objects (Bulk commit)
			for i, obj := range parsedObjects {
				// Re-check at the persistence boundary; the gate is fail-closed.
				if gateErr := ValidateIntakeObject(obj); gateErr != nil {
					return fmt.Errorf("object %d rejected at intake gate: %w", i+1, gateErr)
				}

				// Validate kind existence
				kind, resolveErr := objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), obj.Kind)
				if resolveErr != nil {
					return fmt.Errorf("failed to resolve kind '%s': %w", obj.Kind, resolveErr)
				}

				payload := map[string]any{
					objects.FieldKeyKind:        kind,
					objects.FieldKeyTitle:       obj.Title,
					objects.FieldKeyDescription: obj.Description,
					objects.FieldKeyStatus:      objects.ObjectStatusPlanned, // Create in planned state to avoid immediate bzzzt
				}

				if err := proc.Storage().Create(ctx, proc.SecurityContext(), payload); err != nil {
					return fmt.Errorf("failed to create [%s] '%s': %w", kind, obj.Title, err)
				}
				// Get ID if generated by storage
				id, _ := payload[objects.FieldKeyID].(string)
				fmt.Fprintf(out, "Created [%s] %s (ID: %s)\n", kind, obj.Title, id)
			}

			return nil
		}),
	})

	cmd.Flags().Bool("cluster", false, "Group synthesized intake requests into cohesive workstreams via IntakeMembrane")
	cmd.Flags().Bool("concurrency", false, "Preview and output synthesized concurrency vs sequential execution pipelines")
	cmd.Flags().Bool("strict-anti-chain", true, "Reject redundant 1:1 micro-chain proposals fail-closed")

	return cmd
}

// SynthesizeIntakeMembrane executes the two-stage intake clustering and concurrency synthesis
// pipeline on a batch of synthesized intake objects.
func SynthesizeIntakeMembrane(objects []IntakeObject, strictAntiChain bool) (*kernelintake.IntakeSynthesisResult, error) {
	if len(objects) == 0 {
		return nil, kernelintake.ErrEmptyIntakeRequests
	}

	reqs := make([]kernelintake.IntakeRequest, len(objects))
	for i, obj := range objects {
		targetPaths := extractTargetPaths(obj.Description)
		reqs[i] = kernelintake.IntakeRequest{
			ID:             fmt.Sprintf("REQ-INTAKE-%03d", i+1),
			Title:          obj.Title,
			Description:    obj.Description,
			DomainCategory: obj.Kind,
			TargetPaths:    targetPaths,
		}
	}

	membrane := kernelintake.NewIntakeMembrane(kernelintake.WithStrictAntiChain(strictAntiChain))
	return membrane.ProcessIntake(reqs)
}

func extractTargetPaths(text string) []string {
	words := strings.Fields(text)
	var paths []string
	for _, w := range words {
		cleaned := strings.Trim(w, "(),;:\"'`")
		if strings.HasPrefix(cleaned, "pkg/") || strings.HasPrefix(cleaned, "cmd/") || strings.HasPrefix(cleaned, "internal/") {
			paths = append(paths, cleaned)
		}
	}
	return paths
}

// ValidateAllIntakeObjects enforces the base_object description gate across
// an entire intake batch. It is fail-closed: any rejected object aborts the
// whole batch, and the returned error carries the offending objects for
// operator review.
func ValidateAllIntakeObjects(objs []IntakeObject) error {
	if len(objs) == 0 {
		return nil
	}
	var rejected []string
	for i, obj := range objs {
		if err := ValidateIntakeObject(obj); err != nil {
			rejected = append(rejected, fmt.Sprintf("object %d (kind=%q, title=%q): %v", i+1, obj.Kind, obj.Title, err))
		}
	}
	if len(rejected) == 0 {
		return nil
	}
	return fmt.Errorf("fail-closed CAS intake gate rejected %d of %d object(s): %s",
		len(rejected), len(objs), strings.Join(rejected, "; "))
}
