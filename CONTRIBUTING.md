# Contributing to ZQK (Zen Quantum Kernel)

Welcome to ZQK! We are excited to collaborate with human developers and autonomous AI agent swarms alike. ZQK is built as an operating system for AI + human hybrid engineering teams.

---

## 1. Core Principles

1. **Object-First & Knowledge Kernel Primacy:**
   - Every requirement, decision, criteria, task, and architectural boundary is a typed object in the Knowledge Kernel.
   - **Never manually edit YAML files in `.zqk/process/` directly.** Always use the CLI (`zqk object ...`) or MCP tools.
2. **Traceability:**
   - Every line of code links back to a validated Criteria (`CRIT-*`), Backlog Item (`BLI-*`), and Priority Plan (`PRI-*`).
3. **PR-Only Development:**
   - No direct pushes to `main`. All work proceeds on feature or integration branches (`feature/*`, `integration/*`) and merges through pull requests.
4. **Test-Driven Development (TDD):**
   - Tests are mandatory. Write your test fixtures and assert pass/fail criteria before claiming completion.

---

## 2. Setting Up Your Development Environment

### Prerequisites
- **Go:** 1.26+ (Toolchain `go1.26.6` pinned in `go.mod`)
- **Git:** 2.40+
- **Make:** GNU Make

### Initial Build & Verification
```bash
# Clone the repository
git clone https://github.com/lanceman/zqk.git
cd zqk

# Compile the core CLI binary
make zqk

# Ad-hoc codesign the binary (macOS ARM64)
codesign -s - -f ./bin/zqk

# Verify kernel integrity
./bin/zqk system check
```

---

## 3. Working on Backlog Items / Development Workflow

1. **Find What's Next:**
   ```bash
   ./bin/zqk workflow whats-next
   ```
2. **Inspect Backlog Items:**
   ```bash
   ./bin/zqk object get BLI-xxxx
   ```
3. **Run Pre-Commit Verification Before Submitting:**
   ```bash
   ./scripts/pre-commit-gates.sh
   ./scripts/scan-secrets.sh
   ./scripts/verify-standalone-build.sh
   ```

---

## 4. Coding Standards

- **Logging Compliance (`POL-CODE-007`):**
  Never use raw `fmt.Println` or `fmt.Fprintf(os.Stderr)` in production code. Use structured logging (`logging.Fluent`).
- **Resource Hygiene:**
  Always close and sync open file handles via `defer`. Use `fileutil` abstraction instead of direct `os` calls.
- **Architectural Clarity:**
  Keep command entry points lean (`cmd/zqk/...`) and business logic encapsulated in `pkg/...`.

---

## 5. Submitting Pull Requests / Pull Request Process

1. Create a descriptive branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```
2. Ensure pre-commit gates pass cleanly:
   ```bash
   ./scripts/pre-commit-gates.sh
   ```
3. Open a Pull Request on GitHub using our standard PR template (`.github/PULL_REQUEST_TEMPLATE.md`).
4. Ensure CI tests pass. A maintainer or peer agent will review and merge.
