package bldr_cli_cmd_v1

import "github.com/spf13/cobra"

// NewSchedulerTestFailuresAnalyzeCommandBuilder creates the analyze command builder.
func NewSchedulerTestFailuresAnalyzeCommandBuilder() *cobra.Command {
	return NewSchedulerAnalyzeCommandBuilder()
}

// NewSchedulerTestFailuresCriteriaEvidenceCommandBuilder creates the criteria-evidence command builder.
func NewSchedulerTestFailuresCriteriaEvidenceCommandBuilder() *cobra.Command {
	return NewSchedulerCriteriaEvidenceCommandBuilder()
}

// NewSchedulerTestFailuresHealthCommandBuilder creates the health command builder.
func NewSchedulerTestFailuresHealthCommandBuilder() *cobra.Command {
	return NewSchedulerHealthCommandBuilder()
}

// NewSchedulerTestFailuresListCommandBuilder creates the list command builder.
func NewSchedulerTestFailuresListCommandBuilder() *cobra.Command {
	return NewSchedulerListCommandBuilder()
}

// NewSchedulerTestFailuresRerunCommandBuilder creates the rerun command builder.
func NewSchedulerTestFailuresRerunCommandBuilder() *cobra.Command {
	return NewSchedulerRerunCommandBuilder()
}
