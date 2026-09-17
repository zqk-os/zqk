# How-To: Process CAS Commits

Learn how ZQK's Git Content-Addressable Storage (CAS) membrane manages object versioning and staging.

---

## 1. What is CAS?
In ZQK, system objects are stored in `.zqk/process/<kind>/` named by the SHA-256 hash of their contents:
```text
.zqk/process/backlog/e3dd23b4bf4d60c1a9c784654e64ba93ac272694e2927154779972eef22c64ab.yaml
```
When an object is modified:
1. A new hash file is written.
2. The old hash file is removed.
3. The in-memory and durable id-to-hash indexes are updated.

---

## 2. Staging and Committing CAS Changes
When you use CLI commands like `zqk object create`, `zqk object update`, or `zqk object ref add`, git status will show renamed or untracked hash files:

```bash
# Stage the entire kind directory
git add .zqk/process/backlog/
git add .zqk/process/priority_plans/

# Verify git status shows clean renames
git status

# Commit with standard format
git commit -m "chore(kernel): update object references and status"
```

---

## 3. Resolving CAS Duplicate Blobs
If concurrent updates or aborted operations leave multiple hash blobs for a single ID:
```bash
# Detect and quarantine duplicate blobs
zqk system cleanup-duplicates --hash-duplicates

# Verify clean state
zqk system check
```
