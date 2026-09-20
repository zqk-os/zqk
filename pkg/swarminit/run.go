package swarminit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	runArtifactSchema = "zqk_swarm_init_run_v1"
	stageStatusPass   = "pass"
	stageStatusSkip   = "skip"
	stageStatusFail   = "fail"
	stageStatusGate   = "human_gate"
)

// ObjectGet loads a kernel object by id.
type ObjectGet func(ctx context.Context, id string) (map[string]any, error)

// SteerFunc appends a directed feed event (tests inject fakes).
type SteerFunc func(ctx context.Context, toAgentID, message string, awaitAck bool) (SteerOutcome, error)

// SteerOutcome is the subset of feed steer the runner records.
type SteerOutcome struct {
	EventID   string
	AwaitID   string
	Receipt   bool
	ToAgentID string
	Message   string
}

// ConversationProbe verifies a seat's conversation is live on that PID.
// Return an error containing "trajectory not found" to fail conversation_live.
type ConversationProbe func(ctx context.Context, seatID string, rec agentfeed.PeerSeatRecord) error

// SeatWorkerInstall installs/supervises seat-workers for the given flags.
type SeatWorkerInstall func(ctx context.Context, cfg SeatWorkerInstallConfig) error

// SeatWorkerInstallConfig is passed to the installer hook.
type SeatWorkerInstallConfig struct {
	ExecuteNonComms bool
	PollSeconds     int
	Seats           []string
}

// EnsureMCPFunc brings up the MCP TCP daemon.
type EnsureMCPFunc func(ctx context.Context, tcp string) error

// MCPSubscribersFunc returns the current MCP subscriber count.
type MCPSubscribersFunc func(ctx context.Context) (int, error)

// ChatBootstrapFunc pastes/sends a full chat-turn onboard (human opt-in).
type ChatBootstrapFunc func(ctx context.Context, seatID string, rec agentfeed.PeerSeatRecord, payloadPath string) error

// Env is the injectable runtime for executors.
type Env struct {
	ProjectRoot        string
	DryRun             bool
	AllowChat          bool
	FromStage          string
	PlanID             string
	CoordinatorAgentID string
	CoordinatorPersona string
	Now                func() time.Time
	GetObject          ObjectGet
	Steer              SteerFunc
	ProbeConversation  ConversationProbe
	InstallSeatWorkers SeatWorkerInstall
	EnsureMCP          EnsureMCPFunc
	MCPSubscribers     MCPSubscribersFunc
	ChatBootstrap      ChatBootstrapFunc
	LoadSeats          func(string) (agentfeed.PeerSeatsFile, error)
	SaveSeats          func(string, agentfeed.PeerSeatsFile) error
	InspectFeed        func(agentfeed.DoctorOptions) agentfeed.DoctorResult
}

