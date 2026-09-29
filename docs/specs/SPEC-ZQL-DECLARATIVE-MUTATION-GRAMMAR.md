# Technical Specification: ZQK Declarative Mutation Language (ZQL) Grammar, AST, and Variable Resolution

**Document ID:** `SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR`  
**Status:** Approved Architectural Specification  

---

## 1. Executive Summary & Vision

The **ZQK Knowledge Kernel** models all system artifacts (requirements, backlog items, priority plans, test cases, criteria, policies, prompts, personas, telemetry records) as discrete, typed objects governed by schema contracts, CAS storage, and lifecycle state machines.

Currently, multi-object creation, complex dependency wiring, and batch mutations require sequential CLI invocations (`zqk object create ...`) or hand-crafted procedural scripts. This approach suffers from:
1. **Lack of Transactional Atomicity**: If statement 4 in a 7-step sequence fails schema validation, the first 3 objects remain orphaned in CAS without automatic rollback.
2. **Procedural Foreign-Key Plumbing**: Agents must manually extract generated object IDs and string-interpolate them into subsequent object definitions.
3. **Absence of Preflight Graph Feasibility**: Syntax and reference errors are caught at write time rather than preflight compile time.

**ZQL (ZQK Query & Mutation Language)** is a declarative, language-agnostic domain specific language (DSL) for atomic multi-object mutations, topological variable bindings, and ACID transaction boundaries over the ZQK knowledge graph.

---

## 2. ISO/IEC 14977 Formal EBNF Grammar

The canonical formal grammar for ZQL is defined below in standard ISO/IEC 14977 syntax:

