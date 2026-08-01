package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	goroutinelabels "github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/swarm"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

var swarmWorkerPool *goroutinelabels.Pool
var swarmWorkerPoolOnce sync.Once

func getSwarmWorkerPool() *goroutinelabels.Pool {
	swarmWorkerPoolOnce.Do(func() {
		swarmWorkerPool = goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "swarm_pool", "Bounded worker pool for swarm tasks to prevent DDOS", 10, 10)
	})
	return swarmWorkerPool
}

var TestSwarmWorkerHook func(context.Context)
var SwarmNewLLMClient = func(c *llm.Config) llm.Client {
	provider := os.Getenv(zqkenv.LLMProvider())
	if provider == "openai" {
		return llm.NewClient(c)
	}
	if provider == "gemini" {
		return llm.NewGeminiClient(c)
	}
	return llm.NewQwenClient(c)
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

	filter := storage.ListFilter{
		Kind: objects.KindAgentTask,
	}

	res, err := s.storage.List(ctx, s.secCtx, nil, filter)
	if err != nil {
		s.logger.Error("SWARM WORKER LIST ERROR", err)
		return
	}
	if res == nil {
		return
	}

	s.logger.Info(fmt.Sprintf("SWARM WORKER FOUND %d TASKS", len(res.Objects)))

	for _, task := range res.Objects {
		taskID, _ := task[objects.FieldKeyID].(string)
		status, _ := task[objects.FieldKeyStatus].(string)
		updatedAt, _ := task[objects.FieldKeyUpdatedAt].(string)

		if taskID == "ATK-REDACTED" {
			s.logger.Info(fmt.Sprintf("SWARM WORKER CHECKING TARGET TASK: id=%s status=%s", taskID, status))
		}

		if taskID == "" || status != objects.ObjectStatusProposed {
			continue
		}

		s.logger.Info("SWARM WORKER ATTEMPTING TO LEASE TASK: " + taskID)

		// Update state to in_progress with optimistic locking
		err := s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
			objects.FieldKeyStatus:               objects.ObjectStatusInProgress,
			storage.ConstStreamExpectedUpdatedAt: updatedAt,
		})
		if err != nil {
			if err == storage.ErrVersionConflict {
				s.logger.Info("SWARM WORKER LEASE CONFLICT (task already taken): " + taskID)
				continue
			}
			s.logger.Error("Failed to update task state", err)
			continue
		}

		s.logger.Info("SWARM WORKER SUCCESSFULLY LEASED TASK: " + taskID)

		// Spawn worker via bounded pool to prevent DDOS
		err = getSwarmWorkerPool().Submit(ctx, func(workerCtx context.Context) error {
			secCtx := pkgctx.NewSecurityContext("account:swarm_worker", []string{"swarm_worker"}, []string{"*"})
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
			client := s.resolveLLMClientForTask(workerCtx, task)
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

			engine := swarm.NewEngine(client, executor, 0, taskID)
			engine.OnStep = func(ctx context.Context, stepLog string) error {
				resetTimeout()
				return nil
			}

			respContent, err := engine.Run(workerCtx, systemPrompt, userPrompt)
			if err != nil {
				if strings.Contains(err.Error(), "prompt size exceeds context budget limit") {
					s.logger.Error("SWARM WORKER ABORTED: token budget exceeded. Triggering cleanup.", err)
					_ = s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
						objects.FieldKeyStatus: "failed",
					})
					return nil
				}
				failTask(fmt.Errorf("SWARM ENGINE RUN FAILED: %w", err))
				return nil
			}

			s.logger.Info("Swarm Worker Task completed", logging.String("task_id", taskID), logging.String("response", respContent))
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

func (s *Scheduler) resolveLLMClientForTask(ctx context.Context, task map[string]any) llm.Client {
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
		return SwarmNewLLMClient(nil)
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
		return SwarmNewLLMClient(nil)
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
	apiKey := os.Getenv(zqkenv.LLMAPIKey())
	if provider == "gemini" {
		if key := os.Getenv(zqkenv.GeminiAPIKey()); key != "" {
			apiKey = key
		}
	}

	cfg := &llm.Config{
		Provider:   provider,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		ChatModel:  modelID,
		EmbedModel: os.Getenv(zqkenv.LLMEmbedModel()),
	}

	if provider == "gemini" {
		return llm.NewGeminiClient(cfg)
	} else if provider == "qwen" {
		return llm.NewQwenClient(cfg)
	}
	return llm.NewClient(cfg)
}

// recoverOrphanedTasks queries the database for all agent_task objects that are in 'in_progress' or 'pending_verification' status and resets them to 'proposed' on daemon start.
func (s *Scheduler) recoverOrphanedTasks(ctx context.Context) {
	if s.storage == nil {
		return
	}

	filter := storage.ListFilter{
		Kind: objects.KindAgentTask,
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
		status, _ := task[objects.FieldKeyStatus].(string)

		if taskID == "" {
			continue
		}

		if status == objects.ObjectStatusInProgress || status == objects.ObjectStatusPendingVerification {
			s.logger.Info(fmt.Sprintf("RECOVER ORPHANED TASK: resetting task %s from status %s to proposed", taskID, status))
			err := s.storage.Update(ctx, s.secCtx, taskID, map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusProposed,
			})
			if err != nil {
				s.logger.Error(fmt.Sprintf("Failed to reset orphaned task %s", taskID), err)
			}
		}
	}
}
