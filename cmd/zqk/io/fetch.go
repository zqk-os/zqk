package io

import (
	"encoding/json"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	iopkg "github.com/zqk-os/zqk/pkg/io"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewFetchCmd creates the 'zqk io fetch' command.
func NewFetchCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Fetch HTTP URL content and extract clean text",
		"Fetches HTTP URL content, extracts clean Markdown/plaintext (stripping scripts, styles, nav, footers), and computes SHA256 digest.",
		"",
		"When --cache-cas is specified, persists the raw and extracted content into the Knowledge Kernel CAS keyed by hash.",
		"When --diff is specified and a previous fetch exists, outputs a semantic diff.",
	).
		AddExample("Fetch and extract clean Markdown", paths.CLIUsage("io", "fetch", "https://example.com/docs")).
		AddExample("Fetch and cache into CAS", paths.CLIUsage("io", "fetch", "https://example.com/docs", "--cache-cas")).
		AddExample("Fetch and diff against cached version", paths.CLIUsage("io", "fetch", "https://example.com/docs", "--diff", "--cache-cas"))

	cmd := &cobra.Command{
		Use:   "fetch <url>",
		Short: "Fetch HTTP URL content and extract clean text",
		Long:  "Fetches HTTP URL content, extracts clean Markdown/plaintext (stripping scripts, styles, nav, footers), and computes SHA256.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetURL := args[0]

			var flagsBag clipkg.FlagBag
			cacheCAS := flagsBag.Bool(cmd, "cache-cas")
			diffMode := flagsBag.Bool(cmd, "diff")
			rawMode := flagsBag.Bool(cmd, "raw")
			jsonOutput := flagsBag.Bool(cmd, "json")
			timeoutStr := flagsBag.String(cmd, "timeout")

			if flagsBag.Err() != nil {
				return flagsBag.Err()
			}

			var timeout time.Duration
			if timeoutStr != "" {
				var err error
				timeout, err = time.ParseDuration(timeoutStr)
				if err != nil {
					return errfmt.Newf("invalid --timeout value %q: %v", timeoutStr, err)
				}
			}

			var store storage.ObjectStorageProvider
			var projectRoot string

			proc, err := cli.NewProcessor(cmd)
			if err == nil && proc != nil {
				store = proc.Storage()
				projectRoot = proc.ProjectRoot()
			}

			opts := iopkg.FetchOptions{
				CacheCAS:    cacheCAS,
				Diff:        diffMode,
				Raw:         rawMode,
				Timeout:     timeout,
				ProjectRoot: projectRoot,
			}

			result, err := iopkg.Fetch(cmd.Context(), store, iopkg.DefaultThrottler, targetURL, opts)
			if err != nil {
				return err
			}

			if jsonOutput {
				out, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return errfmt.Newf("failed to format JSON: %v", err)
				}
				cmd.Println(string(out))
				return nil
			}

			if diffMode && result.Diff != "" {
				cmd.Println("================================================================================")
				cmd.Printf("SEMANTIC DIFF: %s\n", targetURL)
				cmd.Println("================================================================================")
				cmd.Println(result.Diff)
				cmd.Println("================================================================================")
				return nil
			}

			if rawMode {
				cmd.Print(result.RawContent)
				return nil
			}

			cmd.Println("================================================================================")
			cmd.Printf("URL:        %s\n", result.URL)
			cmd.Printf("Domain:     %s\n", result.Domain)
			cmd.Printf("Status:     %d\n", result.StatusCode)
			cmd.Printf("Raw Hash:   %s\n", result.RawHash)
			cmd.Printf("Clean Hash: %s\n", result.CleanHash)
			if result.Cached {
				cmd.Printf("Cached CAS: true\n")
			}
			cmd.Println("--------------------------------------------------------------------------------")
			cmd.Println(result.CleanContent)
			cmd.Println("================================================================================")

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().Bool("cache-cas", false, "Store raw and clean content into CAS keyed by hash")
	cmd.Flags().Bool("diff", false, "Output semantic diff against previous cached fetch if available")
	cmd.Flags().Bool("raw", false, "Output raw fetched response body instead of extracted clean text")
	cmd.Flags().Bool("json", false, "Output result in JSON format")
	cmd.Flags().String("timeout", "30s", "HTTP request timeout duration")

	return cmd
}
