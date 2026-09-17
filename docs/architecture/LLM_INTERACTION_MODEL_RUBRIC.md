# LLM interaction model rubric

**Last Verified:** 2026-08-31


**Status:** Active operator note (2026-08-27)  
**Code:** `pkg/llm/model_role.go`, `pkg/swarm/model_harness.go`  
**Related:** `docs/best-practices/local_inference_testing.md`, `docs/architecture/NATIVE_AGENT_EXECUTION_LOOP.md`  
**TRACK:** `BLI-SWM-002` (doer vs draft harness). Small-model lanes below are **documented, not scheduled**.

This is a selection rubric, not a second object store. `provider_profile.model_tier` (`tier_1_complex` / `tier_2_simple`) already exists on ATKs; it is **not** wired to this table yet.

## 0. Phased strategy

| Phase | What we prove | What we do not build yet |
|---|---|---|
| **0 — one local doer** | A seat pointed at a local open-weight model completes AgentX: native or recovered **write**, compile + test. If this works on Ollama, the same loop works on frontier APIs (they already speak `tool_calls`). | Multi-host inventory, context routers, “smart” model pickers. |
| **1 — role split** | Doer vs draft vs prose vs embed as **named clients**, still one machine. | Cross-machine Ollama. |
| **2 — route by request** | `provider_profile` / interaction kind / ATK `model_tier` pick a **base URL + model** (external Ollama hosts). | Fancy classifiers; start from the table in §1–§2. |

Phase 0 write landed on `qwen3.6:latest` (native `zqk_write_file` on a one-file probe). Night-duty ATKs then showed the next gap: 3.6 **does** emit native `tool_calls` but **ignores the named write pin** and looks up until the 10-minute seat timeout. Host now caps unpaid lookup/read streaks (`unpaidLookupBudget` / `unpaidLookupForceWrite` in `pkg/swarm`) instead of adding more prompt text.

Night-duty deliverables then showed a second gap: the execute prompt only *pointed* at the observer. Doers never received `observer_search` on the wire and invented `src/` / new packages. Execute-layer cognition now gets a live, task-scoped AST hint plus the MCP tool (`pkg/observer.Search`, `zqk_observer_search`). Persist envelopes stay pointer-only — never write a census onto `agent_task`.

## 1. Interactions we actually run

Judge a model by the **channel**, not by “is it good at code?”

| Interaction | Call site | Contract | Failure we have seen |
|---|---|---|---|
| **AgentX doer** | `seat_worker` / `swarm_worker` → `GenerateStructuredCompletion` + MCP | Native OpenAI `tool_calls` (or recovered JSON that is a **write**). Outcome: compile + test. | Coder-7B: `toolCalls=0`, markdown `object_get`, park with no write. |
| **Code draft** | Same loop after `ClassifyChatModel` → `code_draft` | Write `path`+`content` (`write_code` / `write_file`). Menu is write/read/test only. | Using the full eager tool list teaches lookup-first. |
| **Prose completion** | `chat_responder`, `sync_loop`, `intake`, `observer/coach`, CAP sentinel | `GenerateCompletion` — no tools. | Over-sized model wastes VRAM; under-sized is fine here. |
| **Intent / embed** | `GenerateIntent`, `GenerateEmbedding` (observer, hivemind, semantic cache) | Short text + vectors. Separate **embed** model. | Pointing chat at an embed endpoint, or vice versa. |
| **COMMS life∧work** | Deterministic seat-worker + feed | Nonce ack + kernel probe. AgentX-class work needs a **successful MCP result**, not a chat model. | Treating notify/toast as presence. |

Human/IDE Composer is **outside** this table. Mesh doer seats are not “another Cursor.”

## 2. Decision rubric (now)

Use the first matching row.

1. **Need MCP tools in a loop (coding ATK, AgentX COMMS work)?** → **Doer** model with a matching **tool-call parser**. Default today: `ZQK_LLM_CHAT_MODEL=qwen3.6:latest` (Ollama). Prefer **Qwen3 / Qwen2.5-Instruct + Hermes** over **Qwen2.5-Coder** on the OpenAI tools API.
2. **Need a file body and the only local weight is a small coder?** → **Draft** harness (already automatic for `qwen2.5-coder:*` and `*coder*7b/8b`). Do not expand the tool menu.
3. **Need a paragraph, title, or coach line?** → Small instruct (7B–9B) is enough. Do not occupy the doer GPU.
4. **Need similarity / search?** → Embed model (`ZQK_LLM_EMBED_MODEL`), not the chat tag.
5. **VRAM cannot hold a doer?** → Keep the **7B as draft/prose**; send doer ATKs to a larger local weight or cloud (`LLM_SECONDARY_*`). Do not “make 7B AgentX” with more prompt text.

**Now vs later:** the live priority is **reliable doer seats**. Small-model lanes (FIM, dual-model plan+write, cheap prose) stay valid and are **not** this session’s build.

## 3. Model × job (industry + our evidence)

Sources consulted 2026-08-27: Qwen function-calling docs (vLLM `--tool-call-parser hermes` for Qwen2.5-Instruct); Qwen GH on Coder ≠ Hermes; InsiderLLM mid-2026 Qwen ranking; OrcaRouter local-coding VRAM guide (2026-08-10); Qwen3-Coder-Next model card (parser `qwen3_xml` / `qwen3_coder`, sampling `temperature=1.0` for that family).

