# Cryptography and Key Management

This document outlines the cryptographic choices, key storage, and rotation policies for the ZQK operating system.

## 1. Cryptographic Primitives

ZQK relies on modern, well-maintained cryptographic algorithms and avoids deprecated standards (e.g., no SHA-1 for signatures, no RSA < 2048).

*   **Signatures & Keypairs**: **Ed25519** is the standard for cryptographic signatures (used in `pkg/crypto` and `pkg/infrastructure/crypto`). It provides strong security with small key sizes and fast signature verification.
*   **Agent Stamps / Claims**: **JWT (v5)** using **EdDSA** (Ed25519) is used for generating and verifying cryptographic stamps (e.g., verifying commit and `agent_id` claims).
*   **Encryption**: Where symmetric encryption is required for at-rest secrets, **AES-256-GCM** is the standard.
*   **Hashing**: Credentials in the keystore are securely hashed (using bcrypt or argon2) rather than stored in plain text.

## 2. Key Management & Storage

ZQK objects and credentials are managed via the centralized keystore (`keystore_entry` objects). 

*   **Keystore Entries**: A `keystore_entry` represents a secure credential (password, token, or key material).
*   **Access Control**: 
    *   Administrators can view and edit all entries.
    *   Regular users (or specific agents) can only access their own assigned entries.
*   **Environment Integration**: For operational tasks, secrets can be injected into agent and user environments without exposing them in logs or process histories.
*   **Disk Storage**: All local persisted keys are protected by strict filesystem permissions. 

## 3. Key Rotation

Key rotation is a critical part of maintaining system integrity and limiting blast radius if a key is compromised.

*   **Agent Identity Keys (Ed25519)**:
    *   Agent keys should be rotated on a standard interval (e.g., 90 days) or immediately if an agent's workspace is compromised.
    *   Old keys are invalidated in the `truth-sentinel` and new keys must be registered in the control plane.
*   **Keystore Credentials**:
    *   Any `keystore_entry` representing a third-party API token or system password should be rotated according to the upstream provider's lifecycle.
    *   Rotation is performed via the ZQK CLI (e.g., `zqk keystore rotate <id>`), which updates the hashed secret, invalidates the old credential in caches, and bumps the version of the `keystore_entry` object.
*   **Audit Logging**: Every key generation and rotation event is recorded in the immutable WAL (Write-Ahead Log) to maintain full traceability.

## 4. Compliance

These choices align with the CEF (Codebase Evaluation Framework) requirements for secure operation and ensure that ZQK agents operate under strict, verifiable identities.
