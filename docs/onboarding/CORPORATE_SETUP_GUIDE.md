# ZQK Corporate & Infrastructure Setup Guide

This document captures the strategic execution plan for formally establishing ZQK as a corporate entity, securing its Intellectual Property (IP), and setting up the foundational infrastructure.

## Phase 1: Legal Incorporation
*   **Platform:** **Clerky** (Lifetime Package recommended)
*   **Entity:** Delaware C-Corporation
*   **Action Items:**
    1.  File the Articles of Incorporation via Clerky.
    2.  Obtain Employer Identification Number (EIN).
    3.  **Critical:** Execute the Confidential Information and Invention Assignment Agreements (CIIA) to legally transfer the ZQK codebase, GraphRAG kernel, and Symbiotic Mesh IP from the founder to the corporation.
    4.  Establish the initial capitalization table (Founder shares, vesting schedules).

## Phase 2: Financial Infrastructure
*   **Platform:** **Mercury** (mercury.com)
*   **Action Items:**
    1.  Once the EIN and Articles of Incorporation are received from Clerky, apply for a startup checking account on Mercury.
    2.  *Note:* Do not set up Stripe until a payment gateway is explicitly required by the Go-To-Market strategy.

## Phase 3: Digital Real Estate & Brand
*   **Action Items:**
    1.  Acquire primary domains via Cloudflare Registrar. Prioritize the OS narrative.
        *   Target: `zqkos.com` (Primary)
        *   Target: `zqk.dev` (Docs/API)
        *   Target: `zqk.network` (Mesh infrastructure)
    2.  Set up the corporate Google Workspace (or equivalent) using the primary domain.
    3.  Create the `zqk-agent@zqk.ai` (or similar) email address to act as the autonomous identity for the ZQK mesh (required for VEED and ILoveSong API integration).

## Phase 4: API & Automation Bindings
*   **Action Items:**
    1.  Use the `zqk-agent` email to register for **ILoveSong.ai** and generate an API key.
    2.  Use the `zqk-agent` email to register for **VEED.io** (fal.ai) and generate an API key.
    3.  Add keys to the local `.env` to enable the Automated Marketing Synthesizer capability.
