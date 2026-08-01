# Go-To-Market (GTM): Pricing & Revenue Engine

**Date:** 2026-06-02
**Context:** As ZQK prepares for the Beta Launch, we must establish the commercial mechanics. To support the "Mad Scientist" business model (high revenue, low headcount, zero sales friction), we reject traditional per-seat enterprise SaaS pricing. We are building the **Open Core / Toll Booth** model.

## 1. The Core Philosophy: "Open Core, Tax the Network"
To win developers, the ZQK engine must be irresistible and frictionless.
*   **The ZQK CLI & Local Daemon:** 100% Free and Open Source.
*   **Why:** We want every developer on earth running ZQK locally. It becomes their primary interface. We do not gate local features. If it runs on their MacBook, it's free.

## 2. The Toll Booths (Where we make money)
We charge for the **Mesh**, not the node. When a developer's local ZQK agent needs to touch the internet asynchronously or securely, they pay.

### Toll Booth 1: The Sovereign Relay (NAT Tunneling)
*   **The Problem:** The developer's free local agent needs to receive a webhook from Stripe or VEED, but it's trapped behind a local firewall.
*   **The Product:** `relay.zqkos.com` (The Cryptographic Transceiver).
*   **Pricing:** $20/month per developer (or $200/year).
*   **The Pitch:** "Stop fighting with `ngrok` and writing manual polling loops. Pay $20/month, and your local agent natively handles long-running async tasks and encrypted webhooks."

### Toll Booth 2: The Federated Economy (Agent-to-Agent Mesh)
*   **The Problem:** A company wants 50 different ZQK agents (running on different servers) to coordinate via the Model Context Protocol (MCP), sharing context and executing TDE Envelopes across boundaries.
*   **The Product:** **ZQK Enterprise Mesh Controller** (A hosted control plane).
*   **Pricing:** $500/month flat fee + usage tax on heavy bandwidth.
*   **The Pitch:** "You own the nodes; we manage the cryptographic routing and state consistency between them."

### Toll Booth 3: The API Synthesizer (The Arbitrage Play)
*   **The Problem:** Developers want their agents to generate music, videos, or SMS messages, but don't want to manage 15 different API keys (OpenAI, VEED, ILoveSong, Twilio).
*   **The Product:** ZQK Managed Capabilities.
*   **Pricing:** Pay-as-you-go markup. We charge $0.05 per API call, while it costs us $0.02 wholesale. The user just tops up a single ZQK balance via Stripe.
*   **The Pitch:** "One API key (your ZQK token) unlocks the entire intelligence ecosystem."

## 3. How do we track who paid? (The Mechanics)
We do not build bloated license-key servers. We use **Cryptographic Subscriptions**.

1.  **Stripe Checkout:** The developer buys the $20/mo Sovereign Relay pass on `zqkos.com`.
2.  **The JWT License:** Our backend generates an EdDSA (Ed25519) signed JSON Web Token (JWT) that acts as the license file.
3.  **Local Storage:** The user runs `zqk auth login`, which drops the JWT into `~/.zqk-state/license.jwt`.
4.  **Zero-Network Validation:** The ZQK daemon locally verifies the cryptographic signature of the JWT instantly. It doesn't need to phone home to check if the user is paid; the math proves it.
5.  **Relay Enforcement:** When the ZQK daemon connects to `relay.zqkos.com`, it passes the JWT. If the Stripe subscription is expired, the relay drops the connection.

## 4. Why This Works for the "Mad Scientist"
1.  **No Sales Calls:** You don't negotiate $500k contracts. Developers swipe a credit card for $20/mo to solve a painful NAT traversal problem.
2.  **No License Servers:** EdDSA signed JWTs mean the local CLI runs fast without querying a central server on every command.
3.  **Infinite Scalability:** Once the relay and payment webhook are built, supporting 1 user requires the same effort as supporting 10,000 users.
