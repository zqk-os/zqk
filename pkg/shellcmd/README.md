# Cross-Platform Shell Execution Engine (`pkg/shellcmd`)

`pkg/shellcmd` converts configured command strings into safe, cross-platform `argv` argument lists for process execution across POSIX (macOS, Linux) and Windows systems.

---

## 1. Motivation

Scheduler jobs, callback hooks, and router actions often contain shell one-liners with pipelines, output redirections, environment variables, and quoted arguments with whitespace:
```bash
tee -a "log.jsonl" >/dev/null && zqk feed steer -m "mission started"
```

If such a string is split naively on whitespace (`strings.Fields`), the shell operators (`>`, `&&`) and quoted words become literal arguments passed to `tee`, corrupting the command and littering files across the workspace.

`pkg/shellcmd` solves this by:
1. Detecting if the command string requires shell interpretation (`NeedsShell`).
2. If plain (no shell metacharacters), splitting on whitespace to bypass shell overhead.
3. If shell syntax is required, delegating to the operating system's native shell processor.
4. Failing closed (returning `nil`) on blank/whitespace-only input.

---

## 2. Windows Behavior & Considerations

When running on Windows (`runtime.GOOS == "windows"`):

1. **Shell Resolution (`ResolveShell`)**:
   - Checks the `%COMSPEC%` environment variable (typically `C:\Windows\System32\cmd.exe`).
   - If `%COMSPEC%` is unset or empty, falls back to `cmd.exe`.
   - Uses the termination switch `/c` (`cmd.exe /c "<command>"`).

2. **Windows Shell Metacharacters**:
   The `shellMetaChars` detector includes Windows-specific syntax:
   - `%` for environment variable expansion (`%USERPROFILE%`, `%PATH%`)
   - `^` for escape sequences in `cmd.exe`
   - In addition to standard POSIX characters (`|`, `&`, `;`, `<`, `>`, `(`, `)`, `$`, `` ` ``, `\`, `"`, `'`, `*`, `?`, `[`, `#`, `\n`)

3. **Important Limitations on Windows**:
   - **POSIX Syntax Incompatibility**: `cmd.exe` does not support POSIX idioms such as `export VAR=val`, `set -e`, `set -o pipefail`, single-quoted strings `'literal'`, or `/dev/null` (Windows uses `NUL`).
   - **POSIX Shim Execution**: Commands written for POSIX shells that are executed under `cmd.exe` will fail with syntax errors unless run inside a POSIX environment (such as Git Bash `bash.exe`, WSL, or MSYS2).
   - **Path Separators**: Backslash vs forward-slash handling in command arguments is respected as authored.
