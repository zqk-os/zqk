#!/usr/bin/env bash
# scripts/open-core/test-normalization/normalize_cycle_test_names.sh
# Automated Normalization of Cycle-Named Test Files across cmd and pkg.
# Supports --dry-run (default), --check-collisions, --validate, --apply
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

MODE="dry-run"
if [[ $# -gt 0 ]]; then
  case "$1" in
    --apply) MODE="apply" ;;
    --check-collisions) MODE="check-collisions" ;;
    --validate) MODE="validate" ;;
    --dry-run) MODE="dry-run" ;;
    *)
      echo "Usage: $0 [--dry-run | --check-collisions | --validate | --apply]"
      exit 1
      ;;
  esac
fi

echo "=== ZQK Repo Hygiene: Test File Normalizer ==="
echo "Mode: $MODE"

# Define Mapping Array (Source|Target|IsExcluded)
MAPPINGS=(
  "cmd/zqk/agent/agent_coverage_elevation_cycle137_test.go|cmd/zqk/agent/agent_subcommands_and_guidance_test.go|0"
  "cmd/zqk/agent/agent_coverage_elevation_cycle92_test.go|cmd/zqk/agent/agent_hooks_and_orchestration_test.go|0"
  "cmd/zqk/ambient/ambient_coverage_elevation_cycle131_test.go|cmd/zqk/ambient/ambient_subcommand_daemon_test.go|0"
  "cmd/zqk/app/app_coverage_elevation_cycle125_test.go|cmd/zqk/app/app_root_pre_test.go|0"
  "cmd/zqk/automation/automation_coverage_elevation_cycle114_test.go|cmd/zqk/automation/automation_docman_sync_test.go|0"
  "cmd/zqk/callback/callback_coverage_elevation_cycle108_test.go|cmd/zqk/callback/callback_operational_dynamics_test.go|0"
  "cmd/zqk/ci/ci_coverage_elevation_cycle113_test.go|cmd/zqk/ci/ci_command_checkout_test.go|0"
  "cmd/zqk/convergence/convergence_coverage_elevation_cycle107_test.go|cmd/zqk/convergence/convergence_command_validation_test.go|0"
  "cmd/zqk/daemon/daemon_coverage_elevation_cycle97_test.go|cmd/zqk/daemon/daemon_command_mock_test.go|0"
  "cmd/zqk/do/do_coverage_elevation_cycle136_test.go|cmd/zqk/do/do_resolution_command_test.go|0"
  "cmd/zqk/docman/docman_coverage_elevation_cycle130_test.go|cmd/zqk/docman/docman_verify_env_test.go|0"
  "cmd/zqk/explain/explain_coverage_elevation_cycle106_test.go|cmd/zqk/explain/explain_command_scheme_test.go|0"
  "cmd/zqk/feed/feed_coverage_elevation_cycle135_test.go|cmd/zqk/feed/feed_validation_ack_test.go|0"
  "cmd/zqk/graph/graph_coverage_elevation_cycle96_test.go|cmd/zqk/graph/graph_discover_end_test.go|0"
  "cmd/zqk/grep/grep_coverage_elevation_cycle129_test.go|cmd/zqk/grep/grep_parse_options_test.go|0"
  "cmd/zqk/inbox/inbox_coverage_elevation_cycle102_test.go|cmd/zqk/inbox/inbox_list_storage_test.go|0"
  "cmd/zqk/intake/intake_coverage_elevation_cycle111_test.go|cmd/zqk/intake/intake_interactive_clarification_test.go|0"
  "cmd/zqk/job/job_coverage_elevation_cycle105_test.go|cmd/zqk/job/job_command_trigger_test.go|0"
  "cmd/zqk/keystore/keystore_coverage_elevation_cycle104_test.go|cmd/zqk/keystore/keystore_command_create_test.go|0"
  "cmd/zqk/kindpack/kindpack_coverage_elevation_cycle101_test.go|cmd/zqk/kindpack/kindpack_kind_pack_test.go|0"
  "cmd/zqk/matrix/matrix_coverage_elevation_cycle98_test.go|cmd/zqk/matrix/matrix_resolve_dimensions_test.go|0"
  "cmd/zqk/mcp-simple/mcp_simple_coverage_elevation_cycle119_test.go|cmd/zqk/mcp-simple/mcp_simple_success_main_test.go|0"
  "cmd/zqk/mcp/mcp_coverage_elevation_cycle118_test.go|cmd/zqk/mcp/mcp_command_daemon_test.go|0"
  "cmd/zqk/mesh/mesh_coverage_elevation_cycle95_test.go|cmd/zqk/mesh/mesh_command_advertise_test.go|0"
  "cmd/zqk/new/new_coverage_elevation_cycle109_test.go|cmd/zqk/new/new_command_spec_test.go|0"
  "cmd/zqk/object/object_coverage_elevation_cycle11_test.go|cmd/zqk/object/object_lifecycle_commands_test.go|1"
  "cmd/zqk/object/object_coverage_elevation_cycle121_test.go|cmd/zqk/object/object_inprocess_commands_test.go|1"
  "cmd/zqk/observer/observer_coverage_elevation_cycle99_test.go|cmd/zqk/observer/observer_populate_extract_test.go|0"
  "cmd/zqk/organizational/organizational_coverage_elevation_cycle114_test.go|cmd/zqk/organizational/organizational_sync_impact_test.go|0"
  "cmd/zqk/pack/pack_coverage_elevation_cycle101_test.go|cmd/zqk/pack/pack_command_help_test.go|0"
  "cmd/zqk/precommit/precommit_coverage_elevation_cycle117_test.go|cmd/zqk/precommit/precommit_write_failure_test.go|0"
  "cmd/zqk/query/query_coverage_elevation_cycle100_test.go|cmd/zqk/query/query_ast_and_visualizer_test.go|0"
  "cmd/zqk/query/query_coverage_elevation_cycle138_test.go|cmd/zqk/query/query_repl_and_execution_test.go|0"
  "cmd/zqk/rollback/rollback_coverage_elevation_cycle103_test.go|cmd/zqk/rollback/rollback_command_paths_test.go|0"
  "cmd/zqk/scheduler/scheduler_coverage_elevation_cycle139_test.go|cmd/zqk/scheduler/scheduler_automation_validation_test.go|0"
  "cmd/zqk/semantic/semantic_coverage_elevation_cycle109_test.go|cmd/zqk/semantic/semantic_adversarial_boundaries_test.go|0"
  "cmd/zqk/state/state_coverage_elevation_cycle110_test.go|cmd/zqk/state/state_stream_mutations_test.go|0"
  "cmd/zqk/swarm/swarm_coverage_elevation_cycle93_test.go|cmd/zqk/swarm/swarm_status_init_test.go|0"
  "cmd/zqk/sync/sync_coverage_elevation_cycle112_test.go|cmd/zqk/sync/sync_status_flag_test.go|0"
  "cmd/zqk/system/system_coverage_elevation_cycle120_test.go|cmd/zqk/system/system_subcommands_and_dashboard_test.go|0"
  "cmd/zqk/system/system_coverage_elevation_cycle12_test.go|cmd/zqk/system/system_maintenance_ops_test.go|0"
  "cmd/zqk/system/system_coverage_elevation_cycle29_crit2_test.go|cmd/zqk/system/system_operational_dynamics_test.go|0"
  "cmd/zqk/system/system_coverage_elevation_cycle29_crit3_test.go|cmd/zqk/system/system_adversarial_boundaries_test.go|0"
  "cmd/zqk/system/system_coverage_elevation_cycle29_test.go|cmd/zqk/system/system_invariants_test.go|0"
  "cmd/zqk/test/test_coverage_elevation_cycle140_test.go|cmd/zqk/test/test_command_dashboard_test.go|0"
  "cmd/zqk/tray/tray_coverage_elevation_cycle115_test.go|cmd/zqk/tray/tray_sign_resolve_test.go|0"
  "cmd/zqk/ui/ui_coverage_elevation_cycle132_test.go|cmd/zqk/ui/ui_command_interactive_test.go|0"
  "cmd/zqk/utility/utility_coverage_elevation_cycle141_test.go|cmd/zqk/utility/utility_subcommands_test.go|0"
  "cmd/zqk/utility/utility_coverage_elevation_cycle94_test.go|cmd/zqk/utility/utility_operations_test.go|0"
  "cmd/zqk/validate/validate_coverage_elevation_cycle116_test.go|cmd/zqk/validate/validate_agent_verify_test.go|0"
  "cmd/zqk/vendor/vendor_coverage_elevation_cycle108_test.go|cmd/zqk/vendor/vendor_command_cursor_test.go|0"
  "cmd/zqk/workflow/workflow_coverage_elevation_cycle128_test.go|cmd/zqk/workflow/workflow_subcommands_and_execution_test.go|1"
  "pkg/accumulator/accumulator_coverage_elevation_cycle150_test.go|pkg/accumulator/accumulator_engine_lite_test.go|0"
  "pkg/acronyms/acronyms_coverage_elevation_cycle178_test.go|pkg/acronyms/acronyms_scenarios_store_test.go|0"
  "pkg/adapters/adapters_coverage_elevation_cycle247_test.go|pkg/adapters/adapters_interface_delivery_test.go|0"
  "pkg/agentclaim/agentclaim_coverage_elevation_cycle162_test.go|pkg/agentclaim/agentclaim_claim_gate_test.go|0"
  "pkg/agentdelivery/agentdelivery_coverage_elevation_cycle163_test.go|pkg/agentdelivery/agentdelivery_deliverer_http_test.go|0"
  "pkg/agentfeed/agentfeed_coverage_elevation_cycle146_test.go|pkg/agentfeed/agentfeed_peer_ack_test.go|0"
  "pkg/agentidle/agentidle_coverage_elevation_cycle161_test.go|pkg/agentidle/agentidle_file_store_test.go|0"
  "pkg/agentonboard/agentonboard_coverage_elevation_cycle164_test.go|pkg/agentonboard/agentonboard_write_packs_test.go|0"
  "pkg/agentpack/agentpack_coverage_elevation_cycle248_test.go|pkg/agentpack/agentpack_agent_pack_test.go|0"
  "pkg/agentprompt/agentprompt_coverage_elevation_cycle165_test.go|pkg/agentprompt/agentprompt_prompt_task_test.go|0"
  "pkg/agentrules/agentrules_coverage_elevation_cycle166_test.go|pkg/agentrules/agentrules_manifest_dir_test.go|0"
  "pkg/aliases/aliases_coverage_elevation_cycle191_test.go|pkg/aliases/aliases_resolve_static_test.go|0"
  "pkg/ambience/ambience_coverage_elevation_cycle249_test.go|pkg/ambience/ambience_signal_aggregator_test.go|0"
  "pkg/ambient/ambient_coverage_elevation_cycle32_test.go|pkg/ambient/ambient_consensus_arbiter_test.go|0"
  "pkg/appledouble/appledouble_coverage_elevation_cycle179_test.go|pkg/appledouble/appledouble_sidecar_dir_test.go|0"
  "pkg/audit/audit_coverage_elevation_cycle251_test.go|pkg/audit/audit_stream_policy_test.go|0"
  "pkg/authcred/authcred_coverage_elevation_cycle252_test.go|pkg/authcred/authcred_uncovered_account_test.go|0"
  "pkg/batchaf/batchaf_coverage_elevation_cycle180_test.go|pkg/batchaf/batchaf_batch_aggregator_test.go|0"
  "pkg/brand/brand_coverage_elevation_cycle153_test.go|pkg/brand/brand_executable_prefix_test.go|0"
  "pkg/bufferpool/bufferpool_coverage_elevation_cycle157_test.go|pkg/bufferpool/bufferpool_stream_read_test.go|0"
  "pkg/circuitbreaker/circuitbreaker_coverage_elevation_cycle149_test.go|pkg/circuitbreaker/circuitbreaker_limiter_concurrency_test.go|0"
  "pkg/cleanup/cleanup_coverage_elevation_cycle186_test.go|pkg/cleanup/cleanup_path_load_test.go|0"
  "pkg/closureevidence/closureevidence_coverage_elevation_cycle253_test.go|pkg/closureevidence/closureevidence_evidence_validate_test.go|0"
  "pkg/community/defect_remediation_cycle37_test.go|pkg/community/defect_remediation_invariants_test.go|1"
  "pkg/concurrency/concurrency_coverage_elevation_cycle134_test.go|pkg/concurrency/concurrency_timeout_mutex_test.go|0"
  "pkg/contextevents/contextevents_coverage_elevation_cycle254_test.go|pkg/contextevents/contextevents_append_record_test.go|0"
  "pkg/contractchange/contractchange_coverage_elevation_cycle255_test.go|pkg/contractchange/contractchange_pending_fingerprint_test.go|0"
  "pkg/convergerollup/convergerollup_coverage_elevation_cycle256_test.go|pkg/convergerollup/convergerollup_cvs_nest_test.go|0"
  "pkg/coordination/coordination_coverage_elevation_cycle151_test.go|pkg/coordination/coordination_coordinator_emit_test.go|0"
  "pkg/crypto/crypto_coverage_elevation_cycle185_test.go|pkg/crypto/crypto_stamp_verification_test.go|0"
  "pkg/datacellregistry/datacellregistry_coverage_elevation_cycle257_test.go|pkg/datacellregistry/datacellregistry_read_descriptor_test.go|0"
  "pkg/decisionpack/decisionpack_coverage_elevation_cycle258_test.go|pkg/decisionpack/decisionpack_decision_pack_test.go|0"
  "pkg/diagnostics/diagnostics_coverage_elevation_cycle259_test.go|pkg/diagnostics/diagnostics_capture_disambiguation_test.go|0"
  "pkg/diskusage/diskusage_coverage_elevation_cycle160_test.go|pkg/diskusage/diskusage_scan_directory_test.go|0"
  "pkg/docman/docman_coverage_elevation_cycle260_test.go|pkg/docman/docman_registry_register_test.go|0"
  "pkg/drifthotspots/drifthotspots_coverage_elevation_cycle261_test.go|pkg/drifthotspots/drifthotspots_ast_barrels_test.go|0"
  "pkg/economy/economy_coverage_elevation_cycle262_test.go|pkg/economy/economy_broker_economic_test.go|0"
  "pkg/envelope/envelope_coverage_elevation_cycle193_test.go|pkg/envelope/envelope_tde_parse_test.go|0"
  "pkg/errfmt/errfmt_coverage_elevation_cycle176_test.go|pkg/errfmt/errfmt_string_builder_test.go|0"
  "pkg/events/events_coverage_elevation_cycle148_test.go|pkg/events/events_router_inspector_test.go|0"
  "pkg/execwrap/execwrap_coverage_elevation_cycle187_test.go|pkg/execwrap/execwrap_group_output_test.go|0"
  "pkg/featureflags/featureflags_coverage_elevation_cycle188_test.go|pkg/featureflags/featureflags_feature_flags_test.go|0"
  "pkg/filter/filter_coverage_elevation_cycle171_test.go|pkg/filter/filter_parse_string_test.go|0"
  "pkg/functional/functional_coverage_elevation_cycle173_test.go|pkg/functional/functional_result_else_test.go|0"
  "pkg/goroutinelabels/goroutinelabels_coverage_elevation_cycle155_test.go|pkg/goroutinelabels/goroutinelabels_worker_pool_test.go|0"
  "pkg/handslapper/handslapper_coverage_elevation_cycle181_test.go|pkg/handslapper/handslapper_monitor_command_test.go|0"
  "pkg/hostload/hostload_coverage_elevation_cycle159_test.go|pkg/hostload/hostload_level_snapshot_test.go|0"
  "pkg/httpheaders/httpheaders_coverage_elevation_cycle189_test.go|pkg/httpheaders/httpheaders_operations_constants_test.go|0"
  "pkg/kernel/kernel_coverage_elevation_cycle147_test.go|pkg/kernel/kernel_adjacency_plane_test.go|0"
  "pkg/kindsynonyms/kindsynonyms_coverage_elevation_cycle192_test.go|pkg/kindsynonyms/kindsynonyms_kind_resolve_test.go|0"
  "pkg/librarypack/librarypack_coverage_elevation_cycle306_test.go|pkg/librarypack/librarypack_library_pack_test.go|0"
  "pkg/license/license_coverage_elevation_cycle307_test.go|pkg/license/license_boundaries_test.go|0"
  "pkg/lifecycle/lifecycle_coverage_elevation_cycle142_test.go|pkg/lifecycle/lifecycle_listener_criterion_test.go|0"
  "pkg/llm/llm_coverage_elevation_cycle308_test.go|pkg/llm/llm_client_config_test.go|0"
  "pkg/lockhealth/lockhealth_coverage_elevation_cycle158_test.go|pkg/lockhealth/lockhealth_sweep_closed_test.go|0"
  "pkg/logging/logging_coverage_elevation_cycle311_test.go|pkg/logging/logging_logger_context_test.go|0"
  "pkg/mcp/mcp_coverage_elevation_cycle312_test.go|pkg/mcp/mcp_deep_server_test.go|0"
  "pkg/mesh/mesh_coverage_elevation_cycle313_test.go|pkg/mesh/mesh_gossip_syncer_test.go|0"
  "pkg/metricpack/metricpack_coverage_elevation_cycle314_test.go|pkg/metricpack/metricpack_kind_verify_test.go|0"
  "pkg/metricsrecording/metricsrecording_coverage_elevation_cycle250_test.go|pkg/metricsrecording/metricsrecording_metrics_recording_test.go|0"
  "pkg/mutation/mutation_coverage_elevation_cycle168_test.go|pkg/mutation/mutation_executor_transaction_test.go|0"
  "pkg/nildecode/nildecode_coverage_elevation_cycle190_test.go|pkg/nildecode/nildecode_decode_nil_test.go|0"
  "pkg/ontology/ontology_coverage_elevation_cycle169_test.go|pkg/ontology/ontology_system_variants_test.go|0"
  "pkg/orchestration/orchestration_coverage_elevation_cycle145_test.go|pkg/orchestration/orchestration_failure_worktree_test.go|0"
  "pkg/paths/paths_coverage_elevation_cycle154_test.go|pkg/paths/paths_root_project_test.go|0"
  "pkg/pipeline/pipeline_coverage_elevation_cycle144_test.go|pkg/pipeline/pipeline_add_stage_test.go|0"
  "pkg/predicate/predicate_coverage_elevation_cycle170_test.go|pkg/predicate/predicate_compile_precondition_test.go|0"
  "pkg/quality/cef_cycle3_regression_test.go|pkg/quality/cef_regression_integrity_test.go|0"
  "pkg/relay/relay_coverage_elevation_cycle196_test.go|pkg/relay/relay_server_client_test.go|0"
  "pkg/releasegate/releasegate_coverage_elevation_cycle197_test.go|pkg/releasegate/releasegate_public_payload_test.go|0"
  "pkg/releasepack/releasepack_coverage_elevation_cycle198_test.go|pkg/releasepack/releasepack_kind_verify_test.go|0"
  "pkg/reports/reports_coverage_elevation_cycle199_test.go|pkg/reports/reports_options_generate_test.go|0"
  "pkg/reqharness/reqharness_coverage_elevation_cycle200_test.go|pkg/reqharness/reqharness_verify_registry_test.go|0"
  "pkg/resourcehygiene/resourcehygiene_coverage_elevation_cycle201_test.go|pkg/resourcehygiene/resourcehygiene_reap_orphaned_test.go|0"
  "pkg/rollback/rollback_coverage_elevation_cycle202_test.go|pkg/rollback/rollback_store_conditions_test.go|0"
  "pkg/rollup/rollup_coverage_elevation_cycle203_test.go|pkg/rollup/rollup_parent_status_test.go|0"
  "pkg/router/router_coverage_elevation_cycle204_test.go|pkg/router/router_model_tool_test.go|0"
  "pkg/runtime/runtime_coverage_elevation_cycle143_test.go|pkg/runtime/runtime_goroutine_manager_test.go|0"
  "pkg/safepath/safepath_coverage_elevation_cycle175_test.go|pkg/safepath/safepath_root_symlink_test.go|0"
  "pkg/scenario/scenario_coverage_elevation_cycle205_test.go|pkg/scenario/scenario_apply_bundle_test.go|0"
  "pkg/scheduler/clusterstatus/bus_coverage_elevation_cycle33_test.go|pkg/scheduler/clusterstatus/bus_emit_decide_test.go|0"
  "pkg/scheduler/hostservice/hostservice_coverage_elevation_cycle32_crit3_test.go|pkg/scheduler/hostservice/hostservice_adversarial_boundaries_test.go|0"
  "pkg/scheduler/hostservice/hostservice_coverage_elevation_cycle32_test.go|pkg/scheduler/hostservice/hostservice_operational_dynamics_test.go|0"
  "pkg/scheduler/scheduler_coverage_elevation_cycle126_test.go|pkg/scheduler/scheduler_engine_dispatch_test.go|0"
  "pkg/scheduler/scheduler_coverage_elevation_cycle31_crit2_test.go|pkg/scheduler/scheduler_operational_dynamics_test.go|0"
  "pkg/scheduler/scheduler_coverage_elevation_cycle31_crit3_test.go|pkg/scheduler/scheduler_adversarial_boundaries_test.go|0"
  "pkg/scheduler/scheduler_coverage_elevation_cycle31_test.go|pkg/scheduler/scheduler_invariants_test.go|0"
  "pkg/scheduler/transceiver/transceiver_coverage_elevation_cycle33_test.go|pkg/scheduler/transceiver/transceiver_router_async_test.go|0"
  "pkg/screencap/screencap_coverage_elevation_cycle182_test.go|pkg/screencap/screencap_terminal_automator_test.go|0"
  "pkg/security/security_coverage_elevation_cycle206_test.go|pkg/security/security_sandbox_release_test.go|0"
  "pkg/semantic/semantic_coverage_elevation_cycle207_test.go|pkg/semantic/semantic_ontology_reconciler_test.go|0"
  "pkg/service/service_coverage_elevation_cycle208_test.go|pkg/service/service_adapter_launchd_test.go|0"
  "pkg/shellcmd/shellcmd_coverage_elevation_cycle183_test.go|pkg/shellcmd/shellcmd_shell_needs_test.go|0"
  "pkg/shockwave/shockwave_coverage_elevation_cycle209_test.go|pkg/shockwave/shockwave_dispatcher_pipeline_test.go|0"
  "pkg/shovelready/shovelready_coverage_elevation_cycle210_test.go|pkg/shovelready/shovelready_dependency_graph_test.go|0"
  "pkg/skill/skill_coverage_elevation_cycle211_test.go|pkg/skill/skill_seal_verify_test.go|0"
  "pkg/specbuilder/specbuilder_coverage_elevation_cycle11_test.go|pkg/specbuilder/specbuilder_spec_builder_test.go|0"
  "pkg/specialization/specialization_coverage_elevation_cycle167_test.go|pkg/specialization/specialization_handler_heart_test.go|0"
  "pkg/specorigination/specorigination_coverage_elevation_cycle212_test.go|pkg/specorigination/specorigination_stage_spec_test.go|0"
  "pkg/stampmemo/stampmemo_coverage_elevation_cycle184_test.go|pkg/stampmemo/stampmemo_table_paths_test.go|0"
  "pkg/stewardbase/stewardbase_coverage_elevation_cycle213_test.go|pkg/stewardbase/stewardbase_enqueue_steward_test.go|0"
  "pkg/storage/filecas/filecas_coverage_elevation_cycle12_test.go|pkg/storage/filecas/filecas_metrics_darwin_test.go|0"
  "pkg/storage/storage_coverage_elevation_cycle122_test.go|pkg/storage/storage_extended_operations_test.go|0"
  "pkg/storage/storage_coverage_elevation_cycle12_test.go|pkg/storage/storage_concurrency_and_caching_test.go|0"
  "pkg/storage/storage_coverage_elevation_cycle30_crit2_test.go|pkg/storage/storage_operational_dynamics_test.go|0"
  "pkg/storage/storage_coverage_elevation_cycle30_crit3_test.go|pkg/storage/storage_adversarial_boundaries_test.go|0"
  "pkg/storage/storage_coverage_elevation_cycle30_test.go|pkg/storage/storage_invariants_test.go|0"
  "pkg/storagetesting/storagetesting_coverage_elevation_cycle214_test.go|pkg/storagetesting/storagetesting_mock_isolation_test.go|0"
  "pkg/strutil/strutil_coverage_elevation_cycle174_test.go|pkg/strutil/strutil_default_levenshtein_test.go|0"
  "pkg/supervision/supervision_coverage_elevation_cycle215_test.go|pkg/supervision/supervision_extract_diagnostic_test.go|0"
  "pkg/supply/supply_coverage_elevation_cycle216_test.go|pkg/supply/supply_verifier_verify_test.go|0"
  "pkg/swarm/swarm_coverage_elevation_cycle218_test.go|pkg/swarm/swarm_engine_tool_test.go|0"
  "pkg/swarminit/swarminit_coverage_elevation_cycle217_test.go|pkg/swarminit/swarminit_executors_schema_test.go|0"
  "pkg/system/system_coverage_elevation_cycle221_test.go|pkg/system/system_apply_diagnose_test.go|0"
  "pkg/systemcheck/systemcheck_coverage_elevation_cycle219_test.go|pkg/systemcheck/systemcheck_check_validation_test.go|0"
  "pkg/systemcheckwake/systemcheckwake_coverage_elevation_cycle220_test.go|pkg/systemcheckwake/systemcheckwake_notify_config_test.go|0"
  "pkg/systempeel/systempeel_coverage_elevation_cycle195_test.go|pkg/systempeel/systempeel_scan_files_test.go|0"
  "pkg/tde/tde_coverage_elevation_cycle222_test.go|pkg/tde/tde_evaluation_matrix_test.go|0"
  "pkg/tdval/tdval_coverage_elevation_cycle223_test.go|pkg/tdval/tdval_validate_envelope_test.go|0"
  "pkg/telemetry/telemetry_coverage_elevation_cycle224_test.go|pkg/telemetry/telemetry_collector_buffer_test.go|0"
  "pkg/testdiscovery/testdiscovery_coverage_elevation_cycle225_test.go|pkg/testdiscovery/testdiscovery_discoverer_engine_test.go|0"
  "pkg/testenvroot/testenvroot_coverage_elevation_cycle194_test.go|pkg/testenvroot/testenvroot_copy_write_test.go|0"
  "pkg/testing/testing_coverage_elevation_cycle226_test.go|pkg/testing/testing_environment_setup_test.go|0"
  "pkg/testkit/testkit_coverage_elevation_cycle227_test.go|pkg/testkit/testkit_pipeline_leak_test.go|0"
  "pkg/testpackageconcurrency/testpackageconcurrency_coverage_elevation_cycle228_test.go|pkg/testpackageconcurrency/testpackageconcurrency_read_map_test.go|0"
  "pkg/testrunner/testrunner_coverage_elevation_cycle229_test.go|pkg/testrunner/testrunner_runner_execute_test.go|0"
  "pkg/testservices/testservices_coverage_elevation_cycle230_test.go|pkg/testservices/testservices_service_docker_test.go|0"
  "pkg/tpm/tpm_coverage_elevation_cycle231_test.go|pkg/tpm/tpm_strategic_readiness_test.go|0"
  "pkg/tracing/tracing_coverage_elevation_cycle232_test.go|pkg/tracing/tracing_binder_test.go|0"
  "pkg/translation/translation_coverage_elevation_cycle233_test.go|pkg/translation/translation_translator_parser_test.go|0"
  "pkg/transport/transport_coverage_elevation_cycle234_test.go|pkg/transport/transport_http_handler_test.go|0"
  "pkg/traversal/traversal_coverage_elevation_cycle152_test.go|pkg/traversal/traversal_executor_parser_test.go|0"
  "pkg/tray/tray_coverage_elevation_cycle235_test.go|pkg/tray/tray_security_validate_test.go|0"
  "pkg/tui/tui_coverage_elevation_cycle133_test.go|pkg/tui/tui_views_refresh_test.go|0"
  "pkg/validation/validation_coverage_elevation_cycle127_test.go|pkg/validation/validation_cache_and_pipeline_test.go|1"
  "pkg/vds/vds_coverage_elevation_cycle236_test.go|pkg/vds/vds_predicates_chunk_test.go|0"
  "pkg/verification/matrix/matrix_coverage_elevation_cycle237_test.go|pkg/verification/matrix/matrix_engine_scorecard_test.go|0"
  "pkg/verification/verification_coverage_elevation_cycle237_test.go|pkg/verification/verification_routing_command_test.go|0"
  "pkg/vet/vet_coverage_elevation_cycle238_test.go|pkg/vet/vet_git_files_exemptions_test.go|0"
  "pkg/vocabularypack/pack_coverage_elevation_cycle239_test.go|pkg/vocabularypack/pack_kind_verify_test.go|0"
  "pkg/walutil/walutil_coverage_elevation_cycle156_test.go|pkg/walutil/walutil_json_line_test.go|0"
  "pkg/when/when_coverage_elevation_cycle172_test.go|pkg/when/when_branching_chain_test.go|0"
  "pkg/workflow/whatsnext/whatsnext_coverage_elevation_cycle147_test.go|pkg/workflow/whatsnext/whatsnext_measure_enrich_test.go|0"
  "pkg/workflow/workflow_coverage_elevation_cycle147_test.go|pkg/workflow/workflow_classify_formula_test.go|0"
  "pkg/workflowpack/pack_coverage_elevation_cycle240_test.go|pkg/workflowpack/pack_kind_verify_test.go|0"
  "pkg/workpack/workpack_coverage_elevation_cycle241_test.go|pkg/workpack/workpack_verify_spec_test.go|0"
  "pkg/wsobs/wsobs_coverage_elevation_cycle242_test.go|pkg/wsobs/wsobs_metric_heuristic_test.go|0"
  "pkg/zqkcli/zqkcli_coverage_elevation_cycle243_test.go|pkg/zqkcli/zqkcli_operations_list_test.go|0"
  "pkg/zqkdev/zqkdev_coverage_elevation_cycle244_test.go|pkg/zqkdev/zqkdev_generate_builders_test.go|0"
  "pkg/zqkenv/zqkenv_coverage_elevation_cycle245_test.go|pkg/zqkenv/zqkenv_daemon_agent_test.go|0"
  "pkg/zqksession/zqksession_coverage_elevation_cycle246_test.go|pkg/zqksession/zqksession_id_session_test.go|0"
  "pkg/zqktime/zqktime_coverage_elevation_cycle177_test.go|pkg/zqktime/zqktime_format_rfc3339_test.go|0"
)

