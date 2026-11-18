// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	// MinModulusBitsFloor is the minimum permitted RSA modulus bit length.
	MinModulusBitsFloor = 512

	// DefaultPrimeBits is the default bit width for deterministic prime representatives.
	DefaultPrimeBits = 128

	// MinPrimeBitsFloor is the minimum permitted prime representative bit width.
	MinPrimeBitsFloor = 64

	// MaxPrimeBitsCeiling is the maximum permitted prime representative bit width.
	MaxPrimeBitsCeiling = 256

	primeSearchMillerRabinRounds = 20
	hashToPrimeDomainTag         = "zqk:rsa-accumulator:prime:v1:"
	stateDigestDomainTag         = "zqk:rsa-accumulator:state:v1:"
	uint64ByteWidth              = 8
	bitsPerByte                  = 8
)

var (
	// ErrInvalidAccumulatorConfig is returned when RSA accumulator parameters violate cryptographic floor invariants.
	ErrInvalidAccumulatorConfig = errors.New("zk_attestation: invalid RSA accumulator configuration")

	// ErrInvalidElement is returned when an element identifier is blank or cannot be mapped to a prime representative.
	ErrInvalidElement = errors.New("zk_attestation: invalid accumulator element")

	// ErrInvalidWitness is returned when a membership witness or non-membership proof fails verification.
	ErrInvalidWitness = errors.New("zk_attestation: invalid membership witness")

	// Note: ErrReplayDetected is shared from pkg/community/attestation_notary.go.
)

// AccumulatorConfig configures the cryptographic parameters of an RSAAccumulator.
type AccumulatorConfig struct {
	Modulus        *big.Int `json:"modulus"`
	Generator      *big.Int `json:"generator"`
	MinModulusBits int      `json:"min_modulus_bits,omitempty"`
	PrimeBits      int      `json:"prime_bits,omitempty"`
}

// MembershipWitness represents a succinct cryptographic proof that an element
// is accumulated in the current RSA accumulator state.
type MembershipWitness struct {
	Element       string    `json:"element"`
	Prime         *big.Int  `json:"prime"`
	Witness       *big.Int  `json:"witness"`
	AccumulatorAt *big.Int  `json:"accumulator_at"`
	Epoch         uint64    `json:"epoch"`
	IssuedAt      time.Time `json:"issued_at"`
}

// NonMembershipProof represents a Bezout-identity proof that an element is not
// currently accumulated in the RSA accumulator.
type NonMembershipProof struct {
	Element       string    `json:"element"`
	Prime         *big.Int  `json:"prime"`
	Cofactor      *big.Int  `json:"cofactor"`
	Residue       *big.Int  `json:"residue"`
	AccumulatorAt *big.Int  `json:"accumulator_at"`
	Epoch         uint64    `json:"epoch"`
	IssuedAt      time.Time `json:"issued_at"`
}

// StateAttestation binds an accumulator state digest with an anti-replay nonce
// and succinct membership witness for cross-node verification.
type StateAttestation struct {
	NodeID      string             `json:"node_id"`
	StateDigest string             `json:"state_digest"`
	Nonce       string             `json:"nonce"`
	Epoch       uint64             `json:"epoch"`
	Witness     *MembershipWitness `json:"witness,omitempty"`
	Timestamp   time.Time          `json:"timestamp"`
}

// RSAAccumulator maintains a thread-safe cryptographic RSA accumulator and
// deterministic prime representative mapping for succinct state attestation.
type RSAAccumulator struct {
	mu         sync.RWMutex
	modulus    *big.Int
	generator  *big.Int
	current    *big.Int
	primeBits  int
	epoch      uint64
	elements   map[string]*big.Int
	seenNonces map[string]struct{}
}

// NewRSAAccumulator constructs an RSAAccumulator after enforcing cryptographic
// parameter floor invariants on the modulus, generator, and prime bit width.
func NewRSAAccumulator(cfg AccumulatorConfig) (*RSAAccumulator, error) {
	minBits, primeBits, err := validateConfigParameters(cfg)
	if err != nil {
		return nil, err
	}
	if err := validateModulusAndGenerator(cfg.Modulus, cfg.Generator, minBits); err != nil {
		return nil, err
	}

	return &RSAAccumulator{
		modulus:    new(big.Int).Set(cfg.Modulus),
		generator:  new(big.Int).Set(cfg.Generator),
		current:    new(big.Int).Set(cfg.Generator),
		primeBits:  primeBits,
		epoch:      0,
		elements:   make(map[string]*big.Int),
		seenNonces: make(map[string]struct{}),
	}, nil
}

