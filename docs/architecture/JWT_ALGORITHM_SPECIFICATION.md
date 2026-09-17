# ZQK Subsystem JWT Algorithm Specification (K:F-SEC-003 / CRIT-CEF-R8K-SEC-003)

**Last Verified:** 2026-08-31


## Centralized Algorithm Allowlists
All subsystems parsing JWT tokens MUST enforce strict algorithm allowlists using `jwt.WithValidMethods` to prevent algorithm confusion attacks (`none`, RSA/HMAC confusion).

| Subsystem | Package | Permitted Algorithms | Key Type | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **Agent Attestation** | `pkg/agent`, `pkg/crypto` | `EdDSA` | Ed25519 (256-bit) | Agent cryptographic identity & action stamps |
| **Scheduler Webhooks** | `pkg/scheduler` | `HS256`, `HS384`, `HS512` | HMAC secret | Webhook HMAC bearer authorization |
| **License Verification** | `pkg/license` | `RS256`, `RS384`, `RS512` | RSA Public Key | Offline license token verification |

## Enforcement
Every `jwt.Parse` / `jwt.ParseWithClaims` call MUST include `jwt.WithValidMethods(...)` matching the table above.
