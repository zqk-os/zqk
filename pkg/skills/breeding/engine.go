package breeding

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// MutationProvider is responsible for creating a physical fork of a skill's backing files.
type MutationProvider interface {
	// MutateSkill takes the original file path and performance feedback,
	// generates a mutated version, and returns the new file path.
	MutateSkill(ctx context.Context, originalFilePath string, feedback []string) (string, error)
}

// Engine implements the Automated Skill Breeding Protocol.
type Engine struct {
	store    storage.ObjectStorageProvider
	mutator  MutationProvider
	fitnessT float64
}

// NewEngine constructs a new Breeding Engine.
func NewEngine(store storage.ObjectStorageProvider, mutator MutationProvider, fitnessThreshold float64) *Engine {
	return &Engine{
		store:    store,
		mutator:  mutator,
		fitnessT: fitnessThreshold,
	}
}

// Breed scans for underperforming agent skills and mutates them.
func (e *Engine) Breed(ctx context.Context) error {
	log := logging.GetLoggerFromContext(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	// List all agent skills
	skillsResult, err := e.store.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	if err != nil {
		return fmt.Errorf("failed to list agent skills: %w", err)
	}

	for _, skillObj := range skillsResult.Objects {
		skillID, ok := skillObj[objects.FieldKeyID].(string)
		if !ok || skillID == "" {
			continue
		}

		// Find maturation reports for this skill
		reportsResult, err := e.store.List(ctx, secCtx, nil, storage.ListFilter{
			Kind: objects.MaturationReport,
			Filters: map[string]interface{}{
				objects.FieldKeyComponentID: skillID,
			},
		})
		if err != nil {
			log.LogWarning("failed to get maturation reports", logging.String("skill_id", skillID), logging.Error(err))
			continue
		}

		if len(reportsResult.Objects) == 0 {
			continue // No performance data available
		}

		// Find the lowest fitness score among reports
		var lowestFitness = 1.0
		var found bool
		for _, r := range reportsResult.Objects {
			if f, ok := r[objects.FieldKeyFitnessScore].(float64); ok && f < lowestFitness {
				lowestFitness = f
				found = true
			}
		}

		// If the fitness is below threshold, trigger a mutation
		if found && lowestFitness < e.fitnessT {
			log.LogInfo("skill requires mutation due to low fitness", logging.String("skill_id", skillID), logging.String("fitness", fmt.Sprintf("%f", lowestFitness)))

			feedback := []string{fmt.Sprintf("Fitness %f below threshold %f", lowestFitness, e.fitnessT)}

			// Optional: We can also query metrics_feedback objects for deeper insights
			mfbResult, err := e.store.List(ctx, secCtx, nil, storage.ListFilter{
				Kind: objects.KindMetricsFeedback,
				Filters: map[string]interface{}{
					objects.FieldKeyTargetReport: skillID, // simplified
				},
			})
			if err == nil {
				for _, m := range mfbResult.Objects {
					if analysis, ok := m[objects.FieldKeyAnalysis].(string); ok {
						feedback = append(feedback, analysis)
					}
				}
			}

			err = e.forkAndMutate(ctx, secCtx, skillObj, feedback)
			if err != nil {
				log.LogError("failed to fork and mutate skill", err, logging.String("skill_id", skillID))
			}
		}
	}
	return nil
}

func (e *Engine) forkAndMutate(ctx context.Context, secCtx *pkgctx.SecurityContext, original map[string]any, feedback []string) error {
	originalPath, ok := original[objects.FieldKeyFilePath].(string)
	if !ok {
		return fmt.Errorf("original skill has no file_path")
	}

	newPath, err := e.mutator.MutateSkill(ctx, originalPath, feedback)
	if err != nil {
		return fmt.Errorf("mutation failed: %w", err)
	}

	// Prepare new skill object.
	// The storage layer will auto-assign an ID if we don't provide one, assuming it supports it,
	// or we can generate one. Since we don't have ID generation directly, we'll let storage try to handle it.
	newSkill := map[string]any{
		objects.FieldKeyKind:     objects.KindAgentSkill,
		objects.FieldKeyProvider: original[objects.FieldKeyProvider],
		objects.FieldKeyFilePath: newPath,
		objects.FieldKeyInstructionsSummary: fmt.Sprintf("Mutated from %v due to: %s",
			original[objects.FieldKeyID], strings.Join(feedback, "; ")),
	}

	return e.store.Create(ctx, secCtx, newSkill)
}