func validateConfigParameters(cfg AccumulatorConfig) (int, int, error) {
	minBits := MinModulusBitsFloor
	if cfg.MinModulusBits != 0 {
		if cfg.MinModulusBits < MinModulusBitsFloor {
			return 0, 0, fmt.Errorf("%w: min_modulus_bits %d below floor %d", ErrInvalidAccumulatorConfig, cfg.MinModulusBits, MinModulusBitsFloor)
		}
		minBits = cfg.MinModulusBits
	}

	primeBits := DefaultPrimeBits
	if cfg.PrimeBits != 0 {
		if cfg.PrimeBits < MinPrimeBitsFloor || cfg.PrimeBits > MaxPrimeBitsCeiling {
			return 0, 0, fmt.Errorf("%w: prime_bits %d outside [%d, %d]", ErrInvalidAccumulatorConfig, cfg.PrimeBits, MinPrimeBitsFloor, MaxPrimeBitsCeiling)
		}
		primeBits = cfg.PrimeBits
	}

	return minBits, primeBits, nil
}

func validateModulusAndGenerator(modulus, generator *big.Int, minBits int) error {
	if modulus == nil || modulus.Sign() <= 0 {
		return fmt.Errorf("%w: modulus must be a positive integer", ErrInvalidAccumulatorConfig)
	}
	if modulus.BitLen() < minBits {
		return fmt.Errorf("%w: modulus bit length %d below floor %d", ErrInvalidAccumulatorConfig, modulus.BitLen(), minBits)
	}
	if modulus.Bit(0) == 0 {
		return fmt.Errorf("%w: modulus must be odd", ErrInvalidAccumulatorConfig)
	}

	if generator == nil || generator.Sign() <= 0 {
		return fmt.Errorf("%w: generator must be positive", ErrInvalidAccumulatorConfig)
	}
	if generator.Cmp(big.NewInt(1)) <= 0 || generator.Cmp(modulus) >= 0 {
		return fmt.Errorf("%w: generator must satisfy 1 < g < N", ErrInvalidAccumulatorConfig)
	}

	gcd := new(big.Int).GCD(nil, nil, generator, modulus)
	if gcd.Cmp(big.NewInt(1)) != 0 {
		return fmt.Errorf("%w: generator must be coprime to modulus", ErrInvalidAccumulatorConfig)
	}

	return nil
}

// HashToPrime deterministically maps a non-blank element string to an odd prime
// representative using the accumulator's configured prime bit width.
func (a *RSAAccumulator) HashToPrime(element string) (*big.Int, error) {
	a.mu.RLock()
	bits := a.primeBits
	a.mu.RUnlock()

	return hashToPrimeWithBits(element, bits)
}

// HashToPrime deterministically maps a non-blank element string to a DefaultPrimeBits
// odd prime representative using domain-separated SHA-256 and probable-prime search.
func HashToPrime(element string) (*big.Int, error) {
	return hashToPrimeWithBits(element, DefaultPrimeBits)
}

func hashToPrimeWithBits(element string, primeBits int) (*big.Int, error) {
	if err := strutil.ErrorOnBlank(element, "%w: element cannot be blank", ErrInvalidElement); err != nil {
		return nil, err
	}

	byteLen := (primeBits + bitsPerByte - 1) / bitsPerByte
	var counter uint64

	for {
		candidate := derivePrimeCandidate(element, counter, byteLen, primeBits)
		if candidate.ProbablyPrime(primeSearchMillerRabinRounds) {
			return candidate, nil
		}
		counter++
	}
}

