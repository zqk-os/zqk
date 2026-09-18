package main

import (
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/zqk-os/zqk/cmd/zqk-community/app"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// init ensures the pure-Go DNS resolver is used instead of cgo's getaddrinfo.
// cgo DNS acquires libc pthread mutexes that deadlock with fork() in
// multi-threaded processes (observed in scheduler daemon crashes).
func init() {
	// Load environment variables from .env file (if present) from project root.
	projectRoot := paths.ResolveProjectRoot(".")
	envPath := filepath.Join(projectRoot, ".env")
	if _, err := fileutil.Stat(envPath); err == nil {
		_ = godotenv.Load(envPath)
	}

	godebug := zqkenv.GoDebug().Get()
	if !strings.Contains(godebug, "netdns=") {
		if godebug == "" {
			_ = zqkenv.GoDebug().Set("netdns=go")
		} else {
			_ = zqkenv.GoDebug().Set(godebug + ",netdns=go")
		}
	}

	// Community edition: frictionless local kernel. Auth middleware seats the
	// system account when no token is present — not ACC-TEST-HARNESS.
	zqkenv.IsCommunityEdition = true
}

// main is the entry point for the primary zqk CLI binary.
func main() {
	app.Execute()
}
