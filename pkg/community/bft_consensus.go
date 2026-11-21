package community

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

// ConsensusPhase represents a deterministic phase in the BFT commit state machine.
type ConsensusPhase string

const (
	// PhasePrepare is the initial proposal-binding phase.
	PhasePrepare ConsensusPhase = "prepare"
	// PhasePreCommit locks the quorum onto a prepared digest.
	PhasePreCommit ConsensusPhase = "pre_commit"
	// PhaseCommit gathers final commit signatures before finalization.
	PhaseCommit ConsensusPhase = "commit"
	// PhaseFinalized indicates immutable finality for a height.
	PhaseFinalized ConsensusPhase = "finalized"
)

const (
	errBlankValidatorID   = "validator id cannot be blank"
	errUnknownValidator   = "validator is not registered"
	errValidatorSlashed   = "validator is slashed"
	errRoundNotFound      = "consensus round not found"
	errRoundFinalized     = "consensus round already finalized"
	errDigestMismatch     = "vote digest does not match active proposal"
	errInvalidPhaseOrder  = "vote phase does not match active round phase"
	errEquivocationDetect = "equivocation detected and validator slashed"
)

// Validator defines a consensus participant in the planetary swarm.
type Validator struct {
	ID        string
	Region    string
	PublicKey string
	Slashed   bool
}

// QuorumSlice defines a subset of validators and its local agreement threshold.
type QuorumSlice struct {
	ID        string
	Members   []string
	Threshold int
}

// BFTConfig configures the validator set, fault tolerance, and quorum slices.
type BFTConfig struct {
	Validators []Validator
	Slices     []QuorumSlice
	MaxFaults  int
	Threshold  int
}

// Proposal represents a block or state transition proposed for a (Height, View).
type Proposal struct {
	Height     uint64
	View       uint64
	Digest     string
	ProposerID string
	CreatedAt  time.Time
}

// Vote represents a validator's signed vote for a proposal in a specific phase.
type Vote struct {
	Height         uint64
	View           uint64
	Phase          ConsensusPhase
	ProposalDigest string
	ValidatorID    string
	Signature      string
	Timestamp      time.Time
}

// EquivocationEvidence captures verifiable proof of Byzantine double-signing.
type EquivocationEvidence struct {
	ValidatorID  string
	Height       uint64
	View         uint64
	Phase        ConsensusPhase
	FirstDigest  string
	SecondDigest string
	DetectedAt   time.Time
}

type roundKey struct {
	height uint64
	view   uint64
}

type roundState struct {
	proposal Proposal
	phase    ConsensusPhase
	votes    map[ConsensusPhase]map[string]Vote
}

// BFTEngine executes multi-phase BFT consensus with quorum slicing and slashing.
type BFTEngine struct {
	mu           sync.RWMutex
	validators   map[string]*Validator
	slices       []QuorumSlice
	threshold    int
	maxFaults    int
	rounds       map[roundKey]*roundState
	finalized    map[uint64]string
	equivocation []EquivocationEvidence
}

// NewBFTEngine validates the BFT configuration and initializes a consensus engine.
func NewBFTEngine(cfg BFTConfig) (*BFTEngine, error) {
	if cfg.MaxFaults < 0 {
		return nil, errors.New("max faults cannot be negative")
	}
	minValidators := 3*cfg.MaxFaults + 1
	if len(cfg.Validators) < minValidators {
		return nil, fmt.Errorf("validator count %d below 3f+1 floor (%d)", len(cfg.Validators), minValidators)
	}
	validators, err := indexValidators(cfg.Validators)
	if err != nil {
		return nil, err
	}
	threshold, err := resolveThreshold(cfg.MaxFaults, cfg.Threshold, len(validators))
	if err != nil {
		return nil, err
	}
	if err := validateQuorumSlices(cfg.Slices, validators); err != nil {
		return nil, err
	}
	slicesCopy := make([]QuorumSlice, len(cfg.Slices))
	copy(slicesCopy, cfg.Slices)
	return &BFTEngine{
		validators: validators,
		slices:     slicesCopy,
		threshold:  threshold,
		maxFaults:  cfg.MaxFaults,
		rounds:     make(map[roundKey]*roundState),
		finalized:  make(map[uint64]string),
	}, nil
}

func indexValidators(list []Validator) (map[string]*Validator, error) {
	indexed := make(map[string]*Validator, len(list))
	for _, v := range list {
		if strutil.IsBlank(v.ID) {
			return nil, errors.New(errBlankValidatorID)
		}
		if _, exists := indexed[v.ID]; exists {
			return nil, fmt.Errorf("duplicate validator id: %s", v.ID)
		}
		vCopy := v
		indexed[v.ID] = &vCopy
	}
	return indexed, nil
}

