# L-SUPPLY-RELEASE Narrative: Supply Chain & Release Hygiene

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-SUPPLY-RELEASE`  
**Density Class:** `D-HIGH` (Exhaustive Checklist)  
**Primary Axes:** `SEC` (Security), `OPS` (Operability), `RCV` (Recoverability)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files)

---

## 1. Executive Assessment

Evaluation of the ZQK open-core candidate against the 7 discrete criteria of the `L-SUPPLY-RELEASE` rubric reveals a dual reality: the project exhibits exceptionally disciplined local file hygiene (`.gitignore` spans 361 lines covering local keys, tokens, and ephemeral runtimes), sound lockfile pinning (`go.mod` and `go.sum` verified clean via `go mod verify`), and multi-stage CI workflows. However, the release machinery, build flags, and downstream installation paths suffer from material configuration defects and missing automation gates:

1. **Broken Release Binary Version Provenance (`F-SUPPLY-RELEASE-001`, High):** In `.goreleaser.yaml` lines 20–23, build ldflags target package `main` (`-X main.version={{.Version}}`, `-X main.commit={{.Commit}}`, `-X main.date={{.Date}}`). However, version variables in ZQK reside in package `app` (`cmd/zqk/app/root.go` lines 73–77). Because Go requires fully qualified package paths for symbol substitution (as correctly implemented in `Makefile` lines 30–32), official GitHub Release binaries fail to inject release tags and commit SHAs, permanently displaying `version: dev` and `unknown` commit/build dates.
2. **Defective Source Installation Path (`F-SUPPLY-RELEASE-002`, Moderate):** The public `install.sh` script provides a build-from-source fallback (`install_source()`, line 256) that executes `make -C "$src_dir" zqk`. However, `Makefile` defines no `zqk` target (the build targets are `all`, `build`, and `compile-bin`). Running `make -n zqk` yields `make: Nothing to be done for 'zqk'.`, causing fresh source installs to fail with `Built binary not found in .../bin/zqk`.
3. **Unwired Secret Scanner & Missing CI Gate (`F-SUPPLY-RELEASE-003`, Moderate):** While `scripts/scan-secrets.sh` is present and effectively detects API tokens and private keys, it is not wired into `tools/git-hooks/pre-commit` nor included in `.github/workflows/ci.yml`. Furthermore, `pkg/processhygiene/secret_scan_workflow_test.go` requires `.github/workflows/secret-scan.yml`, which is absent from the repository.
4. **Missing Dependabot Configuration & Broken Test (`F-SUPPLY-RELEASE-004`, Moderate):** `pkg/processhygiene/sbom_workflow_test.go` verifies that `.github/dependabot.yml` monitors `gomod` and `github-actions`, and that `SECURITY.md` documents SBOM and Dependabot policies. Because `.github/dependabot.yml` is missing and `SECURITY.md` lacks these disclosures, the test fails, and dependencies remain unmonitored for known CVEs.
5. **Unsigned Release Archives & Lack of SLSA Provenance (`F-SUPPLY-RELEASE-009`, Moderate):** Release builds in `.github/workflows/release.yml` and `.goreleaser.yaml` emit SHA-256 checksums but do not generate cryptographic signatures (`cosign`) or SLSA provenance attestations. Dedicated verification tests in `pkg/supply/supply_test.go` explicitly skip execution in open-core.
6. **Incomplete Third-Party NOTICE Attribution (`F-SUPPLY-RELEASE-005`, Low):** The root `NOTICE` file enumerates 14 third-party open-source components but omits key direct runtime dependencies, including `github.com/google/go-cmp`, `github.com/joho/godotenv`, `github.com/mitchellh/go-ps`, and `go.uber.org/goleak`, as well as indirect runtime dependencies.
7. **Pervasive Absence of SPDX Identifiers (`F-SUPPLY-RELEASE-006`, Low):** Across 7,547 Go source files, 7,538 files (99.88%) have no license or copyright header in their opening lines, and zero files feature an `SPDX-License-Identifier` tag. Only 9 files carry explicit Apache-2.0 headers.
8. **Missing -trimpath & Non-Reproducible Builds (`F-SUPPLY-RELEASE-007`, Low):** Neither `Makefile` nor `.goreleaser.yaml` passes `-trimpath` to `go build`, causing workstation and CI runner filesystem paths to be embedded into binary symbols. Additionally, `Makefile` evaluates `date -u` dynamically without respecting `SOURCE_DATE_EPOCH`.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-SUPPLY-RELEASE-001` | GoReleaser configuration specifies incorrect package path for version ldflags | high | E2 | OPS, SEC | D-HIGH |
| `F-SUPPLY-RELEASE-002` | Source installation path in install.sh invokes non-existent Makefile target | moderate | E2 | OPS, ROB | D-HIGH |
| `F-SUPPLY-RELEASE-003` | Credential scanner script not wired into pre-commit hook or CI workflows | moderate | E2 | SEC, OPS | D-HIGH |
| `F-SUPPLY-RELEASE-004` | Missing Dependabot configuration causes test failure and unmonitored dependencies | moderate | E2 | SEC, MNT | D-HIGH |
| `F-SUPPLY-RELEASE-009` | Release artifacts lack cryptographic signatures (cosign) and SLSA provenance | moderate | E2 | SEC, OPS, RCV | D-HIGH |
| `F-SUPPLY-RELEASE-005` | Incomplete third-party dependency attribution in root NOTICE file | low | E2 | OPS, MNT | D-HIGH |
| `F-SUPPLY-RELEASE-006` | Prevalence of missing SPDX license identifiers and copyright headers in Go source files | low | E2 | OPS, MNT | D-HIGH |
| `F-SUPPLY-RELEASE-007` | Compilation commands lack -trimpath flag causing path leaks and non-reproducible builds | low | E2 | SEC, OPS | D-HIGH |
| `F-SUPPLY-RELEASE-008` | Committed go.mod and go.sum pass cryptographic module verification | info | E2 | SEC, ROB | D-HIGH |