func derivePrimeCandidate(element string, counter uint64, byteLen, primeBits int) *big.Int {
	h := sha256.New()
	_, _ = h.Write([]byte(hashToPrimeDomainTag))

	var lenBuf [uint64ByteWidth]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(element)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(element))

	var ctrBuf [uint64ByteWidth]byte
	binary.BigEndian.PutUint64(ctrBuf[:], counter)
	_, _ = h.Write(ctrBuf[:])

	digest := h.Sum(nil)
	buf := make([]byte, byteLen)
	copy(buf, digest[:byteLen])

	// Ensure exact bit length and odd parity.
	topBitOffset := (primeBits - 1) % bitsPerByte
	buf[0] |= 1 << topBitOffset
	if excess := byteLen*bitsPerByte - primeBits; excess > 0 {
		buf[0] &= (1 << (bitsPerByte - excess)) - 1
		buf[0] |= 1 << (bitsPerByte - excess - 1)
	}
	buf[len(buf)-1] |= 1

	return new(big.Int).SetBytes(buf)
}

// StateDigest returns a deterministic SHA-256 hex digest of the accumulator's
// modulus, generator, current accumulated value, and epoch.
func (a *RSAAccumulator) StateDigest() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.stateDigestLocked()
}

func (a *RSAAccumulator) stateDigestLocked() string {
	h := sha256.New()
	_, _ = h.Write([]byte(stateDigestDomainTag))
	_, _ = h.Write(a.modulus.Bytes())
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(a.generator.Bytes())
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(a.current.Bytes())

	var epochBuf [uint64ByteWidth]byte
	binary.BigEndian.PutUint64(epochBuf[:], a.epoch)
	_, _ = h.Write(epochBuf[:])

	return hex.EncodeToString(h.Sum(nil))
}

// Modulus returns a defensive copy of the accumulator's RSA modulus.
func (a *RSAAccumulator) Modulus() *big.Int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return new(big.Int).Set(a.modulus)
}

// Generator returns a defensive copy of the accumulator's generator.
func (a *RSAAccumulator) Generator() *big.Int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return new(big.Int).Set(a.generator)
}

// Current returns a defensive copy of the current accumulated value.
func (a *RSAAccumulator) Current() *big.Int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return new(big.Int).Set(a.current)
}

// Epoch returns the current monotonic accumulator epoch.
func (a *RSAAccumulator) Epoch() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.epoch
}

// Add maps a non-blank element to its deterministic prime representative and
// folds it into the RSA accumulator state. Repeated additions of the same
// element are idempotent.
func (a *RSAAccumulator) Add(element string) error {
	prime, err := a.HashToPrime(element)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.elements[element]; exists {
		return nil
	}

	a.elements[element] = prime
	a.current = new(big.Int).Exp(a.current, prime, a.modulus)
	a.epoch++
	return nil
}

// GenerateMembershipWitness constructs a succinct RSA membership witness
// W = g^(prod_{q != p} q) mod N for an element currently in the accumulator.
func (a *RSAAccumulator) GenerateMembershipWitness(element string) (*MembershipWitness, error) {
	if err := strutil.ErrorOnBlank(element, "%w: element cannot be blank", ErrInvalidElement); err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.generateMembershipWitnessLocked(element)
}

func (a *RSAAccumulator) generateMembershipWitnessLocked(element string) (*MembershipWitness, error) {
	prime, exists := a.elements[element]
	if !exists {
		return nil, fmt.Errorf("%w: element %q is not accumulated", ErrInvalidElement, element)
	}

	witness := new(big.Int).Set(a.generator)
	for otherElement, q := range a.elements {
		if otherElement == element {
			continue
		}
		witness.Exp(witness, q, a.modulus)
	}

	return &MembershipWitness{
		Element:       element,
		Prime:         new(big.Int).Set(prime),
		Witness:       witness,
		AccumulatorAt: new(big.Int).Set(a.current),
		Epoch:         a.epoch,
		IssuedAt:      time.Now().UTC(),
	}, nil
}

// VerifyMembershipWitness verifies that W^p mod N matches the accumulator's
// current state and that the witness prime matches the element's deterministic representative.
func (a *RSAAccumulator) VerifyMembershipWitness(w *MembershipWitness) (bool, error) {
	a.mu.RLock()
	modulus := new(big.Int).Set(a.modulus)
	current := new(big.Int).Set(a.current)
	primeBits := a.primeBits
	a.mu.RUnlock()

	if err := verifyMembershipWitnessWithState(w, modulus, current, primeBits); err != nil {
		return false, err
	}
	return true, nil
}

