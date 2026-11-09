# Scenario Convergence Matrix Engine & Multi-Model Evaluation Benchmark

## Status
- **Author**: PER-COMMUNITY-SYSTEM-ARCHITECT
- **Cycle**: Cycle 66
- **Status**: Approved / Implemented

---

## Architectural Context & Problem Statement
Prior to Cycle 66, scenario evaluations (`zqk eval run`) supported single-template evaluations against targeted file glob patterns or Go packages. In multi-agent and swarm coordination topologies, verifying convergence across multiple distinct model providers (e.g., Claude 3.5 Sonnet, GPT-4o, Gemini 1.5 Pro) required executing disconnected commands and manual variance comparison.

Furthermore, convergence sessions (`CVS-*`) require structured, aggregate metrics across multiple dimensions (`HCODE`, determinism, syntactic hygiene, negative boundary enforcement) to detect model drift or regressions before promoting code.

---

## Design & Implementation

### 1. Matrix Configuration & Aggregation Architecture
The `MatrixEngine` provides a high-throughput, deterministic evaluation harness that orchestrates:
- Multiple evaluation templates (`EvalTemplate`) across targeted codebases.
- Cross-model scoring variance and mean score aggregation.
- Pass/fail rate calculation per model provider.
- Automatic aggregation of remediation recommendations.

```go
type MatrixConfig struct {
    Templates []*EvalTemplate `json:"templates" yaml:"templates"`
    Models    []string        `json:"models" yaml:"models"`
    Targets   []string        `json:"targets" yaml:"targets"`
    SessionID string          `json:"session_id,omitempty" yaml:"session_id,omitempty"`
}

type ModelScoreSummary struct {
    Model         string  `json:"model"`
    RunsCount     int     `json:"runs_count"`
    PassedCount   int     `json:"passed_count"`
    PassRate      float64 `json:"pass_rate"`
    MeanScore     float64 `json:"mean_score"`
    ScoreVariance float64 `json:"score_variance"`
}
```

### 2. Variance Computation
Score variance is computed over $n$ evaluation runs per model:
$$s^2 = \frac{1}{n-1} \sum_{i=1}^n (x_i - \bar{x})^2 \quad (n > 1)$$

### 3. CLI Integration
The `zqk eval matrix` command binds directly to `MatrixEngine`:
```bash
zqk eval matrix --session-id CVS-001 --template ast-hygiene --json
```

---

## Verification & Conformance
- Test Suite: `pkg/eval/scenario_matrix_engine_test.go`
- Coverage: Multi-model variance aggregation, remediation recommendation synthesis, negative empty model/template boundary verification.
