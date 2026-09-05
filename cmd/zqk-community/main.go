package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/lanceman/zqk/cmd/zqk-community/app"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
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

	godebug := os.Getenv("GODEBUG")
	if !strings.Contains(godebug, "netdns=") {
		if godebug == "" {
			os.Setenv("GODEBUG", "netdns=go")
		} else {
			os.Setenv("GODEBUG", godebug+",netdns=go")
		}
	}

	// COMMUNITY EDITION: Permanently bypass strict enterprise RBAC auth.
	// This grants local users frictionless access to the sovereign local kernel
	// without needing a cloud session.
	os.Setenv(zqkenv.TestBypassAuth(), "1")
	zqkenv.IsCommunityEdition = true
}

// main is the entry point for the primary zqk CLI binary.
func main() {
	app.Execute()
}
