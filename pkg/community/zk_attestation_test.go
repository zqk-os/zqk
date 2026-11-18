// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// generateTestRSAModulus constructs a deterministic 512-bit composite RSA-like
// modulus N = p * q and a valid unit generator g in (Z/NZ)* for unit tests.
func generateTestRSAModulus(t *testing.T) (*big.Int, *big.Int) {
	t.Helper()

	p, ok := new(big.Int).SetString("d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f813", 16)
	if !ok {
		t.Fatal("failed to parse test prime p")
	}
	for !p.ProbablyPrime(20) {
		p.Add(p, big.NewInt(2))
	}

	q, ok := new(big.Int).SetString("e1f2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f87", 16)
	if !ok {
		t.Fatal("failed to parse test prime q")
	}
	for !q.ProbablyPrime(20) {
		q.Add(q, big.NewInt(2))
	}

	n := new(big.Int).Mul(p, q)
	if n.BitLen() < MinModulusBitsFloor {
		t.Fatalf("expected test modulus >= %d bits, got %d", MinModulusBitsFloor, n.BitLen())
	}

	g := big.NewInt(65537)
	return n, g
}

func TestZKAttestation_StaticFloorInvariant(t *testing.T) {
	validModulus, validGenerator := generateTestRSAModulus(t)

	t.Run("valid accumulator config initializes and computes deterministic digest", func(t *testing.T) {
		acc1, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("expected valid config to succeed, got error: %v", err)
		}

		acc2, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   new(big.Int).Set(validModulus),
			Generator: new(big.Int).Set(validGenerator),
		})
		if err != nil {
			t.Fatalf("expected second valid config to succeed, got error: %v", err)
		}

		d1 := acc1.StateDigest()
		d2 := acc2.StateDigest()
		if d1 == "" || d1 != d2 {
			t.Fatalf("expected non-empty deterministic state digest, got %q vs %q", d1, d2)
		}
	})

	t.Run("rejects parameter floor violations", func(t *testing.T) {
		smallModulus := new(big.Int).Lsh(big.NewInt(1), 255)
		smallModulus.Add(smallModulus, big.NewInt(1))

		evenModulus := new(big.Int).Lsh(big.NewInt(1), 512)

		cases := []struct {
			name string
			cfg  AccumulatorConfig
		}{
			{
				name: "nil modulus",
				cfg:  AccumulatorConfig{Generator: validGenerator},
			},
			{
				name: "nil generator",
				cfg:  AccumulatorConfig{Modulus: validModulus},
			},
			{
				name: "modulus below 512-bit floor",
				cfg: AccumulatorConfig{
					Modulus:   smallModulus,
					Generator: validGenerator,
				},
			},
			{
				name: "configured min_modulus_bits below floor",
				cfg: AccumulatorConfig{
					Modulus:        validModulus,
					Generator:      validGenerator,
					MinModulusBits: 256,
				},
			},
			{
				name: "even modulus",
				cfg: AccumulatorConfig{
					Modulus:   evenModulus,
					Generator: validGenerator,
				},
			},
			{
				name: "generator <= 1",
				cfg: AccumulatorConfig{
					Modulus:   validModulus,
					Generator: big.NewInt(1),
				},
			},
			{
				name: "generator >= modulus",
				cfg: AccumulatorConfig{
					Modulus:   validModulus,
					Generator: new(big.Int).Set(validModulus),
				},
			},
			{
				name: "prime bits below floor",
				cfg: AccumulatorConfig{
					Modulus:   validModulus,
					Generator: validGenerator,
					PrimeBits: 32,
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := NewRSAAccumulator(tc.cfg)
				if !errors.Is(err, ErrInvalidAccumulatorConfig) {
					t.Fatalf("expected ErrInvalidAccumulatorConfig for %s, got: %v", tc.name, err)
				}
			})
		}
	})

	t.Run("rejects blank elements in HashToPrime", func(t *testing.T) {
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}

		for _, blank := range []string{"", "   ", "\t\n "} {
			if _, err := acc.HashToPrime(blank); !errors.Is(err, ErrInvalidElement) {
				t.Fatalf("expected ErrInvalidElement for blank input %q, got: %v", blank, err)
			}
			if _, err := HashToPrime(blank); !errors.Is(err, ErrInvalidElement) {
				t.Fatalf("expected package HashToPrime ErrInvalidElement for blank input %q, got: %v", blank, err)
			}
		}
	})

	t.Run("deterministic prime representative mapping", func(t *testing.T) {
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}

		p1, err := acc.HashToPrime("node:alpha:state-v1")
		if err != nil {
			t.Fatalf("unexpected HashToPrime error: %v", err)
		}
		p2, err := acc.HashToPrime("node:alpha:state-v1")
		if err != nil {
			t.Fatalf("unexpected second HashToPrime error: %v", err)
		}
		pPkg, err := HashToPrime("node:alpha:state-v1")
		if err != nil {
			t.Fatalf("unexpected package HashToPrime error: %v", err)
		}

		if p1.Cmp(p2) != 0 || p1.Cmp(pPkg) != 0 {
			t.Fatalf("expected deterministic prime mapping, got %s, %s, %s", p1, p2, pPkg)
		}
		if !p1.ProbablyPrime(20) {
			t.Fatalf("expected prime representative to pass primality check, got %s", p1)
		}
		if p1.BitLen() < DefaultPrimeBits {
			t.Fatalf("expected prime bit length >= %d, got %d", DefaultPrimeBits, p1.BitLen())
		}

		pOther, err := acc.HashToPrime("node:beta:state-v1")
		if err != nil {
			t.Fatalf("unexpected HashToPrime error for second element: %v", err)
		}
		if p1.Cmp(pOther) == 0 {
			t.Fatalf("expected distinct elements to map to distinct primes, both mapped to %s", p1)
		}
	})
}

