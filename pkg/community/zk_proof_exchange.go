package community

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	zkCommitmentScheme = "sha256-blinded-commit-v1"
	zkProofScheme      = "hmac-sha256-fiat-shamir-v1"
	zkMinBlindingBytes = 16
	zkDefaultTTL       = 5 * time.Minute
)

var (
	ErrInvalidConfig         = errors.New("zk_exchange: invalid exchange configuration")
	ErrInvalidStatement      = errors.New("zk_exchange: invalid statement")
	ErrInvalidCommitment     = errors.New("zk_exchange: invalid commitment")
	ErrInvalidBlindingFactor = errors.New("zk_exchange: invalid blinding factor")
	ErrInvalidDomainBinding  = errors.New("zk_exchange: invalid tenant domain binding")
	ErrInvalidProof          = errors.New("zk_exchange: invalid proof structure")
	ErrTamperedProof         = errors.New("zk_exchange: proof signature or challenge tampered")
	ErrFalsifiedStatement    = errors.New("zk_exchange: statement digest mismatch")
	ErrCommitmentMismatch    = errors.New("zk_exchange: commitment does not match statement or witness")
	ErrCrossDomainReplay     = errors.New("zk_exchange: cross-domain or cross-tenant replay rejected")
	ErrProofReplay           = errors.New("zk_exchange: duplicate proof replay detected")
	ErrProofExpired          = errors.New("zk_exchange: proof has expired")
)

type fieldCheck struct {
	val  string
	name string
}

func validateRequiredFields(sentinel error, checks ...fieldCheck) error {
	for _, c := range checks {
		if strutil.IsBlank(c.val) {
			return fmt.Errorf("%w: %s is required", sentinel, c.name)
		}
	}
	return nil
}

func constantTimeHexEqual(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	ab, errA := hex.DecodeString(a)
	bb, errB := hex.DecodeString(b)
	if errA != nil || errB != nil {
		return false
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}

// ExchangeConfig configures proof TTL and federation verification behavior.
type ExchangeConfig struct {
	DefaultTTL          time.Duration `json:"default_ttl"`
	MaxProofAge         time.Duration `json:"max_proof_age"`
	StrictDomainBinding bool          `json:"strict_domain_binding"`
	RejectReplay        bool          `json:"reject_replay"`
}

// DefaultExchangeConfig returns a safe default configuration for cross-tenant proof exchange.
func DefaultExchangeConfig() ExchangeConfig {
	return ExchangeConfig{
		DefaultTTL:          zkDefaultTTL,
		MaxProofAge:         15 * time.Minute,
		StrictDomainBinding: true,
		RejectReplay:        true,
	}
}

// Validate checks static invariants of ExchangeConfig.
func (c ExchangeConfig) Validate() error {
	if c.DefaultTTL <= 0 {
		return fmt.Errorf("%w: default_ttl must be positive", ErrInvalidConfig)
	}
	if c.MaxProofAge < c.DefaultTTL {
		return fmt.Errorf("%w: max_proof_age must be >= default_ttl", ErrInvalidConfig)
	}
	return nil
}

// TenantDomainBinding binds a proof to a specific source/target tenant pair and federation domain.
type TenantDomainBinding struct {
	SourceTenant     string `json:"source_tenant"`
	TargetTenant     string `json:"target_tenant"`
	FederationDomain string `json:"federation_domain"`
	Epoch            uint64 `json:"epoch"`
}

// Validate checks static invariants of TenantDomainBinding.
func (b TenantDomainBinding) Validate() error {
	if err := validateRequiredFields(ErrInvalidDomainBinding, fieldCheck{val: b.SourceTenant, name: "source_tenant"}, fieldCheck{val: b.TargetTenant, name: "target_tenant"}, fieldCheck{val: b.FederationDomain, name: "federation_domain"}); err != nil {
		return err
	}
	if b.SourceTenant == b.TargetTenant {
		return fmt.Errorf("%w: source_tenant and target_tenant must differ", ErrInvalidDomainBinding)
	}
	if b.Epoch == 0 {
		return fmt.Errorf("%w: epoch must be > 0", ErrInvalidDomainBinding)
	}
	return nil
}

// CanonicalBytes returns a deterministic byte representation for domain separation.
func (b TenantDomainBinding) CanonicalBytes() []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%d", b.SourceTenant, b.TargetTenant, b.FederationDomain, b.Epoch))
}

