package cli_builders

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// OsmosisEvent represents intercepted output from a CLI tool execution.
type OsmosisEvent struct {
	Command string
	Output  string
}

// KnowledgeAggregator is a daemon that asynchronously compiles fragmented CLI outputs
// into formalized capabilities.
type KnowledgeAggregator struct {
	events chan OsmosisEvent
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

var (
	globalAggregator *KnowledgeAggregator
	aggregatorOnce   sync.Once
)

// GetKnowledgeAggregator returns the singleton daemon.
func GetKnowledgeAggregator() *KnowledgeAggregator {
	aggregatorOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		globalAggregator = &KnowledgeAggregator{
			events: make(chan OsmosisEvent, 100),
			ctx:    ctx,
			cancel: cancel,
		}
		globalAggregator.start()
	})
	return globalAggregator
}

func (ka *KnowledgeAggregator) start() {
	ka.wg.Add(1)
	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				defer ka.wg.Done()
				for {
					select {
					case <-ka.ctx.Done():
						return
					case event := <-ka.events:
						ka.processEvent(event)
					}
				}
			}()
		})
}

// Stop shuts down the aggregator gracefully.
func (ka *KnowledgeAggregator) Stop() {
	ka.cancel()
	ka.wg.Wait()
}

func (ka *KnowledgeAggregator) processEvent(event OsmosisEvent) {
	logger := logging.GetLogger()

	// Simplistic parsing for the demo: look for flags in help output
	if strings.Contains(strings.ToLower(event.Output), "help") || strings.Contains(event.Output, "--") {
		flags := extractFlags(event.Output)
		if len(flags) > 0 {
			logging.FluentEvent(logger).
				Info(fmt.Sprintf("🧠 [OSMOSIS] Synthesized capability map for '%s'", event.Command)).
				Int("discovered_flags", len(flags)).
				String("flags", strings.Join(flags, ", ")).
				Log()
		}
	}
}

// extractFlags uses a regex to find command line flags (e.g. -f, --flag) from text.
func extractFlags(text string) []string {
	re := regexp.MustCompile(`(?m)^\s*(-[a-zA-Z0-9]|--[a-zA-Z0-9\-]+)`)
	matches := re.FindAllStringSubmatch(text, -1)

	uniqueFlags := make(map[string]bool)
	var flags []string
	for _, m := range matches {
		if len(m) > 1 {
			flag := strings.TrimSpace(m[1])
			if !uniqueFlags[flag] {
				uniqueFlags[flag] = true
				flags = append(flags, flag)
			}
		}
	}
	return flags
}

// OsmosisInterceptor intercepts stdout and sends it to the KnowledgeAggregator.
func OsmosisInterceptor() Interceptor {
	aggregator := GetKnowledgeAggregator()

	return func(next RunnerFunc) RunnerFunc {
		return func(ctx context.Context, execCtx *Execution) error {
			err := next(ctx, execCtx)

			// We intercept AFTER the execution completes to grab the captured stdout buffer.
			if execCtx.Stdout.Len() > 0 {
				select {
				case aggregator.events <- OsmosisEvent{
					Command: execCtx.Spec.Command,
					Output:  execCtx.Stdout.String(),
				}:
				default:
					logging.FluentEvent(logging.GetLogger()).
						Warn("KnowledgeAggregator events channel full, dropping osmosis event").
						String("command", execCtx.Spec.Command).
						Log()
				}
			}

			return err
		}
	}
}
