package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/agentclaim"
	"github.com/lanceman/zqk/pkg/agentfeed"
	"github.com/lanceman/zqk/pkg/authcred"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	goroutinelabels "github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/swarm"
)

// swarmSchedulerClaimant is used when an ATK has no assignee persona / peer seat.
// Occupancy must never be status-only (WFL-MULTI-AGENT-WORK-CLAIM).
const swarmSchedulerClaimant = "swarm-scheduler"

var swarmWorkerPool *goroutinelabels.Pool
var swarmWorkerPoolOnce sync.Once

func getSwarmWorkerPool() *goroutinelabels.Pool {
	swarmWorkerPoolOnce.Do(func() {
		swarmWorkerPool = goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "swarm_pool", "Bounded worker pool for swarm tasks to prevent DDOS", 10, 10)
	})
	return swarmWorkerPool
}

var TestSwarmWorkerHook func(context.Context)
var SwarmNewLLMClient = func(ctx context.Context, c *llm.Config) llm.Client {
	provider := config.LLMProvider().OrDefault("")
	if provider == "openai" {
		return llm.NewClient(ctx, c)
	}
	if provider == "gemini" {
		return llm.NewGeminiClient(ctx, c)
	}
	return llm.NewQwenClient(ctx, c)
}
var SwarmNewMCPExecutor = func(ctx context.Context, path string) (swarm.Executor, error) { return swarm.NewMCPExecutor(ctx, path) }