```ebnf
(* ========================================================================== *)
(* ZQL: ZQK Declarative Mutation Language Grammar (ISO/IEC 14977)              *)
(* ========================================================================== *)

program
    = { statement } ;

statement
    = transaction_stmt
    | let_stmt
    | upsert_stmt
    | delete_stmt
    | commit_stmt
    | rollback_stmt ;

(* --- Transaction Boundaries --- *)
transaction_stmt
    = "BEGIN" , [ "TRANSACTION" ] , [ transaction_options ] , ";" ;

transaction_options
    = "ISOLATION" , "LEVEL" , isolation_level ;

isolation_level
    = "STAGED_SNAPSHOT" | "SERIALIZABLE" | "READ_COMMITTED" ;

commit_stmt
    = "COMMIT" , [ "TRANSACTION" ] , ";" ;

rollback_stmt
    = "ROLLBACK" , [ "TRANSACTION" ] , ";" ;

(* --- Binding Statements --- *)
let_stmt
    = "LET" , variable_ident , "=" , expression , ";" ;

variable_ident
    = "$" , identifier ;

(* --- Mutation Statements --- *)
upsert_stmt
    = "UPSERT" , [ kind_ident ] , [ "ID" , ( string_literal | variable_ref ) ]
      , "WITH" , object_literal
      , [ "RETURNING" , projection_clause ]
      , ";" ;

delete_stmt
    = "DELETE" , [ kind_ident ] , ( string_literal | variable_ref )
      , [ "CASCADE" | "RESTRICT" ]
      , ";" ;

kind_ident
    = identifier ;

projection_clause
    = "*" | ( projection_item , { "," , projection_item } ) ;

projection_item
    = expression , [ "AS" , identifier ] ;

(* --- Literals and Object Payloads --- *)
object_literal
    = "{" , [ field_assignment , { "," , field_assignment } ] , "}" ;

field_assignment
    = ( identifier | string_literal ) , ":" , expression ;

expression
    = literal
    | variable_ref
    | list_literal
    | object_literal
    | binary_expr ;

variable_ref
    = variable_ident , [ "." , path_segment , { "." , path_segment } ] ;

path_segment
    = identifier ;

literal
    = string_literal
    | number_literal
    | boolean_literal
    | null_literal ;

list_literal
    = "[" , [ expression , { "," , expression } ] , "]" ;

binary_expr
    = expression , binary_op , expression ;

binary_op
    = "+" | "-" | "==" | "!=" | "AND" | "OR" ;

(* --- Lexical Tokens --- *)
identifier
    = ( letter | "_" ) , { letter | digit | "_" | "-" } ;

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

All ZQL source modules parse into an intermediate JSON AST representation adhering to the following JSON Schema:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://zqk.dev/schemas/ast/zql_statement_v1.json",
  "title": "ZQL Abstract Syntax Tree",
  "type": "object",
  "required": ["version", "type", "statements"],
  "properties": {
    "version": {
      "type": "string",
      "const": "1.0.0"
    },
    "type": {
      "type": "string",
      "const": "Program"
    },
    "statements": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/Statement"
      }
    }
  },
  "$defs": {
    "Statement": {
      "type": "object",
      "required": ["node_type", "line", "column"],
      "properties": {
        "node_type": {
          "type": "string",
          "enum": ["BeginTransaction", "CommitTransaction", "RollbackTransaction", "Let", "Upsert", "Delete"]
        },
        "line": { "type": "integer", "minimum": 1 },
        "column": { "type": "integer", "minimum": 1 }
      },
      "discriminator": {
        "propertyName": "node_type"
      }
    },
    "BeginTransaction": {
      "allOf": [
        { "$ref": "#/$defs/Statement" },
        {
          "properties": {
            "isolation_level": {
              "type": "string",
              "enum": ["STAGED_SNAPSHOT", "SERIALIZABLE", "READ_COMMITTED"],
              "default": "STAGED_SNAPSHOT"
            }
          }
        }
      ]
    },
    "Let": {
      "allOf": [
        { "$ref": "#/$defs/Statement" },
        {
          "required": ["variable_name", "expression"],
          "properties": {
            "variable_name": { "type": "string", "pattern": "^[a-zA-Z_][a-zA-Z0-9_-]*$" },
            "expression": { "$ref": "#/$defs/Expression" }
          }
        }
      ]
    },
    "Upsert": {
      "allOf": [
        { "$ref": "#/$defs/Statement" },
        {
          "required": ["kind", "payload"],
          "properties": {
            "kind": { "type": "string" },
            "id": { "$ref": "#/$defs/Expression" },
            "bind_as": { "type": "string" },
            "payload": { "$ref": "#/$defs/ObjectLiteral" },
            "returning": {
              "type": "array",
              "items": { "$ref": "#/$defs/Expression" }
            }
          }
        }
      ]
    },
    "Delete": {
      "allOf": [
        { "$ref": "#/$defs/Statement" },
        {
          "required": ["kind", "id"],
          "properties": {
            "kind": { "type": "string" },
            "id": { "$ref": "#/$defs/Expression" },
            "cascade_mode": { "type": "string", "enum": ["CASCADE", "RESTRICT"], "default": "RESTRICT" }
          }
        }
      ]
    },
    "Expression": {
      "type": "object",
      "required": ["expr_type"],
      "properties": {
        "expr_type": {
          "type": "string",
          "enum": ["Literal", "VariableRef", "ObjectLiteral", "ListLiteral", "BinaryExpr"]
        }
      }
    },
    "VariableRef": {
      "allOf": [
        { "$ref": "#/$defs/Expression" },
        {
          "required": ["variable_name", "path"],
          "properties": {
            "variable_name": { "type": "string" },
            "path": {
              "type": "array",
              "items": { "type": "string" }
            }
          }
        }
      ]
    },
    "ObjectLiteral": {
      "allOf": [
        { "$ref": "#/$defs/Expression" },
        {
          "required": ["fields"],
          "properties": {
            "fields": {
              "type": "object",
              "additionalProperties": { "$ref": "#/$defs/Expression" }
            }
          }
        }
      ]
    },
    "ListLiteral": {
      "allOf": [
        { "$ref": "#/$defs/Expression" },
        {
          "required": ["items"],
          "properties": {
            "items": {
              "type": "array",
              "items": { "$ref": "#/$defs/Expression" }
            }
          }
        }
      ]
    }
  }
}
```

