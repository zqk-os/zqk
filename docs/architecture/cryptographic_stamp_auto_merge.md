# Cryptographic Agent Stamp Protocol

This architecture specification describes the sequence and paths for validating Cryptographic Agent Stamps in the ZQK kernel, including remote CI auto-merge and local offline fallback paths.

## Sequence Diagram

```mermaid
sequenceDiagram
    participant Agent as Agent (ZQK CLI)
    participant Kernel as ZQK Kernel (Local)
    participant CI as GitHub Actions CI
    participant Repo as Remote Repository (main)

    %% Stamp Generation
    Note over Agent: Completes task & generates JWT Stamp
    Agent->>Kernel: zqk commit --stamp <jwt>
    Kernel-->>Agent: Commit successfully created

    %% Remote CI Path
    Agent->>Repo: Push feature branch
    Repo->>CI: Trigger auto-merge workflow
    Note over CI: Remote validation
    CI->>CI: zqk agent validate --stamp <jwt>
    alt Stamp Verified & AST Compliance Pass
        CI->>Repo: Auto-merge to main
        Repo-->>CI: Merge Successful
    else Validation Failed
        CI->>Agent: Reject PR / Request Changes
    end

    %% Local Offline Fallback Path
    Note over Agent: Operating in offline / local mode
    Agent->>Kernel: zqk agent validate --stamp <jwt> --local-fallback
    Note over Kernel: Local fallback validation engine
    Kernel->>Kernel: 1. Verify cryptographic stamp using local authorized public key
    Kernel->>Kernel: 2. Execute AST compliance rules
    Kernel->>Kernel: 3. Check pre-commit policy status
    Kernel->>Kernel: 4. Run local objective validation via scheduler run_wrapper jobs
    alt Local Validation Passed
        Kernel-->>Agent: Validation complete and green
        Agent->>Kernel: Merge branch locally (Fast-forward)
    else Local Validation Failed
        Kernel-->>Agent: Reject local merge
    end
```

## Description
- **Remote CI Path:** Normal operation where GitHub Actions runs `zqk agent validate` to enforce pre-commit, objective, and stamp policies. If all pass, the PR is auto-merged.
- **Local Fallback Path:** When CI is unavailable, developers use the `--local-fallback` flag on `zqk agent validate` to use local authorized keys for stamp verification and to trigger local background testing via the scheduler. This provides an authoritative offline path to approve merges.