func verifyMembershipWitnessWithState(w *MembershipWitness, modulus, current *big.Int, primeBits int) error {
	if w == nil {
		return fmt.Errorf("%w: witness cannot be nil", ErrInvalidWitness)
	}
	if err := strutil.ErrorOnBlank(w.Element, "%w: witness element cannot be blank", ErrInvalidWitness); err != nil {
		return err
	}
	if w.Prime == nil || w.Prime.Sign() <= 0 {
		return fmt.Errorf("%w: witness prime must be positive", ErrInvalidWitness)
	}
	if w.Witness == nil || w.Witness.Sign() <= 0 {
		return fmt.Errorf("%w: witness value must be positive", ErrInvalidWitness)
	}
	if w.AccumulatorAt == nil || w.AccumulatorAt.Sign() <= 0 {
		return fmt.Errorf("%w: witness accumulator value must be positive", ErrInvalidWitness)
	}

	expectedPrime, err := hashToPrimeWithBits(w.Element, primeBits)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWitness, err)
	}
	if expectedPrime.Cmp(w.Prime) != 0 {
		return fmt.Errorf("%w: witness prime does not match element representative", ErrInvalidWitness)
	}

	if w.AccumulatorAt.Cmp(current) != 0 {
		return fmt.Errorf("%w: witness accumulator state does not match current accumulator", ErrInvalidWitness)
	}

	lhs := new(big.Int).Exp(w.Witness, w.Prime, modulus)
	if lhs.Cmp(current) != 0 {
		return fmt.Errorf("%w: modular exponentiation check failed", ErrInvalidWitness)
	}

	return nil
}

// IssueAttestation binds a membership witness with an anti-replay nonce, epoch,
// and state digest for succinct cluster attestation.
func (a *RSAAccumulator) IssueAttestation(element string, nonce string) (*StateAttestation, error) {
	if err := strutil.ErrorOnBlank(element, "%w: element cannot be blank", ErrInvalidElement); err != nil {
		return nil, err
	}
	if err := strutil.ErrorOnBlank(nonce, "%w: attestation nonce cannot be blank", ErrInvalidWitness); err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	witness, err := a.generateMembershipWitnessLocked(element)
	if err != nil {
		return nil, err
	}

	return &StateAttestation{
		NodeID:      element,
		StateDigest: a.stateDigestLocked(),
		Nonce:       nonce,
		Epoch:       witness.Epoch,
		Witness:     witness,
		Timestamp:   time.Now().UTC(),
	}, nil
}

// VerifyAttestation validates a StateAttestation's fields, state digest, epoch,
// membership witness, and anti-replay nonce. Reused nonces are rejected with ErrReplayDetected.
func (a *RSAAccumulator) VerifyAttestation(att *StateAttestation) (bool, error) {
	if att == nil {
		return false, fmt.Errorf("%w: attestation cannot be nil", ErrInvalidWitness)
	}
	if err := strutil.ErrorOnBlank(att.NodeID, "%w: attestation node_id cannot be blank", ErrInvalidWitness); err != nil {
		return false, err
	}
	if err := strutil.ErrorOnBlank(att.Nonce, "%w: attestation nonce cannot be blank", ErrInvalidWitness); err != nil {
		return false, err
	}
	if err := strutil.ErrorOnBlank(att.StateDigest, "%w: attestation state_digest cannot be blank", ErrInvalidWitness); err != nil {
		return false, err
	}
	if att.Witness == nil {
		return false, fmt.Errorf("%w: attestation witness cannot be nil", ErrInvalidWitness)
	}
	if att.Witness.Element != att.NodeID {
		return false, fmt.Errorf("%w: attestation node_id %q does not match witness element %q", ErrInvalidWitness, att.NodeID, att.Witness.Element)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, seen := a.seenNonces[att.Nonce]; seen {
		return false, fmt.Errorf("%w: attestation nonce %q already consumed", ErrReplayDetected, att.Nonce)
	}
	if att.StateDigest != a.stateDigestLocked() {
		return false, fmt.Errorf("%w: attestation state digest mismatch", ErrInvalidWitness)
	}
	if att.Epoch != a.epoch || att.Witness.Epoch != att.Epoch {
		return false, fmt.Errorf("%w: attestation epoch mismatch", ErrInvalidWitness)
	}
	if err := verifyMembershipWitnessWithState(att.Witness, a.modulus, a.current, a.primeBits); err != nil {
		return false, err
	}

	a.seenNonces[att.Nonce] = struct{}{}
	return true, nil
}

// GenerateNonMembershipProof constructs a Bezout-identity proof that an element
// is not currently accumulated in the RSA accumulator.
func (a *RSAAccumulator) GenerateNonMembershipProof(element string) (*NonMembershipProof, error) {
	prime, err := a.HashToPrime(element)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.elements[element]; exists {
		return nil, fmt.Errorf("%w: element %q is currently accumulated", ErrInvalidElement, element)
	}

	product := big.NewInt(1)
	for _, q := range a.elements {
		product.Mul(product, q)
	}

	cofactor := new(big.Int)
	bCoef := new(big.Int)
	gcd := new(big.Int).GCD(cofactor, bCoef, product, prime)
	if gcd.Cmp(big.NewInt(1)) != 0 {
		return nil, fmt.Errorf("%w: element prime is not coprime to accumulated product", ErrInvalidElement)
	}

	residue, err := modExpSigned(a.generator, bCoef, a.modulus)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to compute non-membership residue: %v", ErrInvalidWitness, err)
	}

	return &NonMembershipProof{
		Element:       element,
		Prime:         prime,
		Cofactor:      cofactor,
		Residue:       residue,
		AccumulatorAt: new(big.Int).Set(a.current),
		Epoch:         a.epoch,
		IssuedAt:      time.Now().UTC(),
	}, nil
}

