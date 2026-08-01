package swarm

import (
	"context"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/shockwave"
)

// Runner represents the interface for executing cognitive run loops.
type Runner interface {
	Run(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// runnerHandler adapts a Runner to the shockwave.Handler interface
type runnerHandler struct {
	runner Runner
}

func (h *runnerHandler) Handle(ctx context.Context, payload interface{}, targetTiers shockwave.TierMask) (interface{}, error) {
	task, ok := payload.(Task)
	if !ok {
		return nil, nil // Ignore non-tasks or log error
	}
	res, err := h.runner.Run(ctx, task.SystemPrompt, task.UserPrompt)
	return TaskResult{Task: task, Result: res, Error: err}, nil
}

// Dispatcher manages the distribution of tasks to runners using the Shockwave Protocol.
type Dispatcher struct {
	queue  TaskQueue
	neuron *shockwave.Neuron
}

// NewDispatcher creates a new task dispatcher powered by an Event-Driven Neuron.
func NewDispatcher(queue TaskQueue, runners []Runner) *Dispatcher {
	// Initialize a new Neuron with default propagation rules
	neuron := shockwave.NewNeuron(shockwave.PropagationRules{
		MaxDepth:    1,
		TargetTiers: shockwave.TierExecution,
	}, nil)

	// Map all legacy runners to the new Shockwave handlers
	for _, r := range runners {
		neuron.RegisterHandler(&runnerHandler{runner: r})
	}

	return &Dispatcher{
		queue:  queue,
		neuron: neuron,
	}
}

// Start begins processing tasks from the queue using the shockwave neuron.
func (d *Dispatcher) Start(ctx context.Context) <-chan TaskResult {
	results := make(chan TaskResult)
	logger := logging.GetLogger()

	logging.FluentEvent(logger).Info("Swarm task dispatcher started via Shockwave Neuron").Log()

	// Start the neuron's async background loop
	d.neuron.Start(ctx)

	// Ingress loop: read from legacy TaskQueue and send to Neuron inbox
	go func() {
		defer d.neuron.Stop()
		for {
			task, err := d.queue.Dequeue(ctx)
			if err != nil {
				return // Context canceled or queue closed
			}
			_ = d.neuron.Send(ctx, task)
		}
	}()

	// Egress loop: read from Neuron outbox and forward to results channel
	go func() {
		defer close(results)
		outbox := d.neuron.Receive()
		for res := range outbox {
			if tr, ok := res.(TaskResult); ok {
				select {
				case <-ctx.Done():
					return
				case results <- tr:
				}
			}
		}
		logging.FluentEvent(logger).Info("Swarm task dispatcher stopped").Log()
	}()

	return results
}