---

## 3. Exhaustive Rubric Checklist Evaluation

### 3.1 Criterion 1: LICENSE / NOTICE / Copyright Consistency
- **Status:** **Partial / Defective**
- **Repository Root:** `LICENSE` contains the canonical Apache License Version 2.0 text. `NOTICE` claims Copyright 2025–2026 ZQK Contributors and provides permissive third-party software notices. `dist-docs/LICENSE` and `dist-docs/NOTICE` match root byte-for-byte.
- **Defects Identified:**
  - `NOTICE` lists 14 dependencies but omits direct dependencies declared in `go.mod`: `github.com/google/go-cmp` (BSD-3-Clause), `github.com/joho/godotenv` (MIT), `github.com/mitchellh/go-ps` (MIT), and `go.uber.org/goleak` (MIT) (`F-SUPPLY-RELEASE-005`).
  - Out of 7,547 Go source files, 7,538 (99.88%) have no license header. Zero files carry `SPDX-License-Identifier` (`F-SUPPLY-RELEASE-006`).

### 3.2 Criterion 2: Dependency Lockfiles Committed & Verified
- **Status:** **Pass / Sound**
- **Evidence:** Both `go.mod` (Go 1.26.0, toolchain go1.26.6) and `go.sum` are committed to version control. Running `go mod verify` succeeds with `all modules verified` (`F-SUPPLY-RELEASE-008`).
- No untracked, unpinned, or dangling dependencies are present. No unauthorized vendoring directory exists.

### 3.3 Criterion 3: Release Artifacts, Provenance & Reproducibility
- **Status:** **Defective**
- **Evidence:**
  - GoReleaser produces archives and SHA-256 checksums (`checksums.txt`), but ldflags target `main.version` instead of `github.com/zqk-os/zqk/cmd/zqk/app.version`. Release binaries will display `version: dev` (`F-SUPPLY-RELEASE-001`).
  - Builds omit `-trimpath`, leaking absolute build paths into binaries (`F-SUPPLY-RELEASE-007`).
  - No cryptographic signing (`cosign`) or SLSA provenance attestations are generated (`F-SUPPLY-RELEASE-009`).

### 3.4 Criterion 4: CI Presence on Primary Branches
- **Status:** **Substantial / Broken Sub-Gates**
- **Evidence:** `.github/workflows/ci.yml` provides extensive matrix testing (core-kernel, storage, cli-commands, objects-and-specs, agent-and-orchestration, validation-and-tpm, scheduler), coverage reporting (`code-coverage.yml`), and smoke testing.
- **Defects Identified:**
  - Automated CI does not execute `scripts/scan-secrets.sh` (`F-SUPPLY-RELEASE-003`).
  - Running unit tests on `pkg/processhygiene` fails due to missing `.github/dependabot.yml` and `.github/workflows/secret-scan.yml` (`F-SUPPLY-RELEASE-004`).

