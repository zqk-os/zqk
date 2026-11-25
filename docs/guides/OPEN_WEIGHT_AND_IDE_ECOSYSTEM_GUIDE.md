# Open-Weight Inference & Local-First IDE Ecosystem Guide

> **Audience:** Engineering teams running local/air-gapped open-weight models (`Ollama`, `Together AI`, `Fireworks AI`, `Baseten`, `Groq`, `OpenRouter`, `DeepSeek`) or open/local-first coding harnesses (`Cline`, `Roo Code`, `Zed`, `Continue`, `Aider`, `Cursor`, `Claude Code`, `Windsurf`, `Gemini CLI`) with ZQK.

---

## 1. Why Pair Open-Weight Models with ZQK?

Raw open-weight coding models (`Qwen2.5-Coder-32B`, `DeepSeek-R1-Distill`, `Llama 3.3 70B`) often degrade on large, unindexed monorepos when fed raw `grep` or `find` dumps. ZQK eliminates that context tax locally:

1. **Sub-15ms Polyglot AST & Trigram Search (`zgrep`):** Returns surgical, token-budgeted JSON slices (`--max-tokens 2000 -f json`) across Go, TypeScript/JS, Python, and Rust.
2. **Portable Context Handoffs (`pkg/sessionpaging/handoff.go`):** Builds deterministic, token-budgeted Markdown/JSON handoff packets (`DefaultHandoffMaxTokens = 2000`) so teams can switch between Claude Code, Cursor, Gemini, and local open-weight models mid-task with minimal Amnesia Tax.
3. **Fail-Closed Verification (`REQ → CRIT → TST`):** Deterministic done-gates (`zqk test run --all`, `zqk workflow vds evaluate`, `zqk gate stage|review|approve`) verify edits before PR review regardless of which LLM authored the diff.

---

## 2. Canonical Open-Weight Inference Presets (`pkg/llm/openweight_presets.go`)

ZQK provides canonical OpenAI-compatible presets via `llm.OpenWeightPresets()` and `llm.RegisterOpenWeightPresets(reg)`:

| Engine ID | Provider | Base URL | Default Coding Model | Auth Env Var |
|---|---|---|---|---|
| `ollama` | Ollama (Air-Gapped Local) | `http://127.0.0.1:11434/v1` | `qwen2.5-coder:32b` | *(none — 100% local)* |
| `together` | Together AI | `https://api.together.xyz/v1` | `Qwen/Qwen2.5-Coder-32B-Instruct` | `TOGETHER_API_KEY` |
| `fireworks` | Fireworks AI | `https://api.fireworks.ai/inference/v1` | `accounts/fireworks/models/qwen2p5-coder-32b-instruct` | `FIREWORKS_API_KEY` |
| `baseten` | Baseten | `https://inference.baseten.co/v1` | `deepseek-ai/DeepSeek-R1-Distill-Qwen-32B` | `BASETEN_API_KEY` |
| `groq` | Groq | `https://api.groq.com/openai/v1` | `llama-3.3-70b-versatile` | `GROQ_API_KEY` |
| `openrouter` | OpenRouter | `https://openrouter.ai/api/v1` | `qwen/qwen-2.5-coder-32b-instruct` | `OPENROUTER_API_KEY` |
| `deepseek` | DeepSeek | `https://api.deepseek.com/v1` | `deepseek-reasoner` | `DEEPSEEK_API_KEY` |

---

## 3. Multi-Host IDE & Local-First Harness Onboarding

Run a single command to detect installed hosts, wire workspace MCP servers, and prime regenerable kernel boot directives:

```bash
# Lightweight Day-1 setup (zgrep + multi-host IDE rules + MCP + PreToolUse hooks)
zqk init --lite --with-mcp --with-hooks

# Re-detect hosts and refresh regenerable boot packs
zqk system agent-onboard
```

### Supported Harnesses & Workspace Paths

| Harness | Detection Markers | Directive File | Workspace MCP Config |
|---|---|---|---|
| **Cursor** | `.cursor`, `.cursorrules` | `.cursor/rules/` | `.cursor/mcp.json` |
| **Claude Code** | `.claude`, `CLAUDE.md` | `CLAUDE.md` | `.claude.json` |
| **Cline** | `.cline`, `.clinerules` | `.clinerules` | `.cline/mcp_settings.json` |
| **Roo Code** | `.roo`, `.roorules` | `.roorules` | `.roo/mcp.json` |
| **Zed** | `.zed`, `.rules` | `.rules` | `.zed/settings.json` (`context_servers`) |
| **Continue** | `.continue`, `.continuerules` | `.continuerules` | `.continue/config.yaml` (`zqk mcp serve`) |
| **Aider** | `.aider.conf.yml`, `CONVENTIONS.md` | `CONVENTIONS.md` | `.aider.conf.yml` (`zqk grep --ast --max-tokens 2000 -f json`) |
| **Windsurf** | `.windsurf`, `.windsurfrules` | `.windsurfrules` | `.codeium/windsurf/mcp_config.json` |
| **Gemini CLI** | `.gemini`, `GEMINI.md` | `GEMINI.md` | `zqk mcp serve` |
| **Ollama** | `.ollama`, `OLLAMA.md` | `OLLAMA.md` | `127.0.0.1:11434` |
| **Headless / Universal** | `AGENTS.md`, `.agents/AGENTS.md` | `.agents/AGENTS.md` | `zqk mcp serve` |

---

## 4. CI & PR Markdown Scorecards

Emit deterministic GitHub-Flavored Markdown reports directly into PR comments or `$GITHUB_STEP_SUMMARY`:

```bash
# Self-benchmark token savings on your own repo in Markdown
zqk grep --bench "func " -f markdown >> "$GITHUB_STEP_SUMMARY"

# Run an evaluation template and emit a Markdown scorecard
zqk eval run --template .zqk/eval/templates/go-ast-hygiene.yaml --target pkg/eval --auto-mint=false -f markdown >> "$GITHUB_STEP_SUMMARY"

# Emit a multi-model convergence matrix scorecard in Markdown
zqk eval matrix --session-id CVS-001 --template .zqk/eval/templates/go-ast-hygiene.yaml -f markdown >> "$GITHUB_STEP_SUMMARY"

# Emit VDS release/launch done-gate status in Markdown
zqk workflow vds evaluate -f markdown >> "$GITHUB_STEP_SUMMARY"

# Emit gate approval queue and discovered test suite inventory in Markdown
zqk gate list -f markdown >> "$GITHUB_STEP_SUMMARY"
zqk test discover pkg/ -f markdown >> "$GITHUB_STEP_SUMMARY"
```
