# RB-CAS-001: Content-Addressed Storage (CAS) Hash Mismatch and Quarantine Exhaustion

## Metadata
- **Severity**: P0 / Critical
- **Component**: `pkg/storage`, `pkg/kernelcas`
- **Owner**: Kernel Storage Stewards

## Symptoms & Alerts
- Error log: `CAS hash mismatch for <OBJECT_ID> (kind: <KIND>): hash mismatch: expected <SHA1>, got <SHA2>`
- Error code: `ZQK_STORAGE_INTEGRITY_ERR`
- CLI fails with: `failed to read object: CAS hash mismatch`
- Quarantine directory `.zqk/state/datacells/<kind>/quarantine/` filling up or exceeding capacity alarms.

## Root Cause Analysis
1. In-place modification of files inside `.zqk/process/<kind>/` or `.zqk/state/datacells/<kind>/cas/` bypassing kernel serialization.
2. Interrupted atomic rename (`os.Rename`) or file truncation caused by power loss or OS crash.
3. Git merge conflict markers or CRLF/LF line ending mutations inserted into stored YAML artifacts.

## Step-by-Step Remediation Procedure

### Step 1: Identify Impacted Object and File
Inspect the error output to extract the object kind, ID, and path:
```bash
./bin/zqk object get <OBJECT_ID>
```
If this returns a CAS hash mismatch, identify the actual file hash:
```bash
shasum -a 256 .zqk/process/<kind>/<hash>.yaml
```

### Step 2: Quarantine Corrupted Content
Move corrupted objects out of active index into quarantine:
```bash
mkdir -p .zqk/quarantine/$(date +%Y%m%d)
mv .zqk/process/<kind>/<mismatched_hash>.yaml .zqk/quarantine/$(date +%Y%m%d)/
```

### Step 3: Reconstruct Object or Reindex
If the file content is valid YAML but the filename or index has drifted:
1. Recompute SHA-256 of the content:
   ```bash
   ACTUAL_SHA=$(shasum -a 256 <file>.yaml | awk '{print $1}')
   mv <file>.yaml .zqk/process/<kind>/${ACTUAL_SHA}.yaml
   ```
2. Update the `.index` file under `.zqk/process/<kind>/.<kind>.index` with the updated mapping `<OBJECT_ID>\t${ACTUAL_SHA}`.
3. Validate kernel integrity:
   ```bash
   ./bin/zqk system check --refresh
   ```

### Step 4: Verification Gate
Verify the object can be fetched cleanly:
```bash
./bin/zqk object get <OBJECT_ID>
./bin/zqk test dashboard --check-dod --refresh
```
Ensure zero CAS errors remain.