func resolveThreshold(maxFaults, configured, validatorCount int) (int, error) {
	supermajority := 2*maxFaults + 1
	if configured == 0 {
		return supermajority, nil
	}
	if configured < supermajority {
		return 0, fmt.Errorf("threshold %d below 2f+1 supermajority (%d)", configured, supermajority)
	}
	if configured > validatorCount {
		return 0, fmt.Errorf("threshold %d exceeds validator count %d", configured, validatorCount)
	}
	return configured, nil
}

func validateQuorumSlices(slices []QuorumSlice, validators map[string]*Validator) error {
	memberSets := make([]map[string]struct{}, len(slices))
	for i, s := range slices {
		set, err := validateSingleSlice(s, validators)
		if err != nil {
			return err
		}
		memberSets[i] = set
	}
	for i := 0; i < len(slices); i++ {
		for j := i + 1; j < len(slices); j++ {
			if err := checkPairwiseIntersection(slices[i], memberSets[i], slices[j], memberSets[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSingleSlice(s QuorumSlice, validators map[string]*Validator) (map[string]struct{}, error) {
	if strutil.IsBlank(s.ID) {
		return nil, errors.New("quorum slice id cannot be blank")
	}
	if len(s.Members) == 0 {
		return nil, fmt.Errorf("quorum slice %s has no members", s.ID)
	}
	if s.Threshold < 1 || s.Threshold > len(s.Members) {
		return nil, fmt.Errorf("quorum slice %s has invalid threshold %d", s.ID, s.Threshold)
	}
	seen := make(map[string]struct{}, len(s.Members))
	for _, id := range s.Members {
		if _, ok := validators[id]; !ok {
			return nil, fmt.Errorf("quorum slice %s references unknown validator %s", s.ID, id)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("quorum slice %s has duplicate member %s", s.ID, id)
		}
		seen[id] = struct{}{}
	}
	return seen, nil
}

func checkPairwiseIntersection(a QuorumSlice, setA map[string]struct{}, b QuorumSlice, setB map[string]struct{}) error {
	overlap := 0
	for id := range setA {
		if _, ok := setB[id]; ok {
			overlap++
		}
	}
	requiredOverlap := (len(setA) - a.Threshold) + (len(setB) - b.Threshold) + 1
	if overlap == 0 || overlap < requiredOverlap {
		return fmt.Errorf("quorum slices %s and %s fail intersection invariant (overlap=%d, required=%d)", a.ID, b.ID, overlap, requiredOverlap)
	}
	return nil
}

// SubmitProposal registers a proposal for (Height, View) or slashes conflicting proposers.
func (e *BFTEngine) SubmitProposal(p Proposal) error {
	if p.Height == 0 || strutil.IsBlank(p.Digest) || strutil.IsBlank(p.ProposerID) {
		return errors.New("proposal requires non-zero height, digest, and proposer id")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.requireActiveValidatorLocked(p.ProposerID); err != nil {
		return err
	}
	key := roundKey{height: p.Height, view: p.View}
	if existing, ok := e.rounds[key]; ok {
		if existing.proposal.ProposerID == p.ProposerID && existing.proposal.Digest != p.Digest {
			e.slashLocked(p.ProposerID, p.Height, p.View, PhasePrepare, existing.proposal.Digest, p.Digest)
			return errors.New(errEquivocationDetect)
		}
		if existing.proposal.Digest != p.Digest {
			return errors.New("conflicting proposal already active for round")
		}
		return nil
	}
	e.rounds[key] = &roundState{
		proposal: p,
		phase:    PhasePrepare,
		votes:    make(map[ConsensusPhase]map[string]Vote),
	}
	return nil
}

// CastVote records a validator vote, detects equivocation, and advances consensus phases.
func (e *BFTEngine) CastVote(v Vote) (ConsensusPhase, error) {
	if err := validateVoteFields(v); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.requireActiveValidatorLocked(v.ValidatorID); err != nil {
		return "", err
	}
	round, ok := e.rounds[roundKey{height: v.Height, view: v.View}]
	if !ok {
		return "", errors.New(errRoundNotFound)
	}
	if round.phase == PhaseFinalized {
		return PhaseFinalized, errors.New(errRoundFinalized)
	}
	phaseVotes := e.ensurePhaseVotesLocked(round, v.Phase)
	if prev, seen := phaseVotes[v.ValidatorID]; seen {
		if prev.ProposalDigest != v.ProposalDigest {
			e.slashLocked(v.ValidatorID, v.Height, v.View, v.Phase, prev.ProposalDigest, v.ProposalDigest)
			delete(phaseVotes, v.ValidatorID)
			return round.phase, errors.New(errEquivocationDetect)
		}
		return round.phase, nil
	}
	if v.Phase != round.phase {
		return round.phase, errors.New(errInvalidPhaseOrder)
	}
	if v.ProposalDigest != round.proposal.Digest {
		return round.phase, errors.New(errDigestMismatch)
	}
	phaseVotes[v.ValidatorID] = v
	if e.hasQuorumLocked(phaseVotes) {
		round.phase = nextConsensusPhase(round.phase)
		if round.phase == PhaseFinalized {
			e.finalized[v.Height] = round.proposal.Digest
		}
	}
	return round.phase, nil
}

func validateVoteFields(v Vote) error {
	if v.Height == 0 {
		return errors.New("vote height must be greater than zero")
	}
	if strutil.IsBlank(v.ValidatorID) {
		return errors.New(errBlankValidatorID)
	}
	if strutil.IsBlank(v.ProposalDigest) || strutil.IsBlank(v.Signature) {
		return errors.New("vote requires non-blank digest and signature")
	}
	switch v.Phase {
	case PhasePrepare, PhasePreCommit, PhaseCommit:
		return nil
	default:
		return fmt.Errorf("invalid vote phase: %s", v.Phase)
	}
}

func (e *BFTEngine) requireActiveValidatorLocked(id string) error {
	v, ok := e.validators[id]
	if !ok {
		return errors.New(errUnknownValidator)
	}
	if v.Slashed {
		return errors.New(errValidatorSlashed)
	}
	return nil
}

func (e *BFTEngine) ensurePhaseVotesLocked(round *roundState, phase ConsensusPhase) map[string]Vote {
	if round.votes[phase] == nil {
		round.votes[phase] = make(map[string]Vote)
	}
	return round.votes[phase]
}

func (e *BFTEngine) slashLocked(id string, height, view uint64, phase ConsensusPhase, first, second string) {
	if v, ok := e.validators[id]; ok {
		v.Slashed = true
	}
	e.equivocation = append(e.equivocation, EquivocationEvidence{
		ValidatorID:  id,
		Height:       height,
		View:         view,
		Phase:        phase,
		FirstDigest:  first,
		SecondDigest: second,
		DetectedAt:   time.Now().UTC(),
	})
}

func (e *BFTEngine) hasQuorumLocked(phaseVotes map[string]Vote) bool {
	activeVotes := 0
	for id := range phaseVotes {
		if v, ok := e.validators[id]; ok && !v.Slashed {
			activeVotes++
		}
	}
	if activeVotes < e.threshold {
		return false
	}
	if len(e.slices) == 0 {
		return true
	}
	for _, s := range e.slices {
		if e.sliceSatisfiedLocked(s, phaseVotes) {
			return true
		}
	}
	return false
}

func (e *BFTEngine) sliceSatisfiedLocked(s QuorumSlice, phaseVotes map[string]Vote) bool {
	count := 0
	for _, id := range s.Members {
		if _, voted := phaseVotes[id]; voted {
			if v, ok := e.validators[id]; ok && !v.Slashed {
				count++
			}
		}
	}
	return count >= s.Threshold
}

func nextConsensusPhase(current ConsensusPhase) ConsensusPhase {
	switch current {
	case PhasePrepare:
		return PhasePreCommit
	case PhasePreCommit:
		return PhaseCommit
	case PhaseCommit:
		return PhaseFinalized
	default:
		return current
	}
}

// CurrentPhase returns the current consensus phase for (Height, View).
func (e *BFTEngine) CurrentPhase(height, view uint64) (ConsensusPhase, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	r, ok := e.rounds[roundKey{height: height, view: view}]
	if !ok {
		return "", false
	}
	return r.phase, true
}

// FinalizedDigest returns the finalized proposal digest for a height, if finalized.
func (e *BFTEngine) FinalizedDigest(height uint64) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	digest, ok := e.finalized[height]
	return digest, ok
}

// IsSlashed reports whether a validator has been slashed for Byzantine behavior.
func (e *BFTEngine) IsSlashed(validatorID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v, ok := e.validators[validatorID]
	return ok && v.Slashed
}

// Equivocations returns a copy of all recorded equivocation evidence.
func (e *BFTEngine) Equivocations() []EquivocationEvidence {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]EquivocationEvidence, len(e.equivocation))
	copy(out, e.equivocation)
	return out
}

// ActiveValidatorCount returns the number of non-slashed validators.
func (e *BFTEngine) ActiveValidatorCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, v := range e.validators {
		if !v.Slashed {
			count++
		}
	}
	return count
}