func TestZKAttestation_OperationalDynamicProof(t *testing.T) {
	validModulus, validGenerator := generateTestRSAModulus(t)

	t.Run("multi-element addition, witness generation, and verification", func(t *testing.T) {
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}

		initialDigest := acc.StateDigest()
		nodes := []string{"node:alpha:v1", "node:beta:v1", "node:gamma:v1"}

		for i, node := range nodes {
			if err := acc.Add(node); err != nil {
				t.Fatalf("failed to add element %q: %v", node, err)
			}
			expectedEpoch := uint64(i + 1)
			if acc.Epoch() != expectedEpoch {
				t.Fatalf("expected epoch %d after adding %q, got %d", expectedEpoch, node, acc.Epoch())
			}
		}

		if acc.StateDigest() == initialDigest {
			t.Fatal("expected StateDigest to change after adding elements")
		}

		// Idempotent re-addition must not increment epoch or alter digest.
		digestBeforeIdempotent := acc.StateDigest()
		epochBeforeIdempotent := acc.Epoch()
		if err := acc.Add("node:beta:v1"); err != nil {
			t.Fatalf("expected idempotent Add to succeed, got: %v", err)
		}
		if acc.Epoch() != epochBeforeIdempotent || acc.StateDigest() != digestBeforeIdempotent {
			t.Fatalf("expected idempotent Add to preserve epoch (%d vs %d) and digest", acc.Epoch(), epochBeforeIdempotent)
		}

		// Generate and verify witnesses for all accumulated elements.
		for _, node := range nodes {
			witness, err := acc.GenerateMembershipWitness(node)
			if err != nil {
				t.Fatalf("failed to generate witness for %q: %v", node, err)
			}
			if witness.Epoch != acc.Epoch() {
				t.Fatalf("expected witness epoch %d, got %d", acc.Epoch(), witness.Epoch)
			}
			ok, err := acc.VerifyMembershipWitness(witness)
			if err != nil || !ok {
				t.Fatalf("expected witness for %q to verify, got ok=%v err=%v", node, ok, err)
			}
		}

		// Issue and verify succinct cluster attestation.
		att, err := acc.IssueAttestation("node:alpha:v1", "nonce-12345")
		if err != nil {
			t.Fatalf("expected IssueAttestation to succeed, got: %v", err)
		}
		if att.NodeID != "node:alpha:v1" || att.Nonce != "nonce-12345" || att.StateDigest != acc.StateDigest() {
			t.Fatalf("unexpected attestation metadata: %+v", att)
		}
		ok, err := acc.VerifyMembershipWitness(att.Witness)
		if err != nil || !ok {
			t.Fatalf("expected attestation witness to verify, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("rejects blank or unaccumulated elements and tampered witnesses", func(t *testing.T) {
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}

		if err := acc.Add("   "); !errors.Is(err, ErrInvalidElement) {
			t.Fatalf("expected ErrInvalidElement for blank Add, got: %v", err)
		}
		if err := acc.Add("node:alpha:v1"); err != nil {
			t.Fatalf("unexpected Add error: %v", err)
		}
		if _, err := acc.GenerateMembershipWitness("node:missing:v1"); err == nil {
			t.Fatal("expected error generating witness for unaccumulated element")
		}
		if _, err := acc.IssueAttestation("node:alpha:v1", "  "); !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for blank nonce, got: %v", err)
		}

		if ok, err := acc.VerifyMembershipWitness(nil); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected nil witness to fail with ErrInvalidWitness, got ok=%v err=%v", ok, err)
		}

		witness, err := acc.GenerateMembershipWitness("node:alpha:v1")
		if err != nil {
			t.Fatalf("unexpected witness error: %v", err)
		}
		tampered := *witness
		tampered.Witness = new(big.Int).Add(witness.Witness, big.NewInt(1))
		if ok, err := acc.VerifyMembershipWitness(&tampered); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected tampered witness to fail with ErrInvalidWitness, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("concurrent read and verify safety", func(t *testing.T) {
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}

		const numElements = 6
		elements := make([]string, numElements)
		for i := 0; i < numElements; i++ {
			elements[i] = fmt.Sprintf("cluster:member:%d", i)
			if err := acc.Add(elements[i]); err != nil {
				t.Fatalf("failed to add %q: %v", elements[i], err)
			}
		}

		var wg sync.WaitGroup
		errCh := make(chan error, numElements*4)

		for g := 0; g < 4; g++ {
			workerID := g
			wg.Add(1)
			goroutinelabels.NewGoroutine("zk_attestation_concurrent_worker", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
				defer wg.Done()
				for _, elem := range elements {
					w, err := acc.GenerateMembershipWitness(elem)
					if err != nil {
						errCh <- fmt.Errorf("worker %d witness gen %q: %w", workerID, elem, err)
						return
					}
					ok, err := acc.VerifyMembershipWitness(w)
					if err != nil || !ok {
						errCh <- fmt.Errorf("worker %d verify %q: ok=%v err=%v", workerID, elem, ok, err)
						return
					}
					att, err := acc.IssueAttestation(elem, fmt.Sprintf("nonce-%d-%s", workerID, elem))
					if err != nil {
						errCh <- fmt.Errorf("worker %d attest %q: %w", workerID, elem, err)
						return
					}
					if att.StateDigest == "" {
						errCh <- fmt.Errorf("worker %d empty state digest", workerID)
						return
					}
				}
			})
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Fatal(err)
		}
	})
}

