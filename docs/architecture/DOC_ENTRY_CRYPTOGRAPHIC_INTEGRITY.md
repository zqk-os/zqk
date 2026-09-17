# doc_entry Cryptographic Verification & Drift Detection Specification

**Specification Version**: 1.0.0  
**Status**: Active  
**Priority Plan**: PRI-DOC-INTEGRITY-001  
**Requirement**: REQ-DOC-INTEGRITY-001  
**Last Updated**: 2026-09-10  

---

## 1. Overview

To prevent documentation sprawl and target drift, `doc_entry` objects are bound to their target physical documents via cryptographic SHA-256 hashes and measured byte sizes.

---

## 2. Specification Extensions

### 2.1 Fields Added
- `content_hash`: SHA-256 hex string (`^[a-f0-9]{64}$`). Computed over the canonical target file.
- `content_size`: Integer byte count (`minimum: 0`). Measures exact target file length.

### 2.2 Lifecycle Enforcement (`doc_entry_lifecycle.yaml`)
- **Review $\rightarrow$ Published**:
  - Target file must be reachable and readable on disk.
  - Cryptographic `content_hash` must be computed and sealed.
  - Document `content_size` must be recorded.
- **Published $\rightarrow$ Active**:
  - Cryptographic `content_hash` must match target file on disk (zero drift).

---

## 3. Automated Verification & Test Suite

The cryptographic sealing and drift detection behaviors are verified by:
- `pkg/storage/doc_entry_integrity_test.go`:
  - `CRIT-DOC-INTEGRITY-001_SpecDefinition`: Validates field presence and schema constraints.
  - `CRIT-DOC-INTEGRITY-002_LifecyclePreconditions`: Validates lifecycle transition barriers.
  - `CRIT-DOC-INTEGRITY-003_DriftDetection`: Validates system check drift and missing-target errors.
  - `CRIT-DOC-INTEGRITY-004_AutomatedSealing`: Validates SHA-256 calculation and sealing.

---

## 4. Work Tracking Matrix

| Backlog Item | Bound Criteria | Test Suite Anchor | Verification Status |
|:---|:---|:---|:---:|
| `BLI-DOC-INTEGRITY-001` | `CRIT-DOC-INTEGRITY-001` | `TestDocEntryIntegrity_Suite/CRIT-DOC-INTEGRITY-001_SpecDefinition` | **Complete** |
| `BLI-DOC-INTEGRITY-002` | `CRIT-DOC-INTEGRITY-002` | `TestDocEntryIntegrity_Suite/CRIT-DOC-INTEGRITY-002_LifecyclePreconditions` | **Complete** |
| `BLI-DOC-INTEGRITY-003` | `CRIT-DOC-INTEGRITY-003` | `TestDocEntryIntegrity_Suite/CRIT-DOC-INTEGRITY-003_DriftDetection` | **Complete** |
| `BLI-DOC-INTEGRITY-004` | `CRIT-DOC-INTEGRITY-004` | `TestDocEntryIntegrity_Suite/CRIT-DOC-INTEGRITY-004_AutomatedSealing` | **Complete** |
