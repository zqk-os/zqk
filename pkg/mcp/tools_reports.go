package mcp

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/reports"
)

// RegisterReportTools registers report generation tools with the MCP server
func RegisterReportTools(server *Server) {
	// PCS report
	NewToolBuilder(
		GetToolName("report_pcs"),
		"Generate Project Confidence Score (PCS) report. Returns JSON.",
	).
		AddJSONYAMLFormatProperty().
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			cmdArgs := map[string]any{
				"_command_path": GetCommandPath("reports pcs"),
			}
			if format, ok := args[objects.FieldKeyFormat].(string); ok && format != "" {
				cmdArgs[objects.FieldKeyFormat] = format
			} else {
				cmdArgs[objects.FieldKeyFormat] = "json"
			}
			return ExecuteCLICommandViaMCP(cmdArgs, pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"*"}), server.GetProjectRoot())
		})

	// EDD report
	NewToolBuilder(
		GetToolName("report_edd"),
		"Generate Effort Distribution Discrepancy (EDD) report. Returns JSON.",
	).
		AddJSONYAMLFormatProperty().
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			cmdArgs := map[string]any{
				"_command_path": GetCommandPath("reports edd"),
			}
			if format, ok := args[objects.FieldKeyFormat].(string); ok && format != "" {
				cmdArgs[objects.FieldKeyFormat] = format
			} else {
				cmdArgs[objects.FieldKeyFormat] = "json"
			}
			return ExecuteCLICommandViaMCP(cmdArgs, pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"*"}), server.GetProjectRoot())
		})

	// Blockers report
	NewToolBuilder(
		GetToolName("report_blockers"),
		"Generate Dependencies & Blockers (D&B) report. Returns JSON.",
	).
		AddJSONYAMLFormatProperty().
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			cmdArgs := map[string]any{
				"_command_path": GetCommandPath("reports blockers"),
			}
			if format, ok := args[objects.FieldKeyFormat].(string); ok && format != "" {
				cmdArgs[objects.FieldKeyFormat] = format
			} else {
				cmdArgs[objects.FieldKeyFormat] = "json"
			}
			return ExecuteCLICommandViaMCP(cmdArgs, pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"*"}), server.GetProjectRoot())
		})

	// Maturation report
	NewToolBuilder(
		GetToolName("report_maturation"),
		"Generate Maturation Report. Returns JSON.",
	).
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			return reports.GenerateMaturationReport(ctx, args)
		})

	// Vitality report
	NewToolBuilder(
		GetToolName("report_vitality"),
		"Generate Vitality Report. Returns JSON.",
	).
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			return reports.GenerateVitalityReport(ctx, args)
		})

	// QA Success report
	NewToolBuilder(
		GetToolName("report_qa_success"),
		"Generate QA Success Report. Returns JSON.",
	).
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			return reports.GenerateQASuccessReport(ctx, args)
		})
}
