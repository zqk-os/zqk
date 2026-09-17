# Graph Mutation Cypher Architecture

## Overview
This document specifies the Cypher implementation, index schemas, and transaction patterns for the ACID-compliant graph mutation commits and idempotency deduplication within the ZQK Graph-State Sync Loop.

## 1. Idempotency Deduplication

### Constraint (Run Once on Migration)
Ensures sub-millisecond lookups and prevents duplicate audit nodes at the DB level.
```cypher
CREATE CONSTRAINT ik_mutation_unique FOR (m:MutationAudit) REQUIRE m.idempotency_key IS UNIQUE;
```

### Go Implementation: `HasCommittedMutation`
```go
func (g *GraphImpl) HasCommittedMutation(ctx context.Context, ik string) (bool, error) {
	query := `MATCH (m:MutationAudit {idempotency_key: $ik}) RETURN count(m) > 0 AS exists;`
	
	var result map[string]interface{}
	err := g.session.ExecuteRead(ctx, func(tx neo4j.TransactionContext) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{"ik": ik})
		if err != nil { return nil, err }
		records, err := res.Single()
		if err != nil { return nil, err }
		return records.AsMap(), nil
	})
	
	if err != nil { return false, err }
	if result == nil { return false, nil }
	return result["exists"].(bool), nil
}
```

## 2. ACID Mutation Commit

### Cypher Query (Single Transaction)
```cypher
UNWIND $mutations AS m
WITH m, $task_id AS tid, $ik_prefix AS ik_base
MERGE (target:{m.target_kind} {id: m.target_id})
ON CREATE SET target.kind = m.target_kind, target.created_at = datetime()
ON MATCH SET target += m.fields

FOREACH (e IN CASE WHEN m.edges IS NOT NULL THEN m.edges ELSE [] END |
  CALL { 
    WITH e, target
    OPTIONAL MATCH (rel_node) WHERE rel_node.id = e.target_id AND rel_node.kind = e.target_kind
    MERGE (target)-[r:` + e.relation + `]->(rel_node)
    ON CREATE SET r.idempotency_key = ik_base + '_' + rel_node.id, r.created_at = datetime()
  }
)

CREATE (a:MutationAudit {
  idempotency_key: $ik,
  task_id: tid,
  action: m.action,
  safety_class: m.safety_class,
  status: "APPLIED",
  timestamp: datetime()
})
RETURN count(a) AS applied_count;
```

### Go Implementation: `CommitMutation`
```go
func (g *GraphImpl) CommitMutation(ctx context.Context, ik string, mutations []mutation.Mutation, taskID string) error {
    // Map mutations to driver-compatible slice
    type mutationPayload struct {
        Action      string                  `json:"action"`
        SafetyClass string                  `json:"safety_class"`
        TargetKind  string                  `json:"target_kind"`
        TargetID    string                  `json:"target_id"`
        Fields      map[string]interface{}  `json:"fields"`
        Edges       []mutation.EdgeMutation `json:"edges"`
    }
    
    payloads := make([]mutationPayload, len(mutations))
    for i, m := range mutations {
        payloads[i] = mutationPayload{
            Action:      m.Action,
            SafetyClass: m.SafetyClass,
            TargetKind:  m.TargetKind,
            TargetID:    m.TargetID,
            Fields:      m.Fields,
            Edges:       m.Edges,
        }
    }

    // Pass 'payloads' to the Cypher query and verify 'applied_count' == len(mutations)
    // ...
}
```
