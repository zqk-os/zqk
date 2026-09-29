# Technical Specification: ZQK Declarative Graph Query & Traversal Engine (ZPARQL)

**Document ID:** `SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR`  
**Status:** Approved Architectural Specification  

---

## 1. Executive Summary & Vision

The **ZQK Knowledge Kernel** forms a rich, multi-layered directed graph where nodes are typed objects (Goals, Milestones, Backlog Items, Criteria, Test Cases, Policies, Persons, Telemetry events) and edges are strongly-typed semantic references (`goal_refs`, `criteria_refs`, `test_case_refs`, `policy_refs`, `parent_ref`, `depends_on`).

Human operators and autonomous swarms frequently need to evaluate complex topological questions:
- *“Find all planned backlog items whose parent goal has open blockers and whose test cases have unpassed criteria.”*
- *“Traverse downstream dependencies of priority plan PRI-XYZ up to depth 3 and project assignee personas.”*
- *“Verify 100% Definition of Done by confirming all criteria reachable from active workstreams have attached test cases with passing QASuccess signatures.”*

Currently, answering these queries requires procedural BFS/DFS sweeps in Go code or multiple sequential CLI calls with JSON piping.

**ZPARQL (ZQK Pattern & Relational Query Language)** is a declarative graph pattern matching language—inspired by Cypher and SPARQL—designed for high-performance, cycle-safe traversals across the kernel object graph.

---

## 2. ISO/IEC 14977 Formal EBNF Grammar

```ebnf
(* ========================================================================== *)
(* ZPARQL: ZQK Graph Pattern Traversal Grammar (ISO/IEC 14977)                 *)
(* ========================================================================== *)

query
    = match_clause , [ where_clause ] , return_clause , [ order_by_clause ] , [ limit_clause ] , ";" ;

(* --- Pattern Matching Clause --- *)
match_clause
    = "MATCH" , path_pattern , { "," , path_pattern } ;

path_pattern
    = node_pattern , { edge_pattern , node_pattern } ;

node_pattern
    = "(" , [ variable_name ] , [ ":" , kind_name ] , [ "{" , property_filters , "}" ] , ")" ;

edge_pattern
    = outgoing_edge | incoming_edge | undirected_edge ;

outgoing_edge
    = "-" , "[" , edge_spec , "]" , "->" ;

incoming_edge
    = "<-" , "[" , edge_spec , "]" , "-" ;

undirected_edge
    = "-" , "[" , edge_spec , "]" , "-" ;

edge_spec
    = [ variable_name ] , [ ":" , edge_type_name ] , [ range_quantifier ] ;

range_quantifier
    = "*" , [ integer , [ ".." , [ integer ] ] ] ;

(* --- Predicates & Filtering --- *)
where_clause
    = "WHERE" , boolean_expr ;

boolean_expr
    = predicate , { ( "AND" | "OR" ) , predicate } ;

predicate
    = [ "NOT" ] , ( comparison_expr | paren_boolean_expr | exists_expr ) ;

paren_boolean_expr
    = "(" , boolean_expr , ")" ;

comparison_expr
    = property_ref , comparison_op , literal ;

comparison_op
    = "=" | "!=" | "<" | "<=" | ">" | ">=" | "CONTAINS" | "IN" ;

exists_expr
    = "EXISTS" , "(" , path_pattern , ")" ;

(* --- Projections & Output --- *)
return_clause
    = "RETURN" , [ "DISTINCT" ] , projection_item , { "," , projection_item } ;

projection_item
    = ( property_ref | aggregate_expr | variable_name ) , [ "AS" , identifier ] ;

aggregate_expr
    = ( "COUNT" | "COLLECT" | "MIN" | "MAX" ) , "(" , ( property_ref | variable_name | "*" ) , ")" ;

order_by_clause
    = "ORDER" , "BY" , property_ref , [ "ASC" | "DESC" ] ;

limit_clause
    = "LIMIT" , integer , [ "OFFSET" , integer ] ;

(* --- Properties & Literals --- *)
property_filters
    = property_assignment , { "," , property_assignment } ;

property_assignment
    = identifier , ":" , literal ;

property_ref
    = variable_name , "." , identifier ;

variable_name
    = identifier ;

kind_name
    = identifier ;

edge_type_name
    = identifier ;

identifier
    = ( letter | "_" ) , { letter | digit | "_" | "-" } ;

integer
    = digit , { digit } ;

literal
    = string_literal | number_literal | boolean_literal | null_literal ;

string_literal
    = '"' , { character - ( '"' | "\" ) | escape_sequence } , '"'
    | "'" , { character - ( "'" | "\" ) | escape_sequence } , "'" ;

number_literal
    = [ "-" ] , digit , { digit } , [ "." , digit , { digit } ] ;

boolean_literal
    = "TRUE" | "FALSE" | "true" | "false" ;

null_literal
    = "NULL" | "null" ;

letter
    = "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J"
    | "K" | "L" | "M" | "N" | "O" | "P" | "Q" | "R" | "S" | "T"
    | "U" | "V" | "W" | "X" | "Y" | "Z"
    | "a" | "b" | "c" | "d" | "e" | "f" | "g" | "h" | "i" | "j"
    | "k" | "l" | "m" | "n" | "o" | "p" | "q" | "r" | "s" | "t"
    | "u" | "v" | "w" | "x" | "y" | "z" ;

digit
    = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;

escape_sequence
    = "\" , ( '"' | "'" | "\" | "n" | "t" | "r" ) ;
```

---

