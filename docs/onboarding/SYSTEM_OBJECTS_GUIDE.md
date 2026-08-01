# ZQK System Objects & Lifecycles Guide

Welcome to the ZQK Knowledge Kernel. This guide is the definitive Rosetta Stone for both human and AI agents operating within the workspace. It defines every system object, its purpose, its lifecycle, and when it should be used.

**Core Philosophy:** Everything is a spec-driven object. By adhering to these objects and their lifecycles, we ensure maximum observability, traceability, and collaboration.

## Table of Contents
- [account](#account)
- [agent_architecture](#agent_architecture)
- [agent_feed](#agent_feed)
- [agent_onboarding_preparation](#agent_onboarding_preparation)
- [agent_task](#agent_task)
- [audit_aggregation_metric](#audit_aggregation_metric)
- [audit_event](#audit_event)
- [auth_strategy](#auth_strategy)
- [auto_fix_rule](#auto_fix_rule)
- [backlog_item](#backlog_item)
- [base_metric](#base_metric)
- [base_sampler](#base_sampler)
- [brand](#brand)
- [bucketing_strategy](#bucketing_strategy)
- [certificate](#certificate)
- [change_journal_entry](#change_journal_entry)
- [code_quality_metric](#code_quality_metric)
- [code_reference](#code_reference)
- [command_metric](#command_metric)
- [command_spec](#command_spec)
- [component](#component)
- [compression_policy](#compression_policy)
- [context_refresh_schedule](#context_refresh_schedule)
- [convergence_session](#convergence_session)
- [corporate_initiative](#corporate_initiative)
- [criteria](#criteria)
- [decision](#decision)
- [department](#department)
- [display](#display)
- [division](#division)
- [doc_entry](#doc_entry)
- [domain_registry](#domain_registry)
- [evolution_management](#evolution_management)
- [extensible_object](#extensible_object)
- [field_registry](#field_registry)
- [file_lock_metric](#file_lock_metric)
- [glossary_term](#glossary_term)
- [glossary_term_relation](#glossary_term_relation)
- [goal](#goal)
- [impact_analysis](#impact_analysis)
- [import_tracking](#import_tracking)
- [important_date](#important_date)
- [integrity_manifest](#integrity_manifest)
- [keystore_entry](#keystore_entry)
- [kind_mapping_metric](#kind_mapping_metric)
- [kind_synonym](#kind_synonym)
- [library](#library)
- [lifecycle](#lifecycle)
- [list_metric_sampler](#list_metric_sampler)
- [mcp_session](#mcp_session)
- [metadata_package](#metadata_package)
- [metrics_exchange_contract](#metrics_exchange_contract)
- [metrics_feedback](#metrics_feedback)
- [milestone](#milestone)
- [mission](#mission)
- [namespace](#namespace)
- [namespace_registry](#namespace_registry)
- [object_spec](#object_spec)
- [ordered_list_metric_sampler](#ordered_list_metric_sampler)
- [organization](#organization)
- [organizational_change](#organizational_change)
- [partnership](#partnership)
- [persona](#persona)
- [pipeline](#pipeline)
- [policy](#policy)
- [priority_plan](#priority_plan)
- [process_hygiene_rule](#process_hygiene_rule)
- [prompt_template](#prompt_template)
- [question](#question)
- [release](#release)
- [requirement](#requirement)
- [resolver](#resolver)
- [risk_blocker](#risk_blocker)
- [roadmap](#roadmap)
- [role](#role)
- [rollback_report](#rollback_report)
- [rule](#rule)
- [sampler_profile](#sampler_profile)
- [scalar_metric_sampler](#scalar_metric_sampler)
- [scenario](#scenario)
- [scheduler_handler_binding](#scheduler_handler_binding)
- [scheduler_health_metric](#scheduler_health_metric)
- [scheduler_job](#scheduler_job)
- [stakeholder_profile](#stakeholder_profile)
- [status_history_metric_sampler](#status_history_metric_sampler)
- [strategic_context](#strategic_context)
- [strategic_plan](#strategic_plan)
- [team](#team)
- [technical_debt](#technical_debt)
- [template](#template)
- [test_audit_aggregation_metric](#test_audit_aggregation_metric)
- [test_case](#test_case)
- [test_command_rule](#test_command_rule)
- [verification_matrix](#verification_matrix)
- [vision](#vision)
- [vocabulary_scheme](#vocabulary_scheme)
- [workflow](#workflow)
- [workstream](#workstream)
- [workstream_transition](#workstream_transition)
- [zqk_session](#zqk_session)

## Object Definitions

### `account`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `display_name` (string)
- `email` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `profile_metadata` (object)
- `questions` (list)
- `related_object_refs` (list)
- `roles` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `tokens` (list)
- `updated_at` (datetime)
- `updated_by` (string)
- `username` (string)

---

### `agent_architecture`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `agent_type` (enum)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `components` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `integration_points` (list)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `role_ref` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `agent_feed`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `config_path_override` (string)
- `context` (text)
- `contract_schema_version` (string)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `delivery_mode` (enum)
- `dependencies` (list)
- `enabled` (bool)
- `estimated_effort` (string)
- `events_jsonl_path_override` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `note` (text)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `probe_command_substrings` (list)
- `probe_tool_allowlist` (list)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `agent_onboarding_preparation`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `agent_type` (enum)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `preparation_status` (enum)
- `preparation_tasks` (list)
- `priority_tier` (enum)
- `questions` (list)
- `readiness_criteria` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (date)
- `target_workstream_ref` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `agent_task`

**Purpose**: A discrete unit of work assigned to a specific persona. It serves as the primary routing envelope.

- **Visibility**: `internal`
- **Extends**: `base_object`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `artifacts` (list)
- `assignee_persona_ref` (string)
- `context` (text)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `inputs` (list)
- `kind` (enum)
- `namespace_id` (string)
- `outputs` (list)
- `pipeline_ref` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `validation_criteria_refs` (list)

---

### `audit_aggregation_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `aggregated_event_ids` (array)
- `aggregation_window_end` (string)
- `aggregation_window_start` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `compression_ratio` (number)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `error_event_count` (integer)
- `error_rate` (number)
- `estimated_effort` (string)
- `event_count` (integer)
- `event_type_counts` (object)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `kind` (enum)
- `last_seen` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `object_kind_counts` (object)
- `operation_counts` (object)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_counts` (object)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `audit_event`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `aggregated_count` (integer)
- `aggregated_events` (array)
- `aggregation_window` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `event_type` (enum)
- `id` (string)
- `kind` (enum)
- `metadata` (object)
- `namespace_id` (string)
- `new_value` (string)
- `occurrence_count` (integer)
- `occurrence_timestamps` (list)
- `operation` (string)
- `origin_project` (string)
- `origin_system` (string)
- `original_value` (string)
- `preserved_samples` (array)
- `priority_tier` (enum)
- `questions` (list)
- `reason` (text)
- `recovery_method` (enum)
- `related_object_refs` (list)
- `schema_version` (string)
- `severity` (enum)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `target_id` (string)
- `target_kind` (string)
- `target_path` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `auth_strategy`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `configuration` (object)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (integer)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `strategy_type` (string)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `auto_fix_rule`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `applies_to_kind` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `condition_category` (string)
- `condition_message_contains` (string)
- `condition_rule` (string)
- `condition_tier` (integer)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `enabled` (boolean)
- `estimated_effort` (string)
- `fix_command_template` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (integer)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `backlog_item`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `acceptance_criteria` (list)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `benefits` (list)
- `category` (string)
- `change_log` (list)
- `commit_refs` (list)
- `completed_at` (string)
- `components` (list)
- `considerations` (list)
- `context` (text)
- `convergence_session_profile` (string)
- `convergence_session_ref` (string)
- `created_at` (datetime)
- `created_by` (string)
- `date_captured` (date)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `document_refs` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `notes` (text)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (enum)
- `priority_plan_ref` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_features` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `base_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `kind` (enum)
- `last_seen` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `base_sampler`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (boolean)
- `id` (string)
- `kind` (enum)
- `max_batch_size` (integer)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `brand`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `brand_name` (string)
- `brand_variant` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `dna_fields` (array)
- `dna_source_ref` (string)
- `dna_version` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `metadata` (object)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `overrides` (object)
- `priority_tier` (enum)
- `propagation_mode` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (object)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `substitution_patterns` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `bucketing_strategy`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `applies_to` (array)
- `archive_strategy` (object)
- `archived_at` (datetime)
- `archived_by` (string)
- `change_log` (list)
- `created_at` (datetime)
- `created_by` (string)
- `enabled` (boolean)
- `field` (string)
- `format` (string)
- `origin_project` (string)
- `origin_system` (string)
- `retention_tolerance` (object)
- `strategy_name` (string)
- `strategy_type` (enum)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `certificate`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `credential_type` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `expires_at` (string)
- `holder_ref` (string)
- `id` (string)
- `issued_at` (string)
- `issuer_id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `payload` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `change_journal_entry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `change_type` (enum)
- `changed_paths` (array)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `diff_summary` (text)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `object_ref` (string)
- `origin_project` (string)
- `origin_system` (string)
- `previous_state` (object)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `code_quality_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `compliance_percentage` (number)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `issue_count` (integer)
- `issue_type` (string)
- `kind` (enum)
- `last_seen` (string)
- `measurement_period` (enum)
- `metric_category` (enum)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `policy_ref` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `resolution_count` (integer)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `tier` (enum)
- `title` (string)
- `trend_direction` (enum)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `code_reference`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `author` (string)
- `author_email` (string)
- `backlog_item_refs` (list)
- `change_log` (list)
- `change_type` (string)
- `commit_date` (string)
- `commit_hash` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `file_path` (string)
- `function_name` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `line_end` (number)
- `line_start` (number)
- `lines_added` (number)
- `lines_removed` (number)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `test_case_refs` (list)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `command_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `avg_duration_seconds` (number)
- `baseline_duration_seconds` (number)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `command` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `error_rate` (number)
- `estimated_effort` (string)
- `failure_count` (integer)
- `fastest_duration_seconds` (number)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `invocation_count` (integer)
- `kind` (enum)
- `last_seen` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `normalized_cmd` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `slowest_duration_seconds` (number)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `success_count` (integer)
- `tags` (array)
- `target_date` (string)
- `timeout_count` (integer)
- `timeout_rate` (number)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `command_spec`

**Purpose**: Defines the specification for a CLI command. Enables spec-driven command generation, automated help-text extraction, and tool discovery in the Sovereign Mesh.

- **Visibility**: `public`
- **Extends**: `base_object`

**Key Fields**:
- `id` (string): CSPEC- prefix followed by numeric identifier.
- `use` (string): The command usage string (e.g. "assess").
- `short` (string): Short description for the command.
- `long` (string): Detailed description including examples.
- `group_id` (string): Logical group for the command.
- `flags` (list): Defined flags and their types.
- `schema_version` (string)

---

### `component`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `child_component_refs` (list)
- `component_type` (string)
- `constraint_contexts` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `dashboard_relationships` (list)
- `deadline` (string)
- `dependencies` (list)
- `display_ref` (string)
- `domain` (string)
- `estimated_effort` (string)
- `gantt_relationships` (list)
- `id` (string)
- `kanban_relationships` (list)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `object_ref` (string)
- `origin_project` (string)
- `origin_system` (string)
- `parent_component_refs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `semantic_group_refs` (list)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `compression_policy`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `compress_field_keys` (boolean)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `target_kind` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `zlib_threshold_bytes` (int)

---

### `context_refresh_schedule`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `cadence` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `last_refresh` (string)
- `namespace_id` (string)
- `next_refresh` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target` (string)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `convergence_session`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `activity_log` (list)
- `actual_effort` (string)
- `after_state_snapshot` (object)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `automation_hooks` (string)
- `backlog_item_refs` (list)
- `before_state_snapshot` (object)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `current_phase` (enum)
- `deadline` (string)
- `debrief_notes` (text)
- `delta_assessment` (enum)
- `dependencies` (list)
- `desired_end_state` (text)
- `estimated_effort` (string)
- `flow_variant` (string)
- `glossary_term_ref` (string)
- `hypothesis` (text)
- `id` (string)
- `iteration_process` (text)
- `kind` (enum)
- `last_measurement_at` (string)
- `namespace_id` (string)
- `next_action` (text)
- `origin_project` (string)
- `origin_system` (string)
- `outcome_character` (enum)
- `predictions` (object)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `start_condition` (text)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `thresholds` (object)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `corporate_initiative`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `aggregated_db` (object)
- `aggregated_edd` (number)
- `aggregated_pcs` (number)
- `aggregation_status` (enum)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `cross_project_dependencies` (list)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `last_aggregated_at` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `owner_ref` (string)
- `priority_tier` (enum)
- `project_refs` (list)
- `questions` (list)
- `related_object_refs` (list)
- `resource_allocation` (object)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `criteria`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `category` (enum)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `validation_method` (enum)
- `validation_threshold` (string)

---

### `decision`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `decision_refs` (list)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `impact` (text)
- `impact_level` (enum)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `revisit` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `department`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `department_name` (string)
- `dependencies` (list)
- `division_ref` (reference)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kernel_goals_refs` (list)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `team_refs` (list)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `display`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `component_refs` (list)
- `constraint_contexts` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `display_type` (string)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `layout_config` (object)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `division`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `child_division_refs` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `division_name` (string)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kernel_goals_refs` (list)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `parent_division_ref` (reference)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `team_refs` (list)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `doc_entry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `category` (string)
- `change_log` (list)
- `content_searchable` (boolean)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `group` (enum)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `path` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `summary` (string)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `domain_registry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `domains` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `evolution_management`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `adaptive_adjustments` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `chaos_indicators` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `period` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `strategy` (enum)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `extensible_object`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `field_registry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `active_fields` (list)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `file_lock_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `avg_acquisition_time_ms` (number)
- `avg_wait_time_ms` (number)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `contention_rate` (number)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `kind` (enum)
- `last_seen` (string)
- `max_acquisition_time_ms` (number)
- `max_wait_time_ms` (number)
- `measurement_window_end` (string)
- `measurement_window_start` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `peak_contention` (integer)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `success_rate` (number)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `total_acquisitions` (integer)
- `total_contention` (integer)
- `total_failures` (integer)
- `total_timeouts` (integer)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `glossary_term`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `agent_prompts` (string)
- `alias_refs` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `category` (string)
- `change_log` (list)
- `context` (text)
- `context_scope` (string)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `definition` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `machine_hints` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `semantic_tags` (list)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `glossary_term_relation`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `notes` (string)
- `origin_project` (string)
- `origin_system` (string)
- `predicate_ref` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scheme_ref` (string)
- `sort_order` (integer)
- `source_term_ref` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `target_term_ref` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `goal`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `achieved_at` (string)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `authority` (string)
- `backlog_item_refs` (list)
- `change_log` (list)
- `commit_refs` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `current_value` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `metric` (string)
- `metric_template_id` (string)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target` (string)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `impact_analysis`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `affected_objects` (map)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `change_ref` (string)
- `change_type` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `impact_categories` (list)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `recommended_actions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `import_tracking`

**Purpose**: Tracks external ontology and schema imports (RDF/OWL, JSON Schema, etc.). Records source file metadata, hashes for drift detection, and translation status for traceability.

- **Visibility**: `internal`
- **Extends**: `base_object`

**Key Fields**:
- `id` (string): IMPTRK- prefix followed by numeric identifier.
- `source_file` (string): Path or identifier of the source file.
- `source_format` (string): Detected or specified format (e.g. turtle, rdf_owl).
- `source_hash` (string): SHA-256 hash of the source file for drift detection.
- `last_checked_at` (datetime): Timestamp of the last drift check.
- `status` (enum): Import status (ready, translated, failed).
- `imported_at` (datetime)
- `schema_version` (string)

---

### `important_date`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `date` (string)
- `date_type` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `impact_scope` (string)
- `importance` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholder_notifications` (list)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `integrity_manifest`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `entries` (list)
- `estimated_effort` (string)
- `hash_algorithm` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (list)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `keystore_entry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `account_id` (string)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `credential_hash` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `estimated_effort` (string)
- `expires_at` (string)
- `id` (string)
- `key_type` (string)
- `kind` (enum)
- `last_used_at` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `revoked` (boolean)
- `revoked_at` (string)
- `salt` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `kind_mapping_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backend_config_merges` (integer)
- `backend_switches` (integer)
- `batch_size` (integer)
- `cache_hits` (integer)
- `cache_misses` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `config_load_duration_ms` (number)
- `config_load_failures` (integer)
- `config_loads` (integer)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `directories_scanned` (integer)
- `directory_lookups` (integer)
- `discovery_errors` (integer)
- `estimated_effort` (string)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `inference_rule_hits` (integer)
- `initialization_duration_ms` (number)
- `initializations` (integer)
- `kind` (enum)
- `kind_lookups` (integer)
- `last_seen` (string)
- `lookup_errors` (integer)
- `mappings_discovered` (integer)
- `max_initialization_time_ms` (number)
- `measurement_window_end` (string)
- `measurement_window_start` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `specs_scanned` (integer)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `kind_synonym`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `convention` (string)
- `description` (string)
- `id` (string)
- `priority` (integer)
- `synonym` (string)
- `target_kind` (string)

---

### `library`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `composable_specs` (array)
- `connector_patterns` (array)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `dna_fields` (array)
- `dna_source_ref` (string)
- `dna_version` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `library_name` (string)
- `library_type` (string)
- `metadata` (object)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `overrides` (object)
- `priority_tier` (enum)
- `propagation_mode` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (object)
- `semantic_structure` (object)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `lifecycle`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `extends` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `object_type` (string)
- `origin_project` (string)
- `origin_system` (string)
- `percent_complete` (object)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `status_mapping` (object)
- `statuses` (list)
- `target_date` (string)
- `title` (string)
- `transitions` (list)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `list_metric_sampler`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (string)
- `id` (string)
- `kind` (enum)
- `max_batch_size` (string)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `mcp_session`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `account_id` (string)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `authentication_status` (string)
- `change_log` (list)
- `client_id` (string)
- `client_name` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `last_activity` (datetime)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `permissions` (array)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `roles` (array)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `metadata_package`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `collected_at` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `metrics` (object)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (enum)
- `scope_id` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `metrics_exchange_contract`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_mapping` (object)
- `format` (enum)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `operation_scope` (list)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `provider_scope` (list)
- `questions` (list)
- `related_object_refs` (list)
- `sample_rate` (number)
- `schema_version` (string)
- `sink_kind` (enum)
- `sink_target` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `metrics_feedback`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `analysis` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `metric_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `suggested_actions` (list)
- `target_date` (string)
- `target_report` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `milestone`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `blocked_by_refs` (list)
- `change_log` (list)
- `commit_refs` (list)
- `completed_at` (string)
- `completion_criteria` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `criteria_refs` (list)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `prerequisite_refs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stage_type` (enum)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `mission`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `mission_statement` (text)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `persona_refs` (list)
- `priority_tier` (enum)
- `problem_statement` (text)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `vision` (text)
- `workstream_refs` (list)

---

### `namespace`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `applicability` (object)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `integration` (object)
- `isolation` (object)
- `kind` (enum)
- `layer` (enum)
- `namespace_id` (string)
- `origin` (object)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `namespace_registry`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `namespaces` (list)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `object_spec`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `file_path` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `ontology` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `ordered_list_metric_sampler`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (string)
- `id` (string)
- `kind` (enum)
- `max_batch_size` (string)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `organization`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `division_refs` (list)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kernel_goals_refs` (list)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `organization_name` (string)
- `origin_project` (string)
- `origin_system` (string)
- `partnership_refs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `organizational_change`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `affected_objects` (map)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_date` (datetime)
- `change_description` (text)
- `change_log` (list)
- `change_type` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `impact_analysis_refs` (list)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `partnership`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `domain` (string)
- `end_date` (date)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `lifecycle_ref` (string)
- `namespace_id` (string)
- `organization_refs` (list)
- `origin_project` (string)
- `origin_system` (string)
- `partnership_name` (string)
- `partnership_type` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `start_date` (date)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `persona`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `mission_refs` (list)
- `name` (string)
- `namespace_id` (string)
- `needs` (list)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `role` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `pipeline`

**Purpose**: Defines a directed acyclic graph (DAG) of tasks that agents must perform to achieve a higher-level goal.

- **Visibility**: `internal`
- **Extends**: `base_object`

**Key Fields**:
- `actual_effort` (string)
- `agent_task_refs` (list)
- `answer_due_by` (string)
- `artifacts` (list)
- `context` (text)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stages` (list)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `trigger` (map)

---

### `policy`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `applicability` (object)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `body` (text)
- `category` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `effective_date` (string)
- `enforcement` (object)
- `estimated_effort` (string)
- `examples` (list)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `policy_type` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `related_patterns` (list)
- `review_date` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `version` (string)
- `workstream_refs` (list)

---

### `priority_plan`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `active_order` (integer)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `next_plan_id` (string)
- `note` (text)
- `origin_project` (string)
- `origin_system` (string)
- `plan_date` (string)
- `plan_version` (string)
- `previous_plan_id` (string)
- `priority_tier` (enum)
- `questions` (list)
- `rationale` (text)
- `related_object_refs` (list)
- `release_ref` (string)
- `schema_version` (string)
- `source_file` (string)
- `source_format` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workflow_ref` (string)
- `workstream_ref` (string)
- `workstream_refs` (list)

---

### `process_hygiene_rule`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `match_equals` (string)
- `match_field` (string)
- `match_prefix` (string)
- `match_regex` (string)
- `match_suffix` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `rule_id` (string)
- `schema_version` (string)
- `sort_order` (integer)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `prompt_template`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `category` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `doc_entry_refs` (list)
- `estimated_effort` (string)
- `expected_outcome_kinds` (list)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `mandatory_constraints` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `originator_questionnaire_refs` (list)
- `priority_tier` (enum)
- `prompt_archetype` (enum)
- `prompt_body` (text)
- `questions` (list)
- `related_object_refs` (list)
- `risk_blocker_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `specificity_level` (enum)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `question`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer` (text)
- `answer_due_by` (string)
- `answer_ref` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `blocking_goal_refs` (list)
- `blocking_milestone_refs` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `question_text` (text)
- `questions` (list)
- `related_object_refs` (list)
- `related_question_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `release`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `criteria_refs` (list)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `release_date` (string)
- `release_type` (enum)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `version` (string)

---

### `requirement`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `acceptance_criteria` (list)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `change_log` (list)
- `completed_at` (string)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `criteria_refs` (list)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `test_case_refs` (list)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `resolver`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `reference_format` (string)
- `related_object_refs` (list)
- `schema_version` (string)
- `scheme` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `user_hint` (text)

---

### `risk_blocker`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `affected_items` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `detected_by` (string)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `impact` (text)
- `kind` (enum)
- `milestone_refs` (list)
- `mitigation_plan` (text)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `probability` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `related_risks` (list)
- `resolution_status` (enum)
- `resolved_at` (string)
- `resolved_by` (string)
- `risk_score` (number)
- `risk_type` (enum)
- `schema_version` (string)
- `severity` (enum)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `roadmap`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `timeline_end` (string)
- `timeline_start` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `role`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `estimated_effort` (string)
- `id` (string)
- `influence_level` (enum)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `permissions` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `role_id` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `rollback_report`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `affected_objects` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `git_reference` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `summary` (text)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `rule`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `body` (text)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `scope` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `type` (enum)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `sampler_profile`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `applies_to` (list)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (boolean)
- `id` (string)
- `is_default` (boolean)
- `kind` (enum)
- `max_batch_size` (integer)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `profile_name` (string)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `scalar_metric_sampler`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (string)
- `id` (string)
- `kind` (enum)
- `max_batch_size` (string)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `scenario`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `category` (string)
- `change_log` (list)
- `change_policy` (string)
- `cleanup_config` (object)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `data_generation_config` (object)
- `deadline` (string)
- `dependencies` (list)
- `documentation_refs` (list)
- `edge_cases` (list)
- `environment_config` (object)
- `estimated_effort` (string)
- `fixtures` (list)
- `goal_refs` (list)
- `hash_mappings` (object)
- `id` (string)
- `infrastructure_config` (object)
- `kind` (enum)
- `last_run_info` (object)
- `namespace_id` (string)
- `objective` (text)
- `origin_project` (string)
- `origin_system` (string)
- `performance_targets` (object)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `scenario_refs` (list)
- `scheduler_config` (object)
- `schema_version` (string)
- `snapshot_hashes` (object)
- `snapshot_timestamp` (string)
- `source_object_ids` (list)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (list)
- `target_date` (string)
- `test_execution_config` (object)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `validation_rules` (object)
- `workstream_refs` (list)

---

### `scheduler_handler_binding`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `enabled` (boolean)
- `estimated_effort` (string)
- `handler_key` (enum)
- `id` (string)
- `job_type` (enum)
- `kind` (enum)
- `namespace_id` (string)
- `notes` (text)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (integer)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `scheduler_health_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `cron_restarts` (integer)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `field_name` (string)
- `first_seen` (string)
- `goroutine_count` (integer)
- `health_check_duration_ms` (number)
- `health_checks` (integer)
- `id` (string)
- `kind` (enum)
- `last_seen` (string)
- `measurement_window_end` (string)
- `measurement_window_start` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `missed_triggers` (integer)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `recovered_jobs` (integer)
- `related_object_refs` (list)
- `runtime_thread_count` (integer)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `scheduler_job`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `action_ref` (string)
- `actual_effort` (string)
- `allow_parallel_execution` (boolean)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `auth_config` (object)
- `auth_type` (enum)
- `callback_on_completion` (string)
- `callback_on_error` (string)
- `callback_on_status` (string)
- `callback_type` (enum)
- `category` (string)
- `change_log` (list)
- `command` (string)
- `command_args` (array)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `enabled` (boolean)
- `environment_variables` (object)
- `estimated_effort` (string)
- `execution_mode` (enum)
- `id` (string)
- `idle_shutdown_seconds` (integer)
- `job_type` (enum)
- `kind` (enum)
- `last_run_at` (string)
- `listener_path` (string)
- `listener_port` (integer)
- `log_level` (enum)
- `max_runtime_seconds` (integer)
- `namespace_id` (string)
- `next_run_at` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `retry_count` (integer)
- `retry_delay_seconds` (integer)
- `route_handlers` (object)
- `schedule_expression` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `transactional` (boolean)
- `trigger_type` (enum)
- `updated_at` (datetime)
- `updated_by` (string)
- `working_directory` (string)

---

### `stakeholder_profile`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `alignment_metrics` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `communication_preferences` (object)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `expectations` (list)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priorities` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholder_type` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `status_history_metric_sampler`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (string)
- `enabled` (boolean)
- `estimated_effort` (string)
- `field_name` (string)
- `flush_interval` (string)
- `group_by_object_id` (string)
- `id` (string)
- `kind` (enum)
- `max_batch_size` (string)
- `metric_type` (string)
- `namespace_id` (string)
- `object_kind` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `strategic_context`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `content` (string)
- `context` (text)
- `context_type` (string)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `important_dates` (list)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `policy_refs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholder_refs` (list)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `strategic_plan`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `phases` (list)
- `planning_horizon` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `team`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `department_ref` (reference)
- `dependencies` (list)
- `division_ref` (reference)
- `domain` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `lifecycle_ref` (string)
- `member_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `spec_context_broker` (string)
- `spec_interpreter` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `team_name` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `technical_debt`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_ref` (string)
- `change_log` (list)
- `complexity_score` (integer)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `debt_type` (enum)
- `dependencies` (list)
- `description` (string)
- `estimated_effort` (string)
- `file_path` (string)
- `function_name` (string)
- `id` (string)
- `impact_assessment` (enum)
- `kind` (enum)
- `linter_rule` (string)
- `mitigation_plan` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `policy_ref` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `resolution_notes` (string)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `target_resolution_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `template`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `category` (enum)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `flow_order` (number)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `outputs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `test_audit_aggregation_metric`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `aggregated_event_ids` (array)
- `aggregation_window_end` (string)
- `aggregation_window_start` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `batch_size` (integer)
- `change_log` (list)
- `collection_count` (integer)
- `compression_ratio` (number)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `error_event_count` (integer)
- `error_rate` (number)
- `estimated_effort` (string)
- `event_count` (integer)
- `event_type_counts` (object)
- `field_name` (string)
- `first_seen` (string)
- `id` (string)
- `kind` (enum)
- `last_seen` (string)
- `metric_type` (enum)
- `metric_type_specific` (string)
- `namespace_id` (string)
- `object_count` (integer)
- `object_kind` (string)
- `object_kind_counts` (object)
- `operation_counts` (object)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `sampled` (boolean)
- `schema_version` (string)
- `source` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_counts` (object)
- `status_history` (list)
- `tags` (array)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `window_end` (string)
- `window_start` (string)

---

### `test_case`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `backlog_item_refs` (list)
- `category` (string)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `criteria_refs` (list)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `milestone_refs` (list)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `path_or_id` (string)
- `priority` (enum)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `scope` (enum)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `test_command_rule`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `conditions` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `name` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority` (integer)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `verification_matrix`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `csv_path_override` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `gate_policy_notes` (text)
- `gated_object_kind` (string)
- `gated_transition_to` (string)
- `id` (string)
- `kind` (enum)
- `linked_goal_refs` (list)
- `linked_milestone_refs` (list)
- `linked_roadmap_refs` (list)
- `matrix_role` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `planning_notes` (text)
- `primary_convergence_session_ref` (string)
- `priority_tier` (enum)
- `profile_path_override` (string)
- `questions` (list)
- `registry_alias` (string)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `vision`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `goal_refs` (list)
- `id` (string)
- `kind` (enum)
- `mission_refs` (list)
- `namespace_id` (string)
- `narrative` (text)
- `origin_project` (string)
- `origin_system` (string)
- `pillars` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workstream_refs` (list)

---

### `vocabulary_scheme`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `context_scope` (string)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `machine_hints` (string)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `purpose` (string)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `summary` (string)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `workflow`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `applicable_accounts` (list)
- `applicable_roles` (list)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `blocking_check_config_ref` (string)
- `category` (enum)
- `change_log` (list)
- `constraints` (object)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `enabled` (boolean)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `lifecycle_refs` (list)
- `mcp_config` (object)
- `metadata` (object)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `policy_refs` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stages` (list)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `template_refs` (list)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `workstream`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `answer_due_by` (string)
- `application` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `blockers` (list)
- `category` (enum)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `description` (text)
- `entry_point` (string)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `metadata` (object)
- `milestone_refs` (list)
- `namespace_id` (string)
- `order` (integer)
- `origin_project` (string)
- `origin_system` (string)
- `owner_display` (string)
- `owner_ref` (reference)
- `prerequisites` (list)
- `priority_tier` (enum)
- `questions` (list)
- `related_docs` (list)
- `related_object_refs` (list)
- `requirement_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stage_type` (string)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)
- `workflow_ref` (string)
- `workstream_refs` (list)

---

### `workstream_transition`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `actual_effort` (string)
- `agent_onboarding` (list)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `from_workstream_ref` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `readiness_criteria` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `to_workstream_ref` (string)
- `transition_date` (date)
- `trigger` (enum)
- `trigger_milestone_ref` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

### `zqk_session`

**Purpose**: No description provided.

- **Visibility**: `internal`
- **Extends**: `N/A`

**Key Fields**:
- `account_id` (string)
- `actual_effort` (string)
- `answer_due_by` (string)
- `archived_at` (datetime)
- `archived_by` (string)
- `artifacts` (list)
- `change_log` (list)
- `context` (text)
- `created_at` (datetime)
- `created_by` (string)
- `deadline` (string)
- `dependencies` (list)
- `estimated_effort` (string)
- `id` (string)
- `kind` (enum)
- `namespace_id` (string)
- `origin_project` (string)
- `origin_system` (string)
- `priority_tier` (enum)
- `questions` (list)
- `related_object_refs` (list)
- `schema_version` (string)
- `source_type` (enum)
- `spec_adherence` (object)
- `stakeholders` (list)
- `status` (enum)
- `status_history` (list)
- `target_date` (string)
- `title` (string)
- `updated_at` (datetime)
- `updated_by` (string)

---