COLLISIONS=0
CANDIDATES=0
EXCLUDED=0

echo "[1/3] Verifying mappings and scanning for collisions..."

# 1. Check duplicate destination targets among active candidates
DUPLICATE_TARGETS=$(for item in "${MAPPINGS[@]}"; do
  IFS="|" read -r SRC DST IS_EX <<< "$item"
  if [[ "$IS_EX" == "0" ]]; then
    echo "$DST"
  fi
done | sort | uniq -d)

if [[ -n "$DUPLICATE_TARGETS" ]]; then
  echo "ERROR: Duplicate rename targets detected:"
  echo "$DUPLICATE_TARGETS"
  ((COLLISIONS++))
fi

# 2. Check if target destination file already exists on disk
for item in "${MAPPINGS[@]}"; do
  IFS="|" read -r SRC DST IS_EX <<< "$item"
  if [[ "$IS_EX" == "1" ]]; then
    ((EXCLUDED++))
    continue
  fi
  ((CANDIDATES++))
  if [[ -e "$SRC" && -e "$DST" && "$SRC" != "$DST" ]]; then
    echo "ERROR: Target already exists on disk: $DST (from $SRC)"
    ((COLLISIONS++))
  fi
done

echo "Summary: ${#MAPPINGS[@]} total, $CANDIDATES candidates, $EXCLUDED excluded (Cycle 37), $COLLISIONS collisions."
if [[ $COLLISIONS -gt 0 ]]; then
  echo "ABORTING: $COLLISIONS collisions detected."
  exit 1
