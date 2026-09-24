# Adapter example: Go

**Not part of CEF core.** Enable only when `adapters_enabled` includes `go`.

## Idiom citations

| Work | Use for |
|------|---------|
| Effective Go | Naming, concurrency basics, errors |
| Go Code Review Comments | Common idioms |
| Go Memory Model | Concurrency findings |
| Google Go Style Guide | Package/API design |

## Recommended tools (prefer if present)

| Tool | Signal |
|------|--------|
| `go vet`, `staticcheck`, `golangci-lint` | Quality/correctness |
| `go test -race` (narrow) | Concurrency |
| AST-based project tools (e.g. custom Go AST scanners) | Exhaustive literal/pattern policy |
| `govulncheck` | Supply/security |

## D-HIGH notes

- Magic strings that should be typed constants/enums.  
- Ignored errors (`_ =`) on I/O.  
- `panic` in libraries vs commands.  

## Pros / cons of a Go adapter

| Pros | Cons |
|------|------|
| Higher precision on Go repos | Must not leak into non-Go evaluations |
| Matches real tooling | Overfitting to one org’s linter config — keep optional |
