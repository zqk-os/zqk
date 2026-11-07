#!/usr/bin/env python3
"""Self-test for cas_unpaired_delete_inbound_refs (no pytest required)."""

from __future__ import annotations

import os
import tempfile
import unittest

from cas_unpaired_delete_inbound_refs import (
    ids_surviving_in_tree,
    inbound_refs_for_deleted_ids,
)


class InboundRefScanTest(unittest.TestCase):
    def test_finds_criteria_ref_and_skips_own_id(self):
        with tempfile.TemporaryDirectory() as tmp:
            crit_dir = os.path.join(tmp, ".zqk", "process", "criteria")
            bli_dir = os.path.join(tmp, ".zqk", "process", "backlog_items")
            os.makedirs(crit_dir)
            os.makedirs(bli_dir)
            deleted = "CRIT-TEST-001"
            with open(os.path.join(bli_dir, "aa" * 32 + ".yaml"), "w", encoding="utf-8") as fh:
                fh.write(
                    "id: BLI-TEST-001\nkind: backlog_item\nstatus: complete\ncriteria_refs:\n  - CRIT-TEST-001\n"
                )
            # Own id file should not count as inbound.
            found = inbound_refs_for_deleted_ids(tmp, [deleted], skip_paths=set())
            self.assertIn(deleted, found)
            self.assertTrue(any("backlog" in p for p in found[deleted]))

    def test_scheduler_job_commit_message_is_not_a_ref(self):
        with tempfile.TemporaryDirectory() as tmp:
            job_dir = os.path.join(tmp, ".zqk", "process", "scheduler_jobs", "2026-08")
            os.makedirs(job_dir)
            with open(os.path.join(job_dir, "ff" * 32 + ".yaml"), "w", encoding="utf-8") as fh:
                fh.write(
                    "id: SCH-TEST-001\nkind: scheduler_job\n"
                    "description: drop unmanaged PRB-CAP-HEALTH leftover\n"
                    "command_args:\n  - drop unmanaged PRB-CAP-HEALTH leftover\n"
                )
            found = inbound_refs_for_deleted_ids(tmp, ["PRB-CAP-HEALTH"])
            self.assertEqual(found, {})

    def test_empty_when_unlinked(self):
        with tempfile.TemporaryDirectory() as tmp:
            bli_dir = os.path.join(tmp, ".zqk", "process", "backlog_items")
            os.makedirs(bli_dir)
            with open(os.path.join(bli_dir, "bb" * 32 + ".yaml"), "w", encoding="utf-8") as fh:
                fh.write(
                    "id: BLI-TEST-002\nkind: backlog_item\nstatus: complete\ncriteria_refs: []\n"
                )
            found = inbound_refs_for_deleted_ids(tmp, ["CRIT-GONE-001"])
            self.assertEqual(found, {})


class SurvivingIDScanTest(unittest.TestCase):
    def _write(self, path: str, body: str) -> None:
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(body)

    def test_dedupe_delete_reports_id_as_surviving(self):
        with tempfile.TemporaryDirectory() as tmp:
            bli_dir = os.path.join(tmp, ".zqk", "process", "backlog_items")
            stale = os.path.join(bli_dir, "cc" * 32 + ".yaml")
            fresh = os.path.join(bli_dir, "dd" * 32 + ".yaml")
            self._write(stale, "id: BLI-DUP-001\nkind: backlog_item\nstatus: planned\n")
            self._write(fresh, "id: BLI-DUP-001\nkind: backlog_item\nstatus: complete\n")
            rel_stale = os.path.relpath(stale, tmp)
            survivors = ids_surviving_in_tree(tmp, ["BLI-DUP-001"], skip_paths={rel_stale})
            self.assertEqual(survivors, {"BLI-DUP-001"})

    def test_real_removal_reports_no_survivor(self):
        with tempfile.TemporaryDirectory() as tmp:
            bli_dir = os.path.join(tmp, ".zqk", "process", "backlog_items")
            only = os.path.join(bli_dir, "ee" * 32 + ".yaml")
            self._write(only, "id: BLI-GONE-001\nkind: backlog_item\nstatus: complete\n")
            rel_only = os.path.relpath(only, tmp)
            survivors = ids_surviving_in_tree(tmp, ["BLI-GONE-001"], skip_paths={rel_only})
            self.assertEqual(survivors, set())

    def test_internal_specs_do_not_count_as_survivors(self):
        with tempfile.TemporaryDirectory() as tmp:
            internal = os.path.join(tmp, ".zqk", "specs", "objects", "x.yaml")
            self._write(internal, "id: BLI-GONE-002\nkind: backlog_item\n")
            survivors = ids_surviving_in_tree(tmp, ["BLI-GONE-002"], skip_paths=set())
            self.assertEqual(survivors, set())


if __name__ == "__main__":
    unittest.main()