fi

if [[ "$MODE" == "check-collisions" ]]; then
  echo "Collisions check passed cleanly (0 collisions detected)."
  exit 0
fi

if [[ "$MODE" == "dry-run" ]]; then
  echo "[2/3] Simulating renames (dry-run):"
  for item in "${MAPPINGS[@]}"; do
    IFS="|" read -r SRC DST IS_EX <<< "$item"
    if [[ "$IS_EX" == "1" ]]; then
      echo "  [DEFERRED (Cycle 37)] $SRC -> $(basename "$DST")"
    else
      echo "  [RENAME]             $SRC -> $(basename "$DST")"
    fi
  done
  echo "[3/3] Dry-run complete. All candidates verified. To apply, execute with --apply"
  exit 0
fi

if [[ "$MODE" == "validate" ]]; then
  echo "[2/3] Performing in-place atomic validation simulation..."
  revert_renames() {
    echo "Reverting simulated renames..."
    for item in "${MAPPINGS[@]}"; do
      IFS="|" read -r SRC DST IS_EX <<< "$item"
      if [[ "$IS_EX" == "0" && -f "$DST" ]]; then
        git mv "$DST" "$SRC" 2>/dev/null || true
      fi
    done
    echo "Revert complete."
  }
  trap revert_renames EXIT INT TERM

  echo "Simulating renames via git mv..."
  for item in "${MAPPINGS[@]}"; do
    IFS="|" read -r SRC DST IS_EX <<< "$item"
    if [[ "$IS_EX" == "0" && -f "$SRC" ]]; then
      git mv "$SRC" "$DST"
    fi
  done

  echo "Validating test compilation (ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test -run=^$ ./cmd/... ./pkg/...)..."
  export ZQK_ALLOW_FOREGROUND_GO_TEST=1
  go test -run=^$ ./cmd/... ./pkg/...
  echo "Validation successful: all candidate packages compile cleanly!"
  exit 0
fi
if [[ "$MODE" == "apply" ]]; then
  echo "[2/3] Executing live git mv operations..."
  MOVED=0
  for item in "${MAPPINGS[@]}"; do
    IFS="|" read -r SRC DST IS_EX <<< "$item"
    if [[ "$IS_EX" == "1" ]]; then
      echo "  [SKIPPING IN-FLIGHT] $SRC"
      continue
    fi
    if [[ -f "$SRC" ]]; then
      git mv "$SRC" "$DST"
      ((MOVED++))
    fi
  done
  echo "[3/3] Successfully renamed $MOVED test files!"
  echo "Validating build..."
  export ZQK_ALLOW_FOREGROUND_GO_TEST=1
  go test -run=^$ ./cmd/... ./pkg/...
  echo "Done!"
fi
