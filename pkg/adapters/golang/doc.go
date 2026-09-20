// Package golang adapts the Go toolchain for seat-worker completion evidence
// and the isolated-worktree pre-merge compile gate.
//
// Kernel seating (cmd/zqk/agent) asks this adapter which written files to
// revert, whether the run may complete, and whether the worktree CLI package
// compiles. `.go` suffixes, `./cmd/<product>`, `go vet`, `go test`, and
// `go build` live here — not in the seat worker.
package golang