func (s *Scheduler) watchSwarmTasks(ctx context.Context) {
	getSwarmWorkerPool().Start(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial poll
	s.pollAndSpawnSwarmTasks(ctx)

	for {
		select {
		case <-ctx.Done():
			getSwarmWorkerPool().Stop()
			return
		case <-ticker.C:
			s.pollAndSpawnSwarmTasks(ctx)
		}
	}
}

func (s *Scheduler) pollAndSpawnSwarmTasks(ctx context.Context) {
	if s.storage == nil {
		return
	}

	// Push status into the list filter — do not scan every ATK then discard in Go.
	// TRACK: BLI-CEF-R27-DUAL-SEAT-REMEASURE-001
	filter := storage.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusApproved,
		},
		Fields: []string{
			objects.FieldKeyID,
			objects.FieldKeyStatus,
			objects.FieldKeyUpdatedAt,
			objects.FieldKeyAssigneePersonaRef,
			objects.FieldKeyTitle,
			objects.FieldKeyDescription,
		},
	}

	res, err := s.storage.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		s.logger.Error("SWARM WORKER LIST ERROR", err)
		return
	}
	if res == nil {
		return
	}

	s.logger.Info(fmt.Sprintf("SWARM WORKER FOUND %d APPROVED TASKS", len(res.Objects)))

	for _, task := range res.Objects {
		taskID, _ := task[objects.FieldKeyID].(string)

		s.logger.Info("SWARM WORKER ATTEMPTING TO LEASE TASK: " + taskID)

		claimant := resolveSwarmClaimant(s.getProjectRoot(), task)
		if claimant == "" {
			// Persona-assigned but no peer seat yet — leave approved for seat-worker claim.
			// TRACK: WFL-MULTI-AGENT-WORK-CLAIM
			s.logger.Info("SWARM WORKER SKIP LEASE (assignee persona has no peer seat): " + taskID)
			continue
		}
		// Lease via TryClaim so approved→in_progress always stamps claimed_by/claimed_at.
		// Raw status Update left in_progress unoccupied — unreliable mesh orchestration.
		claimRes, claimErr := agentclaim.TryClaim(ctx, s.storage, s.secCtx, taskID, claimant)
		if claimErr != nil {
			if claimRes.Reason == "already_claimed" {
				s.logger.Info("SWARM WORKER LEASE CONFLICT (task already claimed): " + taskID)
				continue
			}
			s.logger.Error("SWARM WORKER CLAIM/LEASE FAILED", claimErr)
			continue
		}

		s.logger.Info(fmt.Sprintf("SWARM WORKER SUCCESSFULLY LEASED TASK: %s claimed_by=%s", taskID, claimant))

		// Spawn worker via bounded pool to prevent DDOS
		err = getSwarmWorkerPool().Submit(ctx, func(workerCtx context.Context) error {
			secCtx := pkgctx.NewSecurityContext(authcred.DefaultSwarmWorkerAccount, []string{"swarm_worker"}, []string{"*"})
			workerCtx = pkgctx.WithSecurityContext(workerCtx, secCtx)
			workerCtx, cancel := context.WithCancel(workerCtx)
			defer cancel()

			activityCh := make(chan struct{}, 1)
			goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
				StartSimple(func() {
					func() {
						timer := time.NewTimer(30 * time.Minute) // 30m inactivity timeout
						defer timer.Stop()
						for {
							select {
							case <-workerCtx.Done():
								return
							case <-activityCh:
								if !timer.Stop() {
									select {
									case <-timer.C:
									default:
									}
								}
								timer.Reset(30 * time.Minute)
							case <-timer.C:
								s.logger.Error("SWARM WORKER TIMED OUT DUE TO INACTIVITY", nil)
								cancel() // timeout exceeded
								return
							}
						}
					}()
				})

			resetTimeout := func() {
				select {
				case activityCh <- struct{}{}:
				default:
				}
			}

			failTask := func(err error) {
				s.logger.Error("SWARM WORKER FAILED", err)
				_ = s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusError,
				})
			}

			if TestSwarmWorkerHook != nil {
				TestSwarmWorkerHook(workerCtx)
			}
			// Initiate swarm worker via pkg/llm
			s.logger.Info("SWARM WORKER STARTED FOR TASK: " + taskID)
			client, chatModel := s.resolveLLMClientForTask(workerCtx, task)
			if client == nil {
				failTask(fmt.Errorf("LLM CLIENT INIT FAILED"))
				return nil
			}

			executor, err := SwarmNewMCPExecutor(workerCtx, "./bin/zqk-mcp")
			if err != nil {
				failTask(fmt.Errorf("MCP EXECUTOR INIT FAILED: %w", err))
				return nil
			}
			defer executor.Close()

			taskTitle, _ := task[objects.FieldKeyTitle].(string)
			taskDesc, _ := task[objects.FieldKeyDescription].(string)

			taskInstr := ""
			if steps, ok := task[objects.FieldKeyTaskSteps].([]any); ok {
				if stepsBytes, err := json.MarshalIndent(steps, "", "  "); err == nil {
					taskInstr = string(stepsBytes)
				}
			} else if instrStr, ok := task[objects.FieldKeyInstruction].(string); ok {
				taskInstr = instrStr
			}

			systemPrompt, err := swarm.RenderSystemPrompt(swarm.QwenSystemData{
				WorkerID:     taskID,
				Capabilities: []string{"coding", "review"},
			})
			if err != nil {
				failTask(fmt.Errorf("FAILED TO RENDER SYSTEM PROMPT: %w", err))
				return nil
			}

			userPrompt, err := swarm.RenderTaskPrompt(swarm.QwenTaskData{
				TaskName:        taskTitle,
				TaskDescription: taskDesc,
				Context:         taskInstr,
			})
			if err != nil {
				failTask(fmt.Errorf("FAILED TO RENDER TASK PROMPT: %w", err))
				return nil
			}

			engine := swarm.NewEngine(client, executor, swarm.PreserveToolSchemas, taskID).
				WithRunIdentity("", taskID).
				RequireSuccessfulToolCalls(1)
			if swarm.ApplyCodeDraftHarness(engine, chatModel) {
				s.logger.Info("SWARM WORKER using code-draft harness for model " + chatModel)
			}
			engine.OnStep = func(ctx context.Context, stepLog string) error {
				resetTimeout()
				return nil
			}

			respContent, err := engine.Run(workerCtx, systemPrompt, userPrompt)
			if err != nil {
				if strings.Contains(err.Error(), "prompt size exceeds context budget limit") {
					s.logger.Error("SWARM WORKER ABORTED: token budget exceeded. Triggering cleanup.", err)
					_ = s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
						objects.FieldKeyStatus: objects.ObjectStatusFailed,
					})
					return nil
				}
				failTask(fmt.Errorf("SWARM ENGINE RUN FAILED: %w", err))
				return nil
			}

			s.logger.Info("Swarm Worker Task completed", logging.TaskIDField(taskID), logging.String("response", respContent))
			// Mark as implemented
			_ = s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusImplemented,
			})

			return nil
		})

		if err != nil {
			s.logger.Error("SWARM WORKER SUBMIT FAILED", err)
			_ = s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusFailed,
			})
		}
	}
}

