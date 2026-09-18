package evolution

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// FitnessAssessor evaluates the 'fitness' of shielded (eccentric) components.
type FitnessAssessor struct {
	store storage.ObjectStorageProvider
}

// NewFitnessAssessor creates a new FitnessAssessor.
func NewFitnessAssessor(store storage.ObjectStorageProvider) *FitnessAssessor {
	return &FitnessAssessor{store: store}
}

// Compare evaluates shadow events against mainline events to calculate a delta fitness score.
func (a *FitnessAssessor) Compare(ctx context.Context, componentID string, shadowEvents []infrastructure.Event, realEvents []infrastructure.Event) (float64, error) {
	// Heuristic: If shadow events proposed a valid remediation before the mainline,
	// or found a more efficient path, the fitness score increases.

	// For the prototype, we use a simple 'Differentiation Bonus'
	fitness := 0.5 // Baseline

	if len(shadowEvents) > 0 && len(realEvents) == 0 {
		// Shadow component sensed something the mainline missed!
		fitness += 0.3
	}

	if len(shadowEvents) > len(realEvents) {
		// More 'eccentric' activity detected
		fitness += 0.1
	}

	if fitness > 1.0 {
		fitness = 1.0
	}

	// Update maturation report
	report := map[string]any{
		objects.FieldKeyKind:              objects.MaturationReport,
		objects.FieldKeyID:                fmt.Sprintf("MAT-%s", componentID),
		objects.FieldKeyComponentID:       componentID,
		objects.FieldKeyFitnessScore:      fitness,
		objects.FieldKeyLastCalculationAt: zqktime.NowRFC3339UTC(),
		objects.FieldKeyGraduationStatus:  "maturation",
	}

	_ = a.store.Update(ctx, nil, report[objects.FieldKeyID].(string), report)
	_ = a.store.Create(ctx, nil, report)

	return fitness, nil
}
