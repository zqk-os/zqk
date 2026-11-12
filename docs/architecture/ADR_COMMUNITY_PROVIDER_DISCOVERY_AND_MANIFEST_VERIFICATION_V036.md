# ADR-0040: Community Provider Discovery & Distributed Manifest Verification v0.3.6

- **Title: Community Provider Discovery & Distributed Manifest Verification v0.3.6**
- **Status: Accepted**
- **Date**: 2026-11-12
- **Author**: Community Core Team

## Context

As the community ecosystem scales in v0.3.6, providers and plugins need to be discovered dynamically from configured directory roots, archives, and distributed sources without hardcoding runtime dependencies. To guarantee zero supply-chain poisoning and structural compliance, each discovered provider must present a schema-validated manifest with cryptographic signature verification before being loaded into the provider registry.

## Decision

1. **Manifest Schema & Specification**:
   - Every provider package MUST contain a `provider.json` manifest specifying `id`, `name`, `version`, `author`, `min_engine_version`, and a declared set of `capabilities`.
   - Manifest parsing validates semantic version format and ensures all required fields are populated.

2. **Dynamic Discovery Scanner**:
   - The `ProviderDiscoveryScanner` traverses candidate search paths (e.g. system plugin paths, workspace plugin paths) and discovers provider directories.
   - For each candidate directory, the scanner parses the manifest, verifies structural compliance, and returns typed `DiscoveredProvider` records.

3. **Cryptographic & Integrity Verification**:
   - Manifests include an optional or required SHA256 integrity digest and cryptographic signature attestation.
   - Tampered files, invalid hashes, or unsupported schema versions are rejected fail-closed with structured errors (`ErrManifestInvalid`, `ErrIntegrityMismatch`).

4. **Ontological Traceability & Quality Gate**:
   - Traceable directly to parent Community Ecosystem Extensibility goals and active execution cycles.
   - Adheres to `POL-DOC-001` with zero internal cycle sprint references in public documentation.

## Consequences

- Open-core ecosystem extensions can be safely discovered, inspected, and verified ahead of runtime initialization.
- Supply chain integrity is verified before executing any external provider code.
- Dynamic plugin modularity is achieved without sacrificing deterministic safety or system stability.
