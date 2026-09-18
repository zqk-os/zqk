package swarminit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Metadata key on workflow objects that selects a swarm-init recipe.
const SwarmInitPipelineRefKey = "swarm_init_pipeline_ref"

// Stage on_fail values.
const (
	OnFailStop      = "stop"
	OnFailSkip      = "skip"
	OnFailHumanGate = "human_gate"
)

// Recipe stage object keys (not all are FieldKeys).
const (
	stageKeyExecutor = "executor"
	stageKeyOptional = "optional"
	stageKeyOnFail   = "on_fail"
	stageKeyConfig   = "config"
	stageKeyPass     = "pass"
)

// Registered executor ids (unknown id is fail-closed).
const (
	ExecutorControlPlane    = "control_plane"
	ExecutorBindSeats       = "bind_seats"
	ExecutorSeatWorkers     = "seat_workers"
	ExecutorCommsCheck      = "comms_check"
	ExecutorChatBootstrap   = "chat_bootstrap"
	ExecutorOrchestratePlan = "orchestrate_plan"
)

// Stage is one node in a swarm-init pipeline recipe.
type Stage struct {
	ID       string
	Executor string
	Optional bool
	OnFail   string
	Config   map[string]any
	Pass     map[string]any
}

// Recipe is the parsed pipeline object used by the runner.
type Recipe struct {
	ID     string
	Title  string
	Stages []Stage
}

// ParseRecipe extracts and validates stages from a pipeline object map.
func ParseRecipe(obj map[string]any) (Recipe, error) {
	if obj == nil {
		return Recipe{}, errfmt.Errorf("pipeline object is nil")
	}
	r := Recipe{
		ID:    strings.TrimSpace(objects.GetString(obj, objects.FieldKeyID)),
		Title: strings.TrimSpace(objects.GetString(obj, objects.FieldKeyTitle)),
	}
	if r.ID == "" {
		return Recipe{}, errfmt.Errorf("pipeline id is required")
	}
	raw, ok := obj[objects.FieldKeyStages]
	if !ok || raw == nil {
		return Recipe{}, errfmt.Errorf("pipeline %s: stages required", r.ID)
	}
	list, err := asList(raw)
	if err != nil {
		return Recipe{}, errfmt.Newf("pipeline %s stages", r.ID).Wrap(err)
	}
	if len(list) == 0 {
		return Recipe{}, errfmt.Errorf("pipeline %s: stages must not be empty", r.ID)
	}
	seen := map[string]struct{}{}
	for i, item := range list {
		st, err := parseStage(item, i)
		if err != nil {
			return Recipe{}, errfmt.Newf("pipeline %s stage[%d]", r.ID, i).Wrap(err)
		}
		if _, dup := seen[st.ID]; dup {
			return Recipe{}, errfmt.Errorf("pipeline %s: duplicate stage id %s", r.ID, st.ID)
		}
		seen[st.ID] = struct{}{}
		r.Stages = append(r.Stages, st)
	}
	return r, nil
}

func parseStage(item any, idx int) (Stage, error) {
	m, ok := asMap(item)
	if !ok {
		return Stage{}, errfmt.Errorf("must be an object")
	}
	st := Stage{
		ID:       strings.TrimSpace(objects.GetString(m, objects.FieldKeyID)),
		Executor: strings.TrimSpace(objects.GetString(m, stageKeyExecutor)),
		Optional: boolFromAny(m[stageKeyOptional]),
		OnFail:   strings.TrimSpace(objects.GetString(m, stageKeyOnFail)),
		Config:   mapFromAny(m[stageKeyConfig]),
		Pass:     mapFromAny(m[stageKeyPass]),
	}
	if st.ID == "" {
		st.ID = fmt.Sprintf("stage_%d", idx)
	}
	if st.Executor == "" {
		return Stage{}, errfmt.Errorf("executor is required")
	}
	if st.OnFail == "" {
		st.OnFail = OnFailStop
	}
	switch st.OnFail {
	case OnFailStop, OnFailSkip, OnFailHumanGate:
	default:
		return Stage{}, errfmt.Errorf("on_fail %q is not stop|skip|human_gate", st.OnFail)
	}
	return st, nil
}

// PipelineRefFromWorkflow reads metadata.swarm_init_pipeline_ref.
func PipelineRefFromWorkflow(obj map[string]any) (string, error) {
	if obj == nil {
		return "", errfmt.Errorf("workflow object is nil")
	}
	meta := mapFromAny(obj[objects.FieldKeyMetadata])
	ref := strings.TrimSpace(objects.GetString(meta, SwarmInitPipelineRefKey))
	if ref == "" {
		return "", errfmt.Errorf("workflow %s: metadata.%s is empty",
			objects.GetString(obj, objects.FieldKeyID), SwarmInitPipelineRefKey)
	}
	return ref, nil
}

func asList(v any) ([]any, error) {
	switch t := v.(type) {
	case []any:
		return t, nil
	case []map[string]any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = t[i]
		}
		return out, nil
	default:
		return nil, errfmt.Errorf("want list, got %T", v)
	}
}

func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	default:
		return nil, false
	}
}

func mapFromAny(v any) map[string]any {
	if m, ok := asMap(v); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

func stringFromPass(pass map[string]any, key string) string {
	return strings.TrimSpace(objects.GetString(pass, key))
}

func boolFromPass(pass map[string]any, key string, def bool) bool {
	if pass == nil {
		return def
	}
	if _, ok := pass[key]; !ok {
		return def
	}
	return boolFromAny(pass[key])
}

func intFromAny(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return def
		}
		return n
	default:
		return def
	}
}