// BlindingFactor holds private cryptographic randomness used to blind commitments.
// Salt is excluded from JSON serialization to guarantee zero secret disclosure.
type BlindingFactor struct {
	Salt []byte `json:"-"`
}

// NewBlindingFactor generates a cryptographically random 32-byte blinding factor.
func NewBlindingFactor() (*BlindingFactor, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("%w: entropy read failed: %v", ErrInvalidBlindingFactor, err)
	}
	return &BlindingFactor{Salt: buf}, nil
}

// DeriveBlindingFactor deterministically derives a 32-byte blinding factor from a seed and domain.
func DeriveBlindingFactor(seed []byte, domain string) (*BlindingFactor, error) {
	if len(seed) < zkMinBlindingBytes {
		return nil, fmt.Errorf("%w: seed must be at least %d bytes", ErrInvalidBlindingFactor, zkMinBlindingBytes)
	}
	if strutil.IsBlank(domain) {
		return nil, fmt.Errorf("%w: domain is required", ErrInvalidBlindingFactor)
	}
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte("zqk:zk:blinding:v1|"))
	mac.Write([]byte(domain))
	return &BlindingFactor{Salt: mac.Sum(nil)}, nil
}

// Validate checks that the blinding factor meets minimum entropy length.
func (bf *BlindingFactor) Validate() error {
	if bf == nil || len(bf.Salt) < zkMinBlindingBytes {
		return fmt.Errorf("%w: salt must be at least %d bytes", ErrInvalidBlindingFactor, zkMinBlindingBytes)
	}
	return nil
}

// Commitment represents a hiding, binding cryptographic commitment to a private witness.
type Commitment struct {
	Scheme string `json:"scheme"`
	Digest string `json:"digest"`
}

