package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/packrecord"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	_ "github.com/zqk-os/zqk/pkg/zqkenv/agentguard"
)

const netdnsFlag = "netdns="

const (
	msgDotenvLoadFailed     = "failed to load .env file"
	msgGoDebugSetFailed     = "failed to apply netdns=go GODEBUG pin"
	msgPackRecordLoadFailed = "failed to load recorded packs"
)

// init ensures the pure-Go DNS resolver is used instead of cgo's getaddrinfo.
// cgo DNS acquires libc pthread mutexes that deadlock with fork() in
// multi-threaded processes (observed in scheduler daemon crashes).
func init() {
	// Load environment variables from .env file (if present) from project root.
	logger := logging.GetLogger()
	projectRoot := paths.ResolveProjectRoot(".")
	envPath := filepath.Join(projectRoot, ".env")
	if _, err := fileutil.Stat(envPath); err == nil {
		if err := godotenv.Load(envPath); err != nil {
			logging.FluentEvent(logger).
				Error(msgDotenvLoadFailed, err).
				String("path", envPath).
				Log()
		}
	}

	godebug := zqkenv.GoDebug().Get()
	if !strings.Contains(godebug, netdnsFlag) {
		var next string
		if godebug == "" {
			next = "netdns=go"
		} else {
			next = godebug + ",netdns=go"
		}
		if err := zqkenv.GoDebug().Set(next); err != nil {
			logging.FluentEvent(logger).
				Error(msgGoDebugSetFailed, err).
				String("godebug", next).
				Log()
			os.Exit(1)
		}
	}
}

// main is the entry point for the primary zqk CLI binary.
func main() {
	registerIncludedWorkPack()
	if err := packrecord.Load(paths.ResolveProjectRoot(".")); err != nil {
		logging.FluentEvent(logging.GetLogger()).
			Error(msgPackRecordLoadFailed, err).
			Log()
	}
	app.Execute()
}
