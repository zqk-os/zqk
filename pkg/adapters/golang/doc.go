// Package golang adapts the Go toolchain for seat-worker completion evidence.
//
// Kernel seating (cmd/zqk/agent) asks this adapter which written files to
// revert and whether the run may complete. `.go` suffixes, `./pkg/...`
// patterns, `go vet`, and `go test` live here — not in the seat worker.
package golang
