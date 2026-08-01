package main

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/lanceman/zqk/cmd/zqk-community/app"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Public Open-Core product binary is named "zqk". Enterprise/admin surfaces use
// distinguishing names (e.g. zqk-admin). cmd/zqk-community remains the shared
// app package layout for the Apache product CLI.

// init ensures the pure-Go DNS resolver is used instead of cgo's getaddrinfo.
// cgo DNS acquires libc pthread mutexes that deadlock with fork() in
// multi-threaded processes (observed in scheduler daemon crashes).
func init() {
	_ = godotenv.Load()

	godebug := os.Getenv("GODEBUG")
	if !strings.Contains(godebug, "netdns=") {
		if godebug == "" {
			os.Setenv("GODEBUG", "netdns=go")
		} else {
			os.Setenv("GODEBUG", godebug+",netdns=go")
		}
	}

	// Open-Core / community product: local-first without enterprise RBAC gate.
	os.Setenv(zqkenv.TestBypassAuth(), "1")
	zqkenv.IsCommunityEdition = true
}

func main() {
	app.Execute()
}