## 3. JSON Schema (Draft 2020-12) Abstract Syntax Tree (AST) Specification

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://zqk.dev/schemas/ast/zparql_query_v1.json",
  "title": "ZPARQL Abstract Syntax Tree",
  "type": "object",
  "required": ["version", "type", "patterns", "returns"],
  "properties": {
    "version": { "type": "string", "const": "1.0.0" },
    "type": { "type": "string", "const": "ZPARQLQuery" },
    "patterns": {
      "type": "array",
      "items": { "$ref": "#/$defs/PathPattern" }
    },
    "where": { "$ref": "#/$defs/WhereClause" },
    "returns": {
      "type": "array",
      "items": { "$ref": "#/$defs/ProjectionItem" }
    },
    "order_by": {
      "type": "array",
      "items": { "$ref": "#/$defs/OrderByItem" }
    },
    "limit": { "type": "integer", "minimum": 0 },
    "offset": { "type": "integer", "minimum": 0 }
  },
  "$defs": {
    "PathPattern": {
      "type": "object",
      "required": ["nodes", "edges"],
      "properties": {
        "nodes": {
          "type": "array",
          "items": { "$ref": "#/$defs/NodePattern" },
          "minItems": 1
        },
        "edges": {
          "type": "array",
          "items": { "$ref": "#/$defs/EdgePattern" }
        }
      }
    },
    "NodePattern": {
      "type": "object",
      "properties": {
        "variable": { "type": "string" },
        "kind": { "type": "string" },
        "filters": {
          "type": "object",
          "additionalProperties": { "type": ["string", "number", "boolean", "null"] }
        }
      }
    },
    "EdgePattern": {
      "type": "object",
      "required": ["direction"],
      "properties": {
        "variable": { "type": "string" },
        "edge_type": { "type": "string" },
        "direction": { "type": "string", "enum": ["OUTGOING", "INCOMING", "UNDIRECTED"] },
        "min_depth": { "type": "integer", "minimum": 1, "default": 1 },
        "max_depth": { "type": "integer", "minimum": 1, "default": 1 }
      }
    },
    "ProjectionItem": {
      "type": "object",
      "required": ["expression"],
      "properties": {
        "expression": { "type": "string" },
        "alias": { "type": "string" },
        "distinct": { "type": "boolean", "default": false }
      }
    },
    "OrderByItem": {
      "type": "object",
      "required": ["property"],
      "properties": {
        "property": { "type": "string" },
        "direction": { "type": "string", "enum": ["ASC", "DESC"], "default": "ASC" }
      }
    },
    "WhereClause": {
      "type": "object",
      "required": ["condition"],
      "properties": {
        "condition": { "type": "string" }
      }
    }
  }
}
```

---

## 4. Relational Subgraph Algebra & Evaluation Proof

A ZPARQL query is translated into a relational operator tree over the kernel knowledge graph:

### 4.1 Algebraic Operators
1. **NodeScan $(\sigma_{k, P}(V))$**: Selects all objects in CAS where $\text{kind} = k$ and satisfying local predicate $P$.
2. **EdgeExpand $(\bowtie_{\text{type}, d})$**: For each node in the input set, resolves outgoing or incoming edges matching $\text{edge\_type}$ via the Reverse Index and Object ID Cache up to depth $d$.
3. **EquiJoin $(\bowtie_{v_1 = v_2})$**: Joins multiple path patterns sharing common variable bindings.
4. **Project $(\pi_{A})$**: Projects requested scalar attributes and aggregate groupings.

### 4.2 Cycle Safety and Determinism
- Traversal engines maintain an in-memory bitset of visited CAS content hashes $\mathcal{H}_{\text{visited}}$ per path traversal.
- If a path expansion reaches an already visited node $h \in \mathcal{H}_{\text{visited}}$, that branch is pruned immediately, guaranteeing termination on cyclic graphs without infinite recursion.

---

## 5. Negative Invariants & Error Taxonomy

| Error Code | Error Condition | Diagnostic Context Emitted |
| :--- | :--- | :--- |
| `ERR_ZPARQL_SYNTAX_ERROR` | Syntax violates EBNF grammar | Source Line, Column, Syntax Token, Expected Tokens |
| `ERR_ZPARQL_DISCONNECTED_VARIABLE` | Node variable appears in MATCH without join to root | Disconnected variable name, enclosing pattern |
| `ERR_ZPARQL_INVALID_EDGE_TYPE` | Edge type does not exist in Ontology Reference Registry | Invalid edge string (e.g. `[:invalid_edge]`) |
| `ERR_ZPARQL_INVALID_DEPTH_RANGE` | Max depth is less than min depth ($*3..1$) | Minimum depth, Maximum depth |
| `ERR_ZPARQL_UNDEFINED_PROJECTION` | Variable in RETURN clause was never bound in MATCH | Unbound projection variable |

---

## 6. Concrete Traversal Examples

### 6.1 Multi-Hop Traceability & DoD Verification
```zparql
MATCH (p:priority_plan {status: "in_progress"})-[:workstream_refs]->(w:workstream),
      (b:backlog_item)-[:priority_plan_ref]->(p),
      (b)-[:criteria_refs]->(c:criteria),
      (c)-[:test_case_refs]->(t:test_case)
WHERE b.status != "complete"
RETURN p.title AS plan, b.id AS bli_id, c.title AS criterion, t.id AS test_case
ORDER BY b.id ASC;
```

### 6.2 Cycle-Safe Dependency Traversal (Depth 1 to 4)
```zparql
MATCH (b:backlog_item {id: "BLI-123"})-[:depends_on*1..4]->(dep:backlog_item)
RETURN dep.id AS dependency_id, dep.status AS status, dep.priority_tier AS tier;
```
