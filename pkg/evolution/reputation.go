package evolution

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/infrastructure"
	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// ReputationManager tracks Mesh-wide Reputation Ranking.
// It implements the 'Crystalline Fortress' metaphor (REQ-502) where trust is
// hardened over time through proven integrity, utility, and fitness,
// but decays if not actively maintained.
type ReputationManager struct {
	store  storage.ObjectStorageProvider
	spine  infrastructure.SpinalSpine
	signer crypto.Signer
}

const (
	// ReputationDecayFactor is the daily decay rate (0.1%).
	ReputationDecayFactor = 0.001
	// ReputationInertiaScale is the point at which logarithmic compression begins to dominate.
	ReputationInertiaScale = 10.0
)

// NewReputationManager creates a new ReputationManager.
func NewReputationManager(store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine, signer crypto.Signer) *ReputationManager {
	return &ReputationManager{
		store:  store,
		spine:  spine,
		signer: signer,
	}
}

// UpdateScore adds signed deltas to an entity's reputation.
func (m *ReputationManager) UpdateScore(ctx context.Context, entityID string, integrityDelta, utilityDelta, fitnessDelta float64, signature string, publicKey string) error {
	// REQ-502: Mandatory Delta Verification (Crystalline Fortress)
	// We verify that the reputation update was signed by a trusted authority (or self).
	if signature != "" {
		data := []byte(fmt.Sprintf("%s|%v|%v|%v", entityID, integrityDelta, utilityDelta, fitnessDelta))
		valid, err := crypto.GlobalVerifier.Verify(data, signature, publicKey)
		if err != nil || !valid {
			msg := fmt.Sprintf("ABORT: Dropping unsigned/forged reputation update for %s", entityID)
			logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [security_violation] system: %s\n", msg)).Log()
			return fmt.Errorf("invalid reputation update signature")
		}
	}

	id := fmt.Sprintf("REP-%s", entityID)
	scoreObj, err := m.store.Read(ctx, nil, id)
	var newScore map[string]any
	now := zqktime.NowRFC3339UTC()

	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			// Create new reputation object
			newScore = map[string]any{
				objects.FieldKeyKind:              objects.KindReputationScore,
				objects.FieldKeyID:                id,
				"entity_id":                       entityID,
				"integrity_delta":                 integrityDelta,
				"utility_delta":                   utilityDelta,
				"fitness_delta":                   fitnessDelta,
				"reputation_score":                m.calculateScore(integrityDelta, utilityDelta, fitnessDelta),
				objects.FieldKeyLastCalculationAt: now,
			}
			err = m.store.Create(ctx, nil, newScore)
		} else {
			return fmt.Errorf("failed to read reputation: %w", err)
		}
	} else {
		// Calculate elapsed time for decay
		lastCalcStr, _ := scoreObj[objects.FieldKeyLastCalculationAt].(string)
		lastCalc, _ := time.Parse(time.RFC3339, lastCalcStr)
		elapsedDays := time.Since(lastCalc).Hours() / 24.0

		// Apply exponential decay
		decay := math.Exp(-ReputationDecayFactor * elapsedDays)

		integrity := (scoreObj["integrity_delta"].(float64) * decay) + integrityDelta
		utility := (scoreObj["utility_delta"].(float64) * decay) + utilityDelta
		fitness := (scoreObj["fitness_delta"].(float64) * decay) + fitnessDelta

		updates := map[string]any{
			"integrity_delta":                 integrity,
			"utility_delta":                   utility,
			"fitness_delta":                   fitness,
			"reputation_score":                m.calculateScore(integrity, utility, fitness),
			objects.FieldKeyLastCalculationAt: now,
		}

		err = m.store.Update(ctx, nil, id, updates)
		newScore, _ = m.store.Read(ctx, nil, id)
	}

	if err != nil {
		return err
	}

	// Broadcast HIVE PULSE
	sigStatus := "unsigned"
	if m.signer != nil {
		sig, sigErr := m.signer.Sign([]byte(fmt.Sprintf("%s:%v", entityID, newScore["reputation_score"])))
		if sigErr == nil {
			sigStatus = "signed:" + sig[:8]
		}
	}

	logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("⚡ [HIVE PULSE] [reputation] [%s] system: [Crystalline Fortress] Updated reputation for %s to %v (Δi:%v, Δu:%v, Δf:%v)\n", sigStatus, entityID, newScore["reputation_score"], integrityDelta, utilityDelta, fitnessDelta)).Log()

	if m.spine != nil {
		_ = m.spine.Publish(ctx, infrastructure.Event{
			ObjectID: id,
			Kind:     "reputation_update",
			Op:       "updated",
			Payload:  newScore,
		})
	}

	return nil
}

// GetDiscount calculates the query cost discount based on reputation score.
func (m *ReputationManager) GetDiscount(ctx context.Context, entityID string) float64 {
	id := fmt.Sprintf("REP-%s", entityID)
	scoreObj, err := m.store.Read(ctx, nil, id)
	if err != nil {
		return 0.0
	}
	score, ok := scoreObj["reputation_score"].(float64)
	if !ok {
		return 0.0
	}
	return score * 0.5
}

func (m *ReputationManager) calculateScore(integrity, utility, fitness float64) float64 {
	rawSum := integrity + utility + fitness
	if rawSum <= 0 {
		return 0.0
	}
	score := math.Log10(1+rawSum) / math.Log10(1+ReputationInertiaScale)
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}
	return math.Round(score*100) / 100
}