func (s *Scheduler) resolveLLMClientForTask(ctx context.Context, task map[string]any) (llm.Client, string) {
	// 1. Get required model tier from task
	tier := "tier_2_simple"
	if tVal, ok := task[objects.FieldKeyModelTier].(string); ok && tVal != "" {
		tier = tVal
	}

	s.logger.Info(fmt.Sprintf("Resolving LLM client for task tier: %s", tier))

	// 2. Fetch all provider profiles
	filter := storage.ListFilter{
		Kind: "provider_profile",
	}
	res, err := s.storage.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		s.logger.Error("Failed to list provider profiles, falling back to environment LLM config", err)
		return SwarmNewLLMClient(ctx, nil), fallbackChatModel()
	}

	var matchedProfile map[string]any
	if res != nil {
		for _, profile := range res.Objects {
			pTier, _ := profile[objects.FieldKeyModelTier].(string)
			if pTier == tier {
				matchedProfile = profile
				break
			}
		}
	}

	if matchedProfile == nil {
		s.logger.Info(fmt.Sprintf("No provider profile found for tier %s, falling back to default LLM config", tier))
		return SwarmNewLLMClient(ctx, nil), fallbackChatModel()
	}

	// 3. Construct llm.Config from profile
	baseURL, _ := matchedProfile[objects.FieldKeyBaseURL].(string)
	modelID, _ := matchedProfile[objects.FieldKeyModelID].(string)
	endpointType, _ := matchedProfile[objects.FieldKeyEndpointType].(string)
	title, _ := matchedProfile[objects.FieldKeyTitle].(string)

	s.logger.Info(fmt.Sprintf("Found provider profile: %s (model: %s, endpoint: %s)", title, modelID, endpointType))

	provider := "openai"
	lowerEndpoint := strings.ToLower(endpointType)
	lowerModel := strings.ToLower(modelID)
	if strings.Contains(lowerEndpoint, "gemini") || strings.Contains(lowerModel, "gemini") {
		provider = "gemini"
	} else if strings.Contains(lowerEndpoint, "qwen") || strings.Contains(lowerModel, "qwen") {
		provider = "qwen"
	}

	// Resolve API key
	apiKey := zqkenv.LLMAPIKey().Get()
	if provider == "gemini" {
		if key := zqkenv.GeminiAPIKey().Get(); key != "" {
			apiKey = key
		}
	}

	if modelID == "" {
		modelID = fallbackChatModel()
	}

	cfg := &llm.Config{
		Provider:   provider,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		ChatModel:  modelID,
		EmbedModel: config.LLMEmbedModel().OrDefault(""),
	}
	applyCodeDraftSampling(cfg)

	if provider == "gemini" {
		return llm.NewGeminiClient(ctx, cfg), modelID
	} else if provider == "qwen" {
		return llm.NewQwenClient(ctx, cfg), modelID
	}
	return llm.NewClient(ctx, cfg), modelID
}

func fallbackChatModel() string {
	if m := zqkenv.Get(zqkenv.LLMChatModel().Name()).OrDefault(""); m != "" {
		return m
	}
	if strings.Contains(zqkenv.LLMBaseURL().Get(), "11434") {
		return "qwen3.8:latest"
	}
	return "gpt-4o-mini"
}

func applyCodeDraftSampling(cfg *llm.Config) {
	if cfg == nil || !llm.IsCodeDraftModel(cfg.ChatModel) || cfg.Temperature != nil {
		return
	}
	zero := 0.0
	cfg.Temperature = &zero
}

// recoverOrphanedTasks resets interrupted executors to the CAS-visible approved state.
// Pending-verification tasks are durable continuation records and must survive restarts.
func (s *Scheduler) recoverOrphanedTasks(ctx context.Context) {
	if s.storage == nil {
		return
	}

	filter := storage.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		},
	}

	res, err := s.storage.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		s.logger.Error("RECOVER ORPHANED TASKS LIST ERROR", err)
		return
	}
	if res == nil {
		return
	}

	for _, task := range res.Objects {
		taskID, _ := task[objects.FieldKeyID].(string)
		s.logger.Info(fmt.Sprintf("RECOVER ORPHANED TASK: resetting task %s from status %s to %s", taskID, objects.ObjectStatusInProgress, objects.ObjectStatusApproved))
		err := s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
			objects.FieldKeyStatus:    objects.ObjectStatusApproved,
			objects.FieldKeyClaimedBy: storage.FieldUnset,
			objects.FieldKeyClaimedAt: storage.FieldUnset,
		})
		if err != nil {
			s.logger.Error(fmt.Sprintf("Failed to reset orphaned task %s", taskID), err)
		}
	}
}

func orphanRecoveryStatus(status string) (string, bool) {
	if status == objects.ObjectStatusInProgress {
		return objects.ObjectStatusApproved, true
	}
	return "", false
}

// resolveSwarmClaimant picks the occupancy holder for a swarm lease.
// Persona-assigned ATKs bind to the peer seat that declared that persona_ref.
// Unassigned ATKs fall back to swarm-scheduler. Empty return = skip lease.
// TRACK: WFL-MULTI-AGENT-WORK-CLAIM
func resolveSwarmClaimant(projectRoot string, task map[string]any) string {
	persona, _ := task[objects.FieldKeyAssigneePersonaRef].(string)
	persona = strings.TrimSpace(persona)
	if persona != "" {
		if seat := agentfeed.WorkerSeatForPersona(projectRoot, persona); seat != "" {
			return seat
		}
		return ""
	}
	return swarmSchedulerClaimant
}
