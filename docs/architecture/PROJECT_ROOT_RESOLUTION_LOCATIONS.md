# Project Root Resolution: Locations Updated to ResolveProjectRoot

**Status: All listed locations have been updated** (systematic replace completed).

All call sites that resolve project root from the current working directory now use `ResolveProjectRoot(".")` instead of `FindProjectRoot(".")` so that **ZQK_PROJECT_ROOT** and **ZQK_TEST_ROOT** are respected and the process is always sure which project root is in use.

**Precedence** (see `internal/cli/context/context.go`):

1. `ZQK_PROJECT_ROOT` — explicit production root
2. `ZQK_TEST_ROOT` — explicit test root (test isolation)
3. `FindProjectRoot(".")` — CWD-based discovery

## Locations (by package)

### cmd/zqk/root.go

- Already updated in a prior change (metrics, brand init, PersistentPreRunE, audit event).

### cmd/zqk/scheduler

| File | Line(s) | Replacement |
|------|---------|-------------|
| scheduler_core.go | 32, 167, 249, 401 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| submit.go | 73, 111 | same |
| show_history_helpers.go | 37 | same |
| show_activity_helpers.go | 34, 397 | same |
| scheduler_list.go | 24 | same |
| scheduler_helpers.go | 37 | same |
| scheduler_health.go | 52 | same |
| scan_tests.go | 95 | same |
| test_failures_rerun.go | 28 | `clictx.FindProjectRoot(".")` → `clictx.ResolveProjectRoot(".")` |
| test_failures_list.go | 22 | same |
| test_failures_analyze.go | 41 | same |

### cmd/zqk/utility

| File | Line(s) | Replacement |
|------|---------|-------------|
| scenario_builder_helpers_handlers.go | 18 | `clicontext.FindProjectRoot(".")` → `clicontext.ResolveProjectRoot(".")` |
| scenario_builder_helpers_flags.go | 43 | same |

### cmd/zqk/system

| File | Line(s) | Replacement |
|------|---------|-------------|
| whoami_helpers.go | 26 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| update_specs_helpers.go | 51 | same |
| system_check_progress_e2e_test.go | 27 | `clicontext.FindProjectRoot(".")` → `clicontext.ResolveProjectRoot(".")` |
| sync_cas_index.go | 42 | same |
| status_helpers.go | 38 | `cli.` |
| status.go | 66 | same |
| spec_auto_fixer_types_init.go | 32 | `clicontext.` |
| snapshot_verify_helpers.go | 42 | `cli.` |
| snapshot_scenario_run_helpers.go | 76 | same |
| snapshot_expand.go | 74 | same |
| show_validation_progress_types_init.go | 87 | `clicontext.` |
| show_validation_progress_completion.go | 83, 463 | same |
| service.go | 383, 479 | `cli.` |
| reminders.go | 52 | same |
| recover_cas.go | 54 | `clicontext.` |
| quarantine_report.go | 122 | same |
| orphan_cleanup_fallback.go | 51 | same |
| migrate.go | 72, 183 | same |
| metrics.go | 79 | `cli.` |
| integrity_check_cas.go | 29 | `clicontext.` |
| health_data.go | 55 | same |
| generate_lifecycle_id_list.go | 57 | same |
| fix_hash_mismatches.go | 86 | same |
| fix_command_executor_sync_test.go | 23 | same |
| file_lock_metrics.go | 170 | `cli.` |
| feature_flags.go | 25, 140, 276 | `clicontext.` |
| clear_validation_cache_helpers.go | 16 | same |
| cleanup_quarantine_helpers.go | 25 | same |
| cleanup_events_helpers.go | 29 | `cli.` |
| cleanup_duplicates_helpers.go | 36 | `clicontext.` |
| check_references_helpers.go | 148, 236 | same |
| check_object_legacy_helpers.go | 35 | same |
| check_object_helpers.go | 267, 310 | same |
| check_kind_helpers.go | 42 | same |
| check_instance_validation_helpers.go | 58, 92 | same |
| check_impl_validators.go | 317 | same |
| check_impl_integrity.go | 26, 154 | same |
| check_impl_hash.go | 21 | same |
| check_impl_autofix.go | 120, 237, 336 | same |
| check_impl_audit_buffer.go | 71, 480, 542 | same |
| check_impl.go | 565, 612, 658 | same |
| check_hash.go | 241 | same |
| check_follower.go | 337, 352 | same |
| check_baseline.go | 40, 269 | same |
| check_async_baseline.go | 70, 658 | same |
| check_all_helpers.go | 41 | same |
| auto_fix_helpers_issue_processing.go | 64 | same |
| auto_fix_helpers_integrity.go | 45, 82 | same |
| auto_fix_helpers_hash_fixing.go | 17, 74 | same |
| auto_fix_helpers_cas.go | 47, 176 | same |
| auto_fix_helpers_batch.go | 21 | same |
| auto_fix_cache_invalidation.go | 26 | same |
| audit_report.go | 50 | `cli.` |
| async_check_non_blocking.go | 25, 38 | `clicontext.` |
| async_check_helpers.go | 47 | same |
| async_check.go | 120 | same |
| aggregate_change_journal.go | 59 | `cli.` |
| aggregate_audit_helpers.go | 42 | same |

### cmd/zqk/reports

| File | Line(s) | Replacement |
|------|---------|-------------|
| pcs.go | 71 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| edd.go | 71 | same |
| blockers.go | 70 | same |

### cmd/zqk/object

| File | Line(s) | Replacement |
|------|---------|-------------|
| pplan_prev.go | 36 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| pplan_next.go | 36 | same |
| pplan_current.go | 36 | same |

### pkg/zqkcli

| File | Line(s) | Replacement |
|------|---------|-------------|
| test_extensible_cli_examples_test.go | 36, 548 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| cli_example_generator.go | 60 | same |

### cmd/zqk/docman

| File | Line(s) | Replacement |
|------|---------|-------------|
| register.go | 68 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |

### cmd/zqk/callback

| File | Line(s) | Replacement |
|------|---------|-------------|
| callback.go | 91 | `clictx.FindProjectRoot(".")` → `clictx.ResolveProjectRoot(".")` |
| callback_test.go | 260 | Comment only: mention ResolveProjectRoot for accuracy |

### cmd/zqk/automation

| File | Line(s) | Replacement |
|------|---------|-------------|
| lint_bypass.go | 63 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |
| docman_sync.go | 76 | same |

### cmd/zqk/semantic

| File | Line(s) | Replacement |
|------|---------|-------------|
| assess.go | 58 | `cli.FindProjectRoot(".")` → `cli.ResolveProjectRoot(".")` |

## Import aliases

- `cli` → package `github.com/lanceman/zqk/internal/cli` (exports `ResolveProjectRoot`)
- `clicontext` → package `github.com/lanceman/zqk/internal/cli/context` (has `ResolveProjectRoot`)
- `clictx` → same as `cli` or `clicontext` depending on file; use same replacement with the same alias.

## Already updated (prior change)

- cmd/zqk/root.go
- internal/cli/processor.go
- pkg/storage/object_storage_file.go

## FindProjectRoot removed

The exported **FindProjectRoot** function was removed to avoid confusion. All resolution now goes through **ResolveProjectRoot** (same precedence: env vars then discovery). The discovery-from-path logic lives as unexported **findProjectRoot** in `internal/cli/context/context.go` and is only used by ResolveProjectRoot.