func (e *Env) now() time.Time {
	if e != nil && e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

func (e *Env) loadSeats() (agentfeed.PeerSeatsFile, error) {
	if e != nil && e.LoadSeats != nil {
		return e.LoadSeats(e.ProjectRoot)
	}
	return agentfeed.LoadPeerSeats(e.ProjectRoot)
}

func (e *Env) saveSeats(f agentfeed.PeerSeatsFile) error {
	if e != nil && e.SaveSeats != nil {
		return e.SaveSeats(e.ProjectRoot, f)
	}
	return agentfeed.SavePeerSeats(e.ProjectRoot, f)
}

// Options for Run.
type Options struct {
	PipelineID string
	WorkflowID string
	Env        *Env
	Registry   *Registry
}

// StageRecord is one row in the run artifact / CLI payload.
type StageRecord struct {
	ID       string         `json:"id"`
	Executor string         `json:"executor"`
	Status   string         `json:"status"`
	SkipWhy  string         `json:"skip_why,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// Artifact is the persisted run record.
type Artifact struct {
	Schema     string        `json:"schema"`
	RunID      string        `json:"run_id"`
	PipelineID string        `json:"pipeline_id"`
	WorkflowID string        `json:"workflow_id,omitempty"`
	DryRun     bool          `json:"dry_run"`
	StartedAt  string        `json:"started_at"`
	FinishedAt string        `json:"finished_at"`
	OK         bool          `json:"ok"`
	HumanGate  string        `json:"human_gate,omitempty"`
	Path       string        `json:"path,omitempty"`
	Stages     []StageRecord `json:"stages"`
}

// Run loads the recipe and executes stages in order.
func Run(ctx context.Context, opts Options) (Artifact, error) {
	env := opts.Env
	if env == nil {
		env = &Env{}
	}
	if strings.TrimSpace(env.ProjectRoot) == "" {
		return Artifact{}, errfmt.Errorf("project root is required")
	}
	if env.GetObject == nil {
		return Artifact{}, errfmt.Errorf("object getter is required")
	}
	reg := opts.Registry
	if reg == nil {
		reg = DefaultRegistry()
	}

	pipelineID := strings.TrimSpace(opts.PipelineID)
	workflowID := strings.TrimSpace(opts.WorkflowID)
	if pipelineID == "" && workflowID == "" {
		return Artifact{}, errfmt.Errorf("require --pipeline or --workflow")
	}
	if pipelineID == "" {
		wf, err := env.GetObject(ctx, workflowID)
		if err != nil {
			return Artifact{}, errfmt.Newf("get workflow %s", workflowID).Wrap(err)
		}
		ref, err := PipelineRefFromWorkflow(wf)
		if err != nil {
			return Artifact{}, err
		}
		pipelineID = ref
	}

	obj, err := env.GetObject(ctx, pipelineID)
	if err != nil {
		return Artifact{}, errfmt.Newf("get pipeline %s", pipelineID).Wrap(err)
	}
	recipe, err := ParseRecipe(obj)
	if err != nil {
		return Artifact{}, err
	}

	started := env.now()
	art := Artifact{
		Schema:     runArtifactSchema,
		RunID:      "SWI-" + started.UTC().Format("20060102T150405Z"),
		PipelineID: recipe.ID,
		WorkflowID: workflowID,
		DryRun:     env.DryRun,
		StartedAt:  started.UTC().Format(time.RFC3339),
		OK:         true,
	}

	skipUntil := strings.TrimSpace(env.FromStage)
	inWindow := skipUntil == ""
	for _, st := range recipe.Stages {
		if !inWindow {
			if st.ID == skipUntil {
				inWindow = true
			} else {
				art.Stages = append(art.Stages, StageRecord{
					ID: st.ID, Executor: st.Executor, Status: stageStatusSkip, SkipWhy: "before --from" + "-stage",
				})
				continue
			}
		}
		fn, lookupErr := reg.Lookup(st.Executor)
		if lookupErr != nil {
			art.OK = false
			rec := StageRecord{ID: st.ID, Executor: st.Executor, Status: stageStatusFail, Error: lookupErr.Error()}
			art.Stages = append(art.Stages, rec)
			art.FinishedAt = env.now().UTC().Format(time.RFC3339)
			_ = writeArtifact(env.ProjectRoot, &art)
			return art, lookupErr
		}
		res, execErr := fn(ctx, env, st)
		rec := StageRecord{ID: st.ID, Executor: st.Executor, Evidence: res.Evidence}
		switch {
		case execErr != nil:
			rec.Status = stageStatusFail
			rec.Error = execErr.Error()
			art.Stages = append(art.Stages, rec)
			handled, runErr := applyOnFail(&art, st, rec)
			art.FinishedAt = env.now().UTC().Format(time.RFC3339)
			_ = writeArtifact(env.ProjectRoot, &art)
			if !handled {
				return art, execErr
			}
			if runErr != nil {
				return art, runErr
			}
			if art.HumanGate != "" {
				return art, nil
			}
		case res.Skipped:
			rec.Status = stageStatusSkip
			rec.SkipWhy = res.SkipWhy
			if res.Human != "" {
				rec.Status = stageStatusGate
				art.HumanGate = st.ID
			}
			art.Stages = append(art.Stages, rec)
			if rec.Status == stageStatusGate {
				art.OK = false
				art.FinishedAt = env.now().UTC().Format(time.RFC3339)
				_ = writeArtifact(env.ProjectRoot, &art)
				return art, nil
			}
		case !res.OK:
			rec.Status = stageStatusFail
			rec.Error = "pass predicate not met"
			art.Stages = append(art.Stages, rec)
			handled, runErr := applyOnFail(&art, st, rec)
			art.FinishedAt = env.now().UTC().Format(time.RFC3339)
			_ = writeArtifact(env.ProjectRoot, &art)
			if !handled {
				return art, errfmt.Errorf("stage %s failed", st.ID)
			}
			if runErr != nil {
				return art, runErr
			}
			if art.HumanGate != "" {
				return art, nil
			}
		default:
			rec.Status = stageStatusPass
			art.Stages = append(art.Stages, rec)
		}
	}

	art.FinishedAt = env.now().UTC().Format(time.RFC3339)
	if err := writeArtifact(env.ProjectRoot, &art); err != nil {
		return art, err
	}
	return art, nil
}

func applyOnFail(art *Artifact, st Stage, rec StageRecord) (handled bool, err error) {
	switch st.OnFail {
	case OnFailSkip:
		art.Stages[len(art.Stages)-1].Status = stageStatusSkip
		art.Stages[len(art.Stages)-1].SkipWhy = "on_fail=skip after " + rec.Status
		return true, nil
	case OnFailHumanGate:
		art.OK = false
		art.HumanGate = st.ID
		art.Stages[len(art.Stages)-1].Status = stageStatusGate
		return true, nil
	default:
		art.OK = false
		return false, errfmt.Errorf("stage %s failed: %s", st.ID, rec.Error)
	}
}

func writeArtifact(projectRoot string, art *Artifact) error {
	if art == nil || strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	day := ""
	if t, err := time.Parse(time.RFC3339, art.StartedAt); err == nil {
		day = t.UTC().Format("2006-01-02")
	} else {
		day = time.Now().UTC().Format("2006-01-02")
	}
	dir := filepath.Join(paths.SwarmInitDirPath(projectRoot), day)
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("mkdir swarm-init").Wrap(err)
	}
	path := filepath.Join(dir, art.RunID+".json")
	art.Path = path
	raw, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteStandardFile(path, append(raw, '\n'))
}

// Payload is the CLI FormatOutput map.
func Payload(art Artifact) map[string]any {
	out := map[string]any{
		objects.FieldKeyID:     art.RunID,
		"pipeline_id":          art.PipelineID,
		"ok":                   art.OK,
		"dry_run":              art.DryRun,
		objects.FieldKeyStatus: statusFromArtifact(art),
		objects.FieldKeyStages: art.Stages,
		objects.FieldKeyPath:   art.Path,
	}
	if art.WorkflowID != "" {
		out["workflow_id"] = art.WorkflowID
	}
	if art.HumanGate != "" {
		out["next_human_gate"] = art.HumanGate
	}
	return out
}

func statusFromArtifact(art Artifact) string {
	if art.HumanGate != "" {
		return stageStatusGate
	}
	if art.OK {
		return "ok"
	}
	return stageStatusFail
}