// VerifyNonMembershipProof verifies a Bezout-identity non-membership proof
// against the accumulator's current state: A^a * d^p == g (mod N).
func (a *RSAAccumulator) VerifyNonMembershipProof(p *NonMembershipProof) (bool, error) {
	if p == nil {
		return false, fmt.Errorf("%w: non-membership proof cannot be nil", ErrInvalidWitness)
	}
	if err := strutil.ErrorOnBlank(p.Element, "%w: proof element cannot be blank", ErrInvalidWitness); err != nil {
		return false, err
	}
	if p.Prime == nil || p.Prime.Sign() <= 0 {
		return false, fmt.Errorf("%w: proof prime must be positive", ErrInvalidWitness)
	}
	if p.Cofactor == nil {
		return false, fmt.Errorf("%w: proof cofactor cannot be nil", ErrInvalidWitness)
	}
	if p.Residue == nil || p.Residue.Sign() <= 0 {
		return false, fmt.Errorf("%w: proof residue must be positive", ErrInvalidWitness)
	}
	if p.AccumulatorAt == nil || p.AccumulatorAt.Sign() <= 0 {
		return false, fmt.Errorf("%w: proof accumulator value must be positive", ErrInvalidWitness)
	}

	a.mu.RLock()
	modulus := new(big.Int).Set(a.modulus)
	generator := new(big.Int).Set(a.generator)
	current := new(big.Int).Set(a.current)
	primeBits := a.primeBits
	a.mu.RUnlock()

	if p.Residue.Cmp(modulus) >= 0 {
		return false, fmt.Errorf("%w: proof residue must be reduced modulo N", ErrInvalidWitness)
	}

	expectedPrime, err := hashToPrimeWithBits(p.Element, primeBits)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidWitness, err)
	}
	if expectedPrime.Cmp(p.Prime) != 0 {
		return false, fmt.Errorf("%w: proof prime does not match element representative", ErrInvalidWitness)
	}

	if p.AccumulatorAt.Cmp(current) != 0 {
		return false, fmt.Errorf("%w: proof accumulator state does not match current accumulator", ErrInvalidWitness)
	}

	term1, err := modExpSigned(current, p.Cofactor, modulus)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidWitness, err)
	}
	term2 := new(big.Int).Exp(p.Residue, p.Prime, modulus)

	lhs := new(big.Int).Mul(term1, term2)
	lhs.Mod(lhs, modulus)
	if lhs.Cmp(generator) != 0 {
		return false, fmt.Errorf("%w: Bezout non-membership verification failed", ErrInvalidWitness)
	}

	return true, nil
}

func modExpSigned(base, exp, modulus *big.Int) (*big.Int, error) {
	if exp.Sign() >= 0 {
		return new(big.Int).Exp(base, exp, modulus), nil
	}
	inv := new(big.Int).ModInverse(base, modulus)
	if inv == nil {
		return nil, errors.New("base is not invertible modulo N")
	}
	absExp := new(big.Int).Abs(exp)
	return new(big.Int).Exp(inv, absExp, modulus), nil
}