func TestZKAttestation_NegativeAdversarialBoundary(t *testing.T) {
	validModulus, validGenerator := generateTestRSAModulus(t)

	newSeededAccumulator := func(t *testing.T) *RSAAccumulator {
		t.Helper()
		acc, err := NewRSAAccumulator(AccumulatorConfig{
			Modulus:   validModulus,
			Generator: validGenerator,
		})
		if err != nil {
			t.Fatalf("unexpected init error: %v", err)
		}
		for _, elem := range []string{"node:alpha:v1", "node:beta:v1", "node:gamma:v1"} {
			if err := acc.Add(elem); err != nil {
				t.Fatalf("failed to add %q: %v", elem, err)
			}
		}
		return acc
	}

	t.Run("non-member witness forgery rejection", func(t *testing.T) {
		acc := newSeededAccumulator(t)

		roguePrime, err := acc.HashToPrime("node:rogue:v1")
		if err != nil {
			t.Fatalf("unexpected HashToPrime error: %v", err)
		}

		// Forged witness using generator or another member's witness.
		validWitness, err := acc.GenerateMembershipWitness("node:alpha:v1")
		if err != nil {
			t.Fatalf("unexpected witness error: %v", err)
		}

		forgedCases := []struct {
			name    string
			witness *MembershipWitness
		}{
			{
				name: "forged witness with rogue element and generator",
				witness: &MembershipWitness{
					Element:       "node:rogue:v1",
					Prime:         roguePrime,
					Witness:       acc.Generator(),
					AccumulatorAt: acc.Current(),
					Epoch:         acc.Epoch(),
				},
			},
			{
				name: "swapped element on valid witness",
				witness: &MembershipWitness{
					Element:       "node:rogue:v1",
					Prime:         roguePrime,
					Witness:       new(big.Int).Set(validWitness.Witness),
					AccumulatorAt: new(big.Int).Set(validWitness.AccumulatorAt),
					Epoch:         validWitness.Epoch,
				},
			},
		}

		for _, tc := range forgedCases {
			t.Run(tc.name, func(t *testing.T) {
				ok, err := acc.VerifyMembershipWitness(tc.witness)
				if ok || !errors.Is(err, ErrInvalidWitness) {
					t.Fatalf("expected ErrInvalidWitness for %s, got ok=%v err=%v", tc.name, ok, err)
				}
			})
		}
	})

	t.Run("tampered witness and attestation field rejection", func(t *testing.T) {
		acc := newSeededAccumulator(t)

		att, err := acc.IssueAttestation("node:alpha:v1", "nonce-tamper-check-1")
		if err != nil {
			t.Fatalf("unexpected IssueAttestation error: %v", err)
		}

		// Nil attestation and blank nonce.
		if ok, err := acc.VerifyAttestation(nil); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for nil attestation, got ok=%v err=%v", ok, err)
		}
		blankNonceAtt := *att
		blankNonceAtt.Nonce = "   "
		if ok, err := acc.VerifyAttestation(&blankNonceAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for blank nonce, got ok=%v err=%v", ok, err)
		}

		// Tamper StateDigest.
		badDigestAtt := *att
		badDigestAtt.StateDigest = "0000000000000000000000000000000000000000000000000000000000000000"
		if ok, err := acc.VerifyAttestation(&badDigestAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered StateDigest, got ok=%v err=%v", ok, err)
		}

		// Tamper Epoch.
		badEpochAtt := *att
		badEpochAtt.Epoch = att.Epoch + 1
		if ok, err := acc.VerifyAttestation(&badEpochAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Epoch, got ok=%v err=%v", ok, err)
		}

		// Tamper Witness.Witness.
		badWitnessVal := *att.Witness
		badWitnessVal.Witness = new(big.Int).Add(att.Witness.Witness, big.NewInt(1))
		badWitnessAtt := *att
		badWitnessAtt.Witness = &badWitnessVal
		if ok, err := acc.VerifyAttestation(&badWitnessAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Witness.Witness, got ok=%v err=%v", ok, err)
		}

		// Tamper Witness.Prime.
		badPrimeVal := *att.Witness
		badPrimeVal.Prime = new(big.Int).Add(att.Witness.Prime, big.NewInt(2))
		badPrimeAtt := *att
		badPrimeAtt.Witness = &badPrimeVal
		if ok, err := acc.VerifyAttestation(&badPrimeAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Witness.Prime, got ok=%v err=%v", ok, err)
		}

		// Tamper Witness.AccumulatorAt.
		badAccAtVal := *att.Witness
		badAccAtVal.AccumulatorAt = new(big.Int).Add(att.Witness.AccumulatorAt, big.NewInt(1))
		badAccAtAtt := *att
		badAccAtAtt.Witness = &badAccAtVal
		if ok, err := acc.VerifyAttestation(&badAccAtAtt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Witness.AccumulatorAt, got ok=%v err=%v", ok, err)
		}

		// Valid original attestation still passes (failed tampered attempts must not consume the nonce).
		ok, err := acc.VerifyAttestation(att)
		if err != nil || !ok {
			t.Fatalf("expected valid attestation to pass, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("replay attack rejection on reused nonce", func(t *testing.T) {
		acc := newSeededAccumulator(t)

		att, err := acc.IssueAttestation("node:beta:v1", "unique-nonce-replay-1")
		if err != nil {
			t.Fatalf("unexpected IssueAttestation error: %v", err)
		}

		ok, err := acc.VerifyAttestation(att)
		if err != nil || !ok {
			t.Fatalf("expected first VerifyAttestation to succeed, got ok=%v err=%v", ok, err)
		}

		// Replaying the same attestation must fail with ErrReplayDetected.
		if ok, err := acc.VerifyAttestation(att); ok || !errors.Is(err, ErrReplayDetected) {
			t.Fatalf("expected ErrReplayDetected on replay, got ok=%v err=%v", ok, err)
		}

		// Reusing the same nonce on a freshly issued attestation must also fail with ErrReplayDetected.
		att2, err := acc.IssueAttestation("node:gamma:v1", "unique-nonce-replay-1")
		if err != nil {
			t.Fatalf("unexpected second IssueAttestation error: %v", err)
		}
		if ok, err := acc.VerifyAttestation(att2); ok || !errors.Is(err, ErrReplayDetected) {
			t.Fatalf("expected ErrReplayDetected on reused nonce across elements, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("non-membership proof generation, verification, and tamper rejection", func(t *testing.T) {
		acc := newSeededAccumulator(t)

		// Generating a non-membership proof for an actual member must fail with ErrInvalidElement.
		if _, err := acc.GenerateNonMembershipProof("node:alpha:v1"); !errors.Is(err, ErrInvalidElement) {
			t.Fatalf("expected ErrInvalidElement for accumulated member, got: %v", err)
		}
		if _, err := acc.GenerateNonMembershipProof("   "); !errors.Is(err, ErrInvalidElement) {
			t.Fatalf("expected ErrInvalidElement for blank element, got: %v", err)
		}

		// Generate and verify non-membership proof for a genuine non-member.
		proof, err := acc.GenerateNonMembershipProof("node:outsider:v1")
		if err != nil {
			t.Fatalf("expected GenerateNonMembershipProof to succeed, got: %v", err)
		}
		ok, err := acc.VerifyNonMembershipProof(proof)
		if err != nil || !ok {
			t.Fatalf("expected VerifyNonMembershipProof to succeed, got ok=%v err=%v", ok, err)
		}

		// Reject nil and tampered non-membership proofs.
		if ok, err := acc.VerifyNonMembershipProof(nil); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for nil proof, got ok=%v err=%v", ok, err)
		}

		badCofactor := *proof
		badCofactor.Cofactor = new(big.Int).Add(proof.Cofactor, big.NewInt(1))
		if ok, err := acc.VerifyNonMembershipProof(&badCofactor); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Cofactor, got ok=%v err=%v", ok, err)
		}

		badResidue := *proof
		badResidue.Residue = new(big.Int).Add(proof.Residue, big.NewInt(1))
		if ok, err := acc.VerifyNonMembershipProof(&badResidue); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Residue, got ok=%v err=%v", ok, err)
		}

		badPrime := *proof
		badPrime.Prime = new(big.Int).Add(proof.Prime, big.NewInt(2))
		if ok, err := acc.VerifyNonMembershipProof(&badPrime); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered Prime, got ok=%v err=%v", ok, err)
		}

		badAccAt := *proof
		badAccAt.AccumulatorAt = new(big.Int).Add(proof.AccumulatorAt, big.NewInt(1))
		if ok, err := acc.VerifyNonMembershipProof(&badAccAt); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected ErrInvalidWitness for tampered AccumulatorAt, got ok=%v err=%v", ok, err)
		}

		// Once the outsider is added, the old non-membership proof must fail.
		if err := acc.Add("node:outsider:v1"); err != nil {
			t.Fatalf("unexpected Add error: %v", err)
		}
		if ok, err := acc.VerifyNonMembershipProof(proof); ok || !errors.Is(err, ErrInvalidWitness) {
			t.Fatalf("expected stale non-membership proof to fail after Add, got ok=%v err=%v", ok, err)
		}
	})
}