| Weight (typical tag) | Industry “optimal deploy” | ZQK job | Parser / template | Priority |
|---|---|---|---|---|
| **Qwen3.6 / 3.8 27B** (e.g. `qwen3.6:latest`) | Best open **agentic** coder on ~24GB (SWE-bench / agent indexes). | **Doer** default. | Qwen3 chat template; thinking off for tool turns if the server exposes it. | **Now** |
| **Qwen3-Coder-30B-A3B** | 24GB agent pick when you want coder-tuned MoE + long ctx. | Doer if Ollama/vLLM parser is **qwen3_*** not `hermes`. | `qwen3_xml` (vLLM). | Now if it is what you already serve |
| **Qwen3-Coder-Next 80B-A3B** | Canonical large local coding **agent** (64GB+ unified). Card: `temp=1.0`, `top_p=0.95`. | Doer for long ATKs when hardware exists. | `qwen3_xml` / `qwen3_coder` — **not** Hermes. | When VRAM allows |
| **Qwen2.5-7B/14B/32B-Instruct** | Hermes-style tools if vLLM `--enable-auto-tool-choice --tool-call-parser hermes` (or llama.cpp `--jinja`). | Doer fallback if Qwen3 is unavailable. | Hermes | Now as fallback |
| **Qwen2.5-Coder-7B** | Strongest **8GB code-completion / FIM** (HumanEval-class). Weak as an OpenAI tools agent. | **Draft + FIM only.** Harness already classifies `code_draft`. | Qwen2 / Qwen-Agent (not Hermes). | **Documented; not this week** |
| **Qwen2.5-Coder-32B** | Still useful **FIM / autocomplete** on 24GB; superseded for agents by Qwen3.6-27B. | Draft or editor complete — not AgentX default. | Same Coder template issue on `/v1` tools. | Later |
| **Qwen 3.5 9B** | Best **general** 8GB instruct. | Prose / intake / coach. | Instruct template; tools only if the server parses them. | Later (prose split) |
| **Cloud (Gemini / Qwen-Max)** | Highest first-pass agent quality. | Secondary doer (`LLM_SECONDARY_*`) when local doer is down. | Provider native tools. | As needed |

**Rule experts keep repeating:** pick by **VRAM and job column** (agent vs complete), then match the **parser to the family**. A 7B that scores well on HumanEval can still return `tool_calls: []` on our wire.

## 4. Sampling (do not copy one card onto every family)

| Family | Tools / AgentX | Prose / draft |
|---|---|---|
| Qwen2.5 Instruct + Hermes | `temperature=0` (or omit) | 0.7 / `top_p=0.8` (Qwen chat blog) |
| Qwen2.5-Coder draft | `0` (we set this when classified draft) | 0.2–0.5 for FIM later |
| Qwen3-Coder-Next card | `1.0` / `top_p=0.95` / `top_k=40` | Follow the card if you serve that weight |
| Env knobs | `ZQK_LLM_TEMPERATURE`, `ZQK_LLM_TOP_P` (omit = server default) | same |

## 5. Later (explicitly not now)

- **Dual-model:** doer plans the path; Coder-7B fills `write_code` content (`LLM_SECONDARY_*` or a dedicated draft client).
- **Map `model_tier` → this rubric** so `tier_1_complex` cannot land on a draft weight.
- **FIM / autocomplete** path that never enters `swarm.Engine`.
- **Prose pool** (7B–9B) so AgentX does not share a GPU with chat-responder.

Until then: one doer default (`qwen3.6:latest`), draft harness if someone still points a seat at Coder-7B, traces at `.zqk/logs/llm/`.

## 6. Future pulls (objectified, not this week)

Already on this studio — **do not re-pull:** `qwen3.6:latest`, `zqk-qwen:latest`, `llama3.1:latest`, `qwen2.5-coder:7b`. Phase 0 still uses `qwen3.6:latest`.

| When | Ollama tag | Job | BLI | Status |
|---|---|---|---|---|
| After Phase 0 write lands | `qwen2.5:7b-instruct` | Hermes control: same size as Coder-7B, Instruct template. Optional follow `qwen2.5:14b-instruct` if 7B Instruct is also `toolCalls=0` (`BLI-1787824086124576000-c8d68aff`, still exploring). | `BLI-1787823835447254000-255d29d7` | validated (P2) |
| After Phase 0, if 3.6 is weak | `qwen3-coder` | AgentX doer candidate (`qwen3_*` parser, not Hermes) | `BLI-1787823836830521000-c90fb1b7` | validated (P2) |
| Phase 1 prose split | `qwen3.5:9b` | Intake / coach / chat-responder — **not** AgentX | `BLI-1787823838145269000-6e1ae4f0` | validated (P3) |
| When a 64GB+ host exists | `qwen3-coder-next` | Long-ATK doer; card `temp=1.0` | `BLI-1787823839546074000-3d025dfb` | **deferred** (P3) |

Do not `ollama pull` these until the matching BLI is in progress. Do not park those BLIs while they still share `REQ-1785885786383550000-cd8e26b6` — `object park` cluster-deferred that shared REQ on 2026-08-27; revive edge is `requirement` `deferred→active`.
