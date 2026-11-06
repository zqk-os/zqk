# Architecture & Execution Specification: Cycle 47 - Final GitHub Issue Burndown & v0.3.2 Release Certification

**Document ID**: `DEC-CYCLE47-RELEASE-CERTIFICATION`
**Initiative**: Release Gate Readiness & Final GitHub Issue Burndown (GH-995, GH-996)
**Scope**: Darwin Installation Codesigning, Ambient Stop Operational Feedback, and v0.3.2 Multi-Architecture Release Certification

---

## 1. Executive Summary & Context

With statement test coverage elevated above 85% across core subsystems (Cycle 46), the repository has converged to zero open defects and only two remaining functional enhancements:
1. **GitHub Issue #995**: `enhancement(install): add ad-hoc codesign on darwin arm64 in scripts/install.sh to prevent SIGKILL (exit code 137)`
2. **GitHub Issue #996**: `enhancement(ambient): emit confirmation message on successful 'zqk ambient stop'`

Resolving these two items achieves **100% resolution of all reported repository issues** and prepares the repository for release cut `v0.3.2`.

---

## 2. Technical Design: GH-995 (Darwin arm64 Ad-Hoc Codesigning)

### 2.1 Problem Statement
When downloading or upgrading pre-built binaries on Apple Silicon macOS (macOS 14 Sonoma, macOS 15 Sequoia), Apple Mobile File Integrity (AMFI) and Gatekeeper enforce strict code signing checks. Even after clearing the quarantine attribute via `xattr -d com.apple.quarantine`, binaries modified in-place or downloaded without signature identity can trigger immediate termination with `SIGKILL` (signal 9, exit code 137).

### 2.2 Invariant & Architecture
Inside `scripts/install.sh`, the `_place_binary()` function must be augmented with an ad-hoc codesign pass using the system `codesign` tool:

```bash
# Remove macOS quarantine flag and apply ad-hoc signature on Apple Silicon / Darwin
if [ "$OS" = "darwin" ]; then
  if command -v xattr >/dev/null 2>&1; then
    if [ ! -w "$INSTALL_DIR" ] && [ "$NO_SUDO" != "1" ]; then
      sudo xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk" 2>/dev/null || true
      [ -f "${INSTALL_DIR}/zqk-mcp" ] && sudo xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk-mcp" 2>/dev/null || true
    else
      xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk" 2>/dev/null || true
      [ -f "${INSTALL_DIR}/zqk-mcp" ] && xattr -d com.apple.quarantine "${INSTALL_DIR}/zqk-mcp" 2>/dev/null || true
    fi
  fi

  if command -v codesign >/dev/null 2>&1; then
    if [ ! -w "$INSTALL_DIR" ] && [ "$NO_SUDO" != "1" ]; then
      sudo codesign -s - -f "${INSTALL_DIR}/zqk" 2>/dev/null || true
      [ -f "${INSTALL_DIR}/zqk-mcp" ] && sudo codesign -s - -f "${INSTALL_DIR}/zqk-mcp" 2>/dev/null || true
    else
      codesign -s - -f "${INSTALL_DIR}/zqk" 2>/dev/null || true
      [ -f "${INSTALL_DIR}/zqk-mcp" ] && codesign -s - -f "${INSTALL_DIR}/zqk-mcp" 2>/dev/null || true
    fi
  fi
fi
```

### 2.3 Boundary Invariants
- Fail-open fallback: If `codesign` is absent or encounters permission barriers in restricted environments, `|| true` prevents fatal script termination.

---

## 3. Technical Design: GH-996 (Ambient Daemon Stop Operational Feedback)

### 3.1 Problem Statement
When running `zqk ambient start`, the CLI writes directly to stdout:
```
Started ambient daemon (PID: 62863)
Logs: /path/to/.zqk/logs/ambient-daemon.log
```
Conversely, `zqk ambient stop` only recorded events via the structured logging facade (`logging.Fluent(logger).Info(...)`), producing zero stdout feedback to human operators and script harnesses.

### 3.2 Invariant & Architecture
In `cmd/zqk/ambient/daemon.go`:
1. Update `StopDaemon(projectRoot string, out io.Writer) error` (or provide `cmd.OutOrStdout()` pass-through from `newStopCmd`).
2. If running PIDs are detected and signaled:
   ```go
   for _, pid := range pids {
       if out != nil {
           _, _ = fmt.Fprintf(out, "Stopped ambient daemon (PID: %d)\n", pid)
       }
   }
   ```
3. If no running PID was found:
   ```go
   if len(pids) == 0 {
       if out != nil {
           _, _ = fmt.Fprintln(out, "Ambient daemon is not running")
       }
       return nil
   }
   ```
4. Verify regression test coverage in `cmd/zqk/ambient/ambient_subcommand_daemon_test.go`.

---

## 4. Release Certification Protocol (v0.3.2)

Upon completion of BLI-1 (GH-995) and BLI-2 (GH-996):
1. Execute `scripts/package-community.sh v0.3.2 --dry` to certify multi-architecture binaries:
   - `darwin/amd64`
   - `darwin/arm64`
   - `linux/amd64`
   - `linux/arm64`
2. Run `scripts/open-core/docs-portal/deploy-docs-portal.sh` to update `docs.zqk.dev`.
3. Open PR, squash-merge into `main`, and tag release `v0.3.2`.