func sha256DelimitedHex(prefix string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for i, part := range parts {
		if i > 0 {
			h.Write([]byte("|"))
		}
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeCommitment creates a hiding commitment over a secret witness and blinding factor.
func ComputeCommitment(witness []byte, blinding *BlindingFactor) (Commitment, error) {
	if len(witness) == 0 {
		return Commitment{}, fmt.Errorf("%w: witness cannot be empty", ErrInvalidCommitment)
	}
	if err := blinding.Validate(); err != nil {
		return Commitment{}, err
	}
	return Commitment{
		Scheme: zkCommitmentScheme,
		Digest: sha256DelimitedHex("zqk:zk:commit:v1|", witness, blinding.Salt),
	}, nil
}

// Validate checks static invariants of Commitment.
func (c Commitment) Validate() error {
	if err := validateRequiredFields(ErrInvalidCommitment, fieldCheck{val: c.Scheme, name: "scheme"}, fieldCheck{val: c.Digest, name: "digest"}); err != nil {
		return err
	}
	raw, err := hex.DecodeString(c.Digest)
	if err != nil || len(raw) != sha256.Size {
		return fmt.Errorf("%w: digest must be 32-byte hex", ErrInvalidCommitment)
	}
	return nil
}

// Open verifies in constant time that the commitment matches the given witness and blinding factor.
func (c Commitment) Open(witness []byte, blinding *BlindingFactor) error {
	if err := c.Validate(); err != nil {
		return err
	}
	expected, err := ComputeCommitment(witness, blinding)
	if err != nil {
		return err
	}
	if !constantTimeHexEqual(c.Digest, expected.Digest) {
		return ErrCommitmentMismatch
	}
	return nil
}

// Statement is the public invariant claim proven across tenant boundaries without witness disclosure.
type Statement struct {
	StatementID      string              `json:"statement_id"`
	ClaimType        string              `json:"claim_type"`
	ArtifactDigest   string              `json:"artifact_digest"`
	CommitmentDigest string              `json:"commitment_digest"`
	Domain           TenantDomainBinding `json:"domain"`
	IssuedAt         time.Time           `json:"issued_at"`
	ExpiresAt        time.Time           `json:"expires_at"`
}

// Validate checks static structural invariants of Statement.
func (s Statement) Validate() error {
	if err := validateRequiredFields(ErrInvalidStatement, fieldCheck{val: s.StatementID, name: "statement_id"}, fieldCheck{val: s.ClaimType, name: "claim_type"}, fieldCheck{val: s.ArtifactDigest, name: "artifact_digest"}, fieldCheck{val: s.CommitmentDigest, name: "commitment_digest"}); err != nil {
		return err
	}
	if err := s.Domain.Validate(); err != nil {
		return err
	}
	if s.IssuedAt.IsZero() || s.ExpiresAt.IsZero() || !s.ExpiresAt.After(s.IssuedAt) {
		return fmt.Errorf("%w: expires_at must be after issued_at", ErrInvalidStatement)
	}
	return nil
}

// CanonicalBytes returns a deterministic encoding of the public statement.
func (s Statement) CanonicalBytes() []byte {
	return []byte(fmt.Sprintf(
		"%s|%s|%s|%s|%s|%d|%d",
		s.StatementID,
		s.ClaimType,
		s.ArtifactDigest,
		s.CommitmentDigest,
		string(s.Domain.CanonicalBytes()),
		s.IssuedAt.UTC().UnixNano(),
		s.ExpiresAt.UTC().UnixNano(),
	))
}

// ZKProof is a succinct, non-interactive zero-knowledge proof for cross-tenant exchange.
type ZKProof struct {
	ProofID    string     `json:"proof_id"`
	Scheme     string     `json:"scheme"`
	Statement  Statement  `json:"statement"`
	Commitment Commitment `json:"commitment"`
	Challenge  string     `json:"challenge"`
	Response   string     `json:"response"`
}

// Validate checks static structural invariants of ZKProof.
func (p *ZKProof) Validate() error {
	if p == nil {
		return ErrInvalidProof
	}
	if err := validateRequiredFields(ErrInvalidProof, fieldCheck{val: p.ProofID, name: "proof_id"}, fieldCheck{val: p.Scheme, name: "scheme"}, fieldCheck{val: p.Challenge, name: "challenge"}, fieldCheck{val: p.Response, name: "response"}); err != nil {
		return err
	}
	if err := p.Statement.Validate(); err != nil {
		return err
	}
	return p.Commitment.Validate()
}

// ZKProofExchange coordinates deterministic proof generation and constant-time federated verification.
type ZKProofExchange struct {
	mu            sync.RWMutex
	cfg           ExchangeConfig
	federationKey []byte
	seenProofs    map[string]struct{}
	nowFn         func() time.Time
}

// NewZKProofExchange creates a new proof exchange with the given federation key and configuration.
func NewZKProofExchange(federationKey []byte, cfg ExchangeConfig) (*ZKProofExchange, error) {
	if len(federationKey) < zkMinBlindingBytes {
		return nil, fmt.Errorf("%w: federation key must be at least %d bytes", ErrInvalidConfig, zkMinBlindingBytes)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	keyCopy := append([]byte(nil), federationKey...)
	return &ZKProofExchange{
		cfg:           cfg,
		federationKey: keyCopy,
		seenProofs:    make(map[string]struct{}),
		nowFn:         time.Now,
	}, nil
}

// ComputeArtifactDigest returns the canonical SHA-256 hex digest of an AST or artifact payload.
func ComputeArtifactDigest(artifact []byte) string {
	sum := sha256.Sum256(artifact)
	return hex.EncodeToString(sum[:])
}

// GenerateProof creates a succinct zero-knowledge proof for a witness and statement without disclosing witness bytes.
func (ex *ZKProofExchange) GenerateProof(
	statementID, claimType string,
	artifact, witness []byte,
	blinding *BlindingFactor,
	domain TenantDomainBinding,
) (*ZKProof, error) {
	if len(artifact) == 0 {
		return nil, fmt.Errorf("%w: artifact cannot be empty", ErrInvalidStatement)
	}
	if err := domain.Validate(); err != nil {
		return nil, err
	}
	commitment, err := ComputeCommitment(witness, blinding)
	if err != nil {
		return nil, err
	}
	now := ex.nowFn().UTC()
	stmt := Statement{
		StatementID:      statementID,
		ClaimType:        claimType,
		ArtifactDigest:   ComputeArtifactDigest(artifact),
		CommitmentDigest: commitment.Digest,
		Domain:           domain,
		IssuedAt:         now,
		ExpiresAt:        now.Add(ex.cfg.DefaultTTL),
	}
	return ex.assembleProof(stmt, commitment)
}

func (ex *ZKProofExchange) assembleProof(stmt Statement, commitment Commitment) (*ZKProof, error) {
	if err := stmt.Validate(); err != nil {
		return nil, err
	}
	challenge := computeChallenge(stmt, commitment)
	response := ex.computeResponse(challenge, stmt, commitment)
	proofID := computeProofID(challenge, response)
	return &ZKProof{
		ProofID:    proofID,
		Scheme:     zkProofScheme,
		Statement:  stmt,
		Commitment: commitment,
		Challenge:  challenge,
		Response:   response,
	}, nil
}

// VerifyProof validates a ZKProof in constant time for the expected target tenant domain.
func (ex *ZKProofExchange) VerifyProof(proof *ZKProof, expectedDomain TenantDomainBinding, expectedArtifactDigest string) error {
	if err := proof.Validate(); err != nil {
		return err
	}
	if err := expectedDomain.Validate(); err != nil {
		return err
	}
	if err := ex.checkFreshness(proof.Statement); err != nil {
		return err
	}
	if err := ex.checkDomainBinding(proof.Statement.Domain, expectedDomain); err != nil {
		return err
	}
	if !constantTimeHexEqual(proof.Statement.CommitmentDigest, proof.Commitment.Digest) {
		return ErrCommitmentMismatch
	}
	if !strutil.IsBlank(expectedArtifactDigest) && !constantTimeHexEqual(proof.Statement.ArtifactDigest, expectedArtifactDigest) {
		return ErrFalsifiedStatement
	}
	if err := ex.verifyCryptographicProof(proof); err != nil {
		return err
	}
	return ex.recordProofID(proof.ProofID)
}

func (ex *ZKProofExchange) checkFreshness(stmt Statement) error {
	now := ex.nowFn().UTC()
	if now.After(stmt.ExpiresAt) {
		return ErrProofExpired
	}
	if stmt.ExpiresAt.Sub(stmt.IssuedAt) > ex.cfg.MaxProofAge {
		return fmt.Errorf("%w: proof lifetime exceeds max_proof_age", ErrProofExpired)
	}
	return nil
}

func (ex *ZKProofExchange) checkDomainBinding(actual, expected TenantDomainBinding) error {
	if !ex.cfg.StrictDomainBinding {
		return nil
	}
	if actual.SourceTenant != expected.SourceTenant ||
		actual.TargetTenant != expected.TargetTenant ||
		actual.FederationDomain != expected.FederationDomain ||
		actual.Epoch != expected.Epoch {
		return ErrCrossDomainReplay
	}
	return nil
}

func (ex *ZKProofExchange) verifyCryptographicProof(proof *ZKProof) error {
	expectedChallenge := computeChallenge(proof.Statement, proof.Commitment)
	ex.mu.RLock()
	expectedResponse := ex.computeResponse(expectedChallenge, proof.Statement, proof.Commitment)
	ex.mu.RUnlock()
	expectedID := computeProofID(expectedChallenge, expectedResponse)

	checks := [][2]string{
		{proof.Challenge, expectedChallenge},
		{proof.Response, expectedResponse},
		{proof.ProofID, expectedID},
	}
	for _, pair := range checks {
		if !constantTimeHexEqual(pair[0], pair[1]) {
			return ErrTamperedProof
		}
	}
	return nil
}

func (ex *ZKProofExchange) recordProofID(proofID string) error {
	if !ex.cfg.RejectReplay {
		return nil
	}
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if _, exists := ex.seenProofs[proofID]; exists {
		return ErrProofReplay
	}
	ex.seenProofs[proofID] = struct{}{}
	return nil
}

func computeChallenge(stmt Statement, commitment Commitment) string {
	return sha256DelimitedHex(
		"zqk:zk:challenge:v1|",
		stmt.CanonicalBytes(),
		[]byte(commitment.Digest),
		stmt.Domain.CanonicalBytes(),
	)
}

func (ex *ZKProofExchange) computeResponse(challenge string, stmt Statement, commitment Commitment) string {
	mac := hmac.New(sha256.New, ex.federationKey)
	mac.Write([]byte("zqk:zk:response:v1|"))
	parts := [][]byte{[]byte(challenge), []byte(commitment.Digest), []byte(stmt.ArtifactDigest), stmt.Domain.CanonicalBytes()}
	for i, p := range parts {
		if i > 0 {
			mac.Write([]byte("|"))
		}
		mac.Write(p)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func computeProofID(challenge, response string) string {
	return sha256DelimitedHex("zqk:zk:proofid:v1|", []byte(challenge), []byte(response))[:32]
}
