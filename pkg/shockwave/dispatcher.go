package shockwave

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// SimulatedNode represents a target node in the distributed mesh
type SimulatedNode interface {
	Deliver(ctx context.Context, payload interface{}, tiers TierMask) (interface{}, error)
}

// GatewayDispatcher implements gateway dispatch logic that parses a shockwave_router
// object and routes the semantic payload to multiple simulated nodes using a Neuron.
type GatewayDispatcher struct {
	neuron *Neuron
	nodes  map[string]SimulatedNode
	logger logging.Logger
}

// RoutingTask represents a targeted delivery job
type RoutingTask struct {
	NodeID     string
	Payload    interface{}
	Tiers      TierMask
	ResultChan chan<- DeliveryResult
}

// DeliveryResult represents the outcome of a node's delivery
type DeliveryResult struct {
	NodeID string
	Result interface{}
	Error  error
}

// deliveryHandler is an internal handler for the Neuron to process RoutingTasks
type deliveryHandler struct {
	nodes map[string]SimulatedNode
}

func (h *deliveryHandler) Handle(ctx context.Context, payload interface{}, targetTiers TierMask) (interface{}, error) {
	task, ok := payload.(RoutingTask)
	if !ok {
		return nil, fmt.Errorf("invalid payload type: expected RoutingTask")
	}

	node, exists := h.nodes[task.NodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found in gateway registry", task.NodeID)
	}

	res, err := node.Deliver(ctx, task.Payload, task.Tiers)

	if task.ResultChan != nil {
		task.ResultChan <- DeliveryResult{
			NodeID: task.NodeID,
			Result: res,
			Error:  err,
		}
	}

	return res, err
}

// NewGatewayDispatcher creates a new dispatcher with an internal Neuron
func NewGatewayDispatcher(logger logging.Logger, maxWorkers int) *GatewayDispatcher {
	if logger == nil {
		logger = logging.GetLoggerFromProfile("system")
	}

	rules := PropagationRules{
		MaxDepth:    1,
		TargetTiers: ^TierMask(0), // All tiers allowed by default
		MaxWorkers:  maxWorkers,
	}

	neuron := NewNeuron(rules, logger)
	nodes := make(map[string]SimulatedNode)

	neuron.RegisterHandler(&deliveryHandler{nodes: nodes})

	return &GatewayDispatcher{
		neuron: neuron,
		nodes:  nodes,
		logger: logger,
	}
}

// Start initializes the internal Neuron's worker pool
func (g *GatewayDispatcher) Start(ctx context.Context) {
	g.neuron.Start(ctx)
}

// Stop gracefully shuts down the internal Neuron
func (g *GatewayDispatcher) Stop() {
	g.neuron.Stop()
}

// RegisterNode adds a simulated node to the gateway registry
func (g *GatewayDispatcher) RegisterNode(id string, node SimulatedNode) {
	g.nodes[id] = node
}

// ParseTier converts a string tier name to its TierMask equivalent
func ParseTier(tier string) TierMask {
	switch strings.ToLower(tier) {
	case "security":
		return TierSecurity
	case "syntax":
		return TierSyntax
	case "semantic":
		return TierSemantic
	case "execution":
		return TierExecution
	default:
		return 0
	}
}

// Dispatch parses a shockwave_router object and physically routes the
// semantic payload to multiple simulated nodes asynchronously via the Neuron.
// It aggregates the result (Reduce phase) and returns the collected data.
func (g *GatewayDispatcher) Dispatch(ctx context.Context, routerObj map[string]interface{}, payload interface{}) ([]interface{}, error) {
	// Extract kind
	kind, _ := routerObj[objects.FieldKeyKind].(string)
	if kind != "shockwave_router" {
		return nil, fmt.Errorf("expected kind 'shockwave_router', got %s", kind)
	}

	// Parse spec
	specAny, ok := routerObj["spec"]
	if !ok {
		return nil, fmt.Errorf("missing spec in shockwave_router")
	}

	specMap, ok := specAny.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("spec is not a map")
	}

	routesAny, ok := specMap["routes"]
	if !ok {
		return nil, fmt.Errorf("missing routes in spec")
	}

	routesList, ok := routesAny.([]interface{})
	if !ok {
		return nil, fmt.Errorf("routes is not a list")
	}

	// Parse ReduceStrategy (optional)
	timeout := time.Second * 5 // Default timeout
	requireAll := false
	if rsAny, ok := specMap[objects.FieldKeyReduceStrategy]; ok {
		if rsMap, ok := rsAny.(map[string]interface{}); ok {
			if ra, ok := rsMap["require_all"].(bool); ok {
				requireAll = ra
			}
			switch tm := rsMap["timeout_ms"].(type) {
			case float64:
				timeout = time.Duration(tm) * time.Millisecond
			case int:
				timeout = time.Duration(tm) * time.Millisecond
			}
		}
	}

	resultsChan := make(chan DeliveryResult, len(routesList))
	expectedResults := 0

	// Submit tasks to Neuron
	for _, routeAny := range routesList {
		routeMap, ok := routeAny.(map[string]interface{})
		if !ok {
			continue
		}

		nodeID, ok := routeMap["node_id"].(string)
		if !ok || nodeID == "" {
			continue
		}

		var mask TierMask
		if tiersList, ok := routeMap["tiers"].([]interface{}); ok {
			for _, t := range tiersList {
				if ts, ok := t.(string); ok {
					mask |= ParseTier(ts)
				}
			}
		} else {
			// If no tiers specified, default to full routing
			mask = ^TierMask(0)
		}

		task := RoutingTask{
			NodeID:     nodeID,
			Payload:    payload,
			Tiers:      mask,
			ResultChan: resultsChan,
		}

		err := g.neuron.Send(ctx, task)
		if err != nil {
			if g.logger != nil {
				logging.Fluent(g.logger).Error("Failed to submit routing task to neuron", err).Log()
			}
		} else {
			expectedResults++
		}
	}

	// Reduce phase: aggregate results
	var aggregated []interface{}

	if expectedResults == 0 {
		return aggregated, nil
	}

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for i := 0; i < expectedResults; i++ {
		select {
		case <-ctx.Done():
			return aggregated, ctx.Err()
		case <-timeoutTimer.C:
			if requireAll {
				return aggregated, fmt.Errorf("reduce phase timeout waiting for all results")
			}
			return aggregated, nil
		case res := <-resultsChan:
			if res.Error != nil {
				if requireAll {
					return aggregated, fmt.Errorf("node %s failed: %w", res.NodeID, res.Error)
				}
				if g.logger != nil {
					logging.Fluent(g.logger).Warn(fmt.Sprintf("node %s failed: %v", res.NodeID, res.Error)).Log()
				}
			} else {
				aggregated = append(aggregated, res.Result)
			}
		}
	}

	return aggregated, nil
}

// Reviewed and verified for Shockwave MVP
// Initializing Shockwave Protocol Dispatcher