---

## 4. Deterministic Variable Binding & Kahn Topological Resolution

When mutating interrelated graph nodes, statements may reference fields of other mutations (e.g., child items referencing `$parent_plan.id`).

### 4.1 Dependency Graph Construction
Given a set of statements $S = \{s_1, s_2, \dots, s_n\}$:
1. Define $\text{Def}(s_i)$ as the variable bound by $s_i$ (e.g., `LET $x = ...` or `UPSERT ... AS $x`).
2. Define $\text{Ref}(s_i)$ as the set of variable names referenced in the expressions of $s_i$.
3. Construct a directed dependency graph $G = (V, E)$ where $V = S$ and a directed edge $(s_j \to s_i) \in E$ indicates that $s_i$ depends on $\text{Def}(s_j)$.

### 4.2 Topological Ordering via Kahn's Algorithm
The compiler schedules execution using Kahn's algorithm:
1. Compute in-degree $\text{deg}^-(s)$ for all $s \in V$.
2. Initialize queue $Q = \{s \in V \mid \text{deg}^-(s) = 0\}$.
3. While $Q \neq \emptyset$:
   - Dequeue $u \leftarrow Q$; append $u$ to execution plan $L$.
   - For each edge $(u, v) \in E$:
     - Decrement $\text{deg}^-(v)$.
     - If $\text{deg}^-(v) = 0$, enqueue $v \to Q$.
4. **Negative Invariant Check**: If $|L| < |V|$, there exists at least one cycle $C \subseteq V$. The compiler aborts compilation and emits `ERR_ZQL_CIRCULAR_DEPENDENCY`.

---

## 5. Negative Invariants & Error Taxonomy

ZQL enforces strict preflight rejection of unsafe or ambiguous mutations:

| Error Code | Error Condition | Diagnostic Context Emitted |
| :--- | :--- | :--- |
| `ERR_ZQL_SYNTAX_ERROR` | Token stream violates EBNF grammar | File, Line, Column, Unexpected Token, Expected Tokens |
| `ERR_ZQL_UNBOUND_VARIABLE` | `$var.field` referenced without prior definition | Referenced variable name, referencing AST statement line/col |
| `ERR_ZQL_CIRCULAR_DEPENDENCY` | Variable dependency cycle detected by Kahn's algorithm | Full cycle dependency chain (e.g., `$a -> $b -> $c -> $a`) |
| `ERR_ZQL_INVALID_KIND` | Kind identifier does not exist in Kernel Kind Catalog | Unrecognized kind string, closest matches |
| `ERR_ZQL_UNRESOLVED_PATH` | Property path segment does not exist on target schema | Root variable, invalid path segment, schema definition |

---

## 6. Concrete Example: Atomic Workstream Initiation

```zql
BEGIN TRANSACTION ISOLATION LEVEL STAGED_SNAPSHOT;

LET $ws_title = "Swarm Telemetry Overhaul";

LET $plan = UPSERT priority_plan {
    title: $ws_title,
    priority_tier: "P1",
    status: "in_progress",
    description: "Multi-seat event streaming pipeline"
} RETURNING id, version_context;

UPSERT backlog_item {
    title: "Implement Stream Consumer Daemon",
    priority_plan_ref: $plan.id,
    priority_tier: "P1",
    status: "planned",
    estimated_effort: "4h"
};

UPSERT backlog_item {
    title: "Stream Buffer Shockwave Verification",
    priority_plan_ref: $plan.id,
    priority_tier: "P1",
    status: "planned",
    estimated_effort: "2h"
};

COMMIT TRANSACTION;
```
