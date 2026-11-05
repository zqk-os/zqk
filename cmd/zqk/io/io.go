package io

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewIOCmd creates the 'zqk io' command group for external respiration gateway operations.
func NewIOCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Universal external respiration gateway and CAS scraping cache",
		"Universal External Respiration Gateway and CAS Scraping Cache for fetching external URLs, extracting clean Markdown/plaintext, computing cryptographic SHA256 digests, and persisting content-addressed cache blobs with semantic diffing.",
		"",
		"Ensures multi-agent swarms can consume web content safely with rate limiting, zero token-waste HTML stripping, and immutable CAS caching.",
	).
		AddExample("Fetch URL and extract clean Markdown", paths.CLIUsage("io", "fetch", "https://example.com/api-docs")).
		AddExample("Fetch and cache into CAS", paths.CLIUsage("io", "fetch", "https://example.com/spec", "--cache-cas")).
		AddExample("Fetch and output semantic diff against previous cache", paths.CLIUsage("io", "fetch", "https://example.com/spec", "--diff", "--cache-cas"))

	cmd := &cobra.Command{
		Use:   "io",
		Short: "Universal external respiration gateway and CAS scraping cache",
		Long:  "Universal External Respiration Gateway and CAS Scraping Cache for fetching external content with clean extraction and CAS caching.",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewFetchCmd())

	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, true)

	return cmd
}