### 3.5 Criterion 5: Secret Scanning & .gitignore Hygiene
- **Status:** **Mixed (File Hygiene Exemplary, Tooling Unwired)**
- **Evidence:** Root `.gitignore` (361 lines) contains robust ignore rules covering credentials, `.zqk/keystore/`, `.env`, temporary dumps, and OS files. Scanning with `./scripts/scan-secrets.sh .` completed with `Zero secrets detected`.
- **Defect Identified:** `scripts/scan-secrets.sh` is not invoked by `tools/git-hooks/pre-commit` or CI workflows (`F-SUPPLY-RELEASE-003`).

### 3.6 Criterion 6: SBOM & Dependency Review Process
- **Status:** **Tooling Gap / Incomplete**
- **Evidence:** `.github/workflows/sbom.yml` generates SPDX JSON via `anchore/sbom-action@v0` on push/PR to main.
- **Defects Identified:**
  - `.github/dependabot.yml` is missing, preventing automated vulnerability alerts and dependency PR generation (`F-SUPPLY-RELEASE-004`).
  - Neither GoReleaser nor CI runs `govulncheck` to detect vulnerabilities in compiled dependencies (recorded in `tooling_gaps.md`).

### 3.7 Criterion 7: Public Installation Path Verification
- **Status:** **Partial / Source Install Broken**
- **Evidence:** `install.sh` supports public unauthenticated downloads of release tarballs with SHA-256 verification against `checksums.txt`.
- **Defect Identified:** The source build fallback executes `make -C "$src_dir" zqk`, which fails because `Makefile` defines no `zqk` target (`F-SUPPLY-RELEASE-002`).

---

## 4. Structural Illumination

Refer to diagram `diagrams/D-SUPPLY-01.md` ("ZQK Build, Release, and Installation Supply Chain Pipeline").

The supply chain lifecycle spans four tiers:
1. **Developer Workstation Plane:** Pre-commit hook enforces branch protection, CAS hash invariants, and DoD validation, but omits secret scanning (`scripts/scan-secrets.sh`).
2. **Continuous Integration Plane:** Multi-job GitHub Actions pipeline validates payload boundaries, linting, and partitioned unit tests, but lacks dependabot integration and secret scanning jobs.
3. **Release & Packaging Plane:** GoReleaser runs on git tag pushes but injects ldflags into the wrong package path (`main` instead of `cmd/zqk/app`), yielding binaries stripped of version metadata. Release checksums are published without cryptographic signatures.
4. **Consumer Installation Plane:** `install.sh` downloads and verifies binary archives cleanly, but its source-build fallback breaks when attempting to build non-existent target `zqk`.

---

## 5. Key Recommendations & Remediation Plan

1. **Fix GoReleaser Version Ldflags (`F-SUPPLY-RELEASE-001` - P0):**
   Update `.goreleaser.yaml` to specify `-X github.com/zqk-os/zqk/cmd/zqk/app.version={{.Version}}`, matching `Makefile`.
2. **Add Missing Makefile Target for Installer (`F-SUPPLY-RELEASE-002` - P1):**
   Add `zqk: compile-bin` alias in `Makefile` and align `install.sh` to call `make -C "$src_dir" all`.
3. **Wire Secret Scanning into Pre-Commit & CI (`F-SUPPLY-RELEASE-003` - P1):**
   Add `scripts/scan-secrets.sh` check to `tools/git-hooks/pre-commit` and add a static secret scanning step in `.github/workflows/ci.yml`.
4. **Materialize Dependabot Configuration (`F-SUPPLY-RELEASE-004` - P1):**
   Create `.github/dependabot.yml` for `gomod` and `github-actions`, and document policies in `SECURITY.md` to resolve failing tests in `pkg/processhygiene`.
5. **Implement Keyless Cosign Signing in Release Pipeline (`F-SUPPLY-RELEASE-009` - P2):**
   Configure Sigstore Cosign in `.github/workflows/release.yml` and `.goreleaser.yaml` for verifiable provenance and signed checksums.
6. **Synchronize NOTICE and Inject SPDX Headers (`F-SUPPLY-RELEASE-005`, `F-SUPPLY-RELEASE-006` - P2):**
   Update `NOTICE` with omitted dependencies (`google/go-cmp`, `joho/godotenv`, `mitchellh/go-ps`, `go.uber.org/goleak`) and introduce an automated license header check.
7. **Adopt -trimpath & SOURCE_DATE_EPOCH (`F-SUPPLY-RELEASE-007` - P2):**
   Add `-trimpath` to compilation flags in `Makefile` and `.goreleaser.yaml` to prevent path leaks and ensure reproducible builds.
