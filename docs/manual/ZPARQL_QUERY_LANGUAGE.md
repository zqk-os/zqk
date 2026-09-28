# ZPARQL Query Language Developer Manual

## Overview

**ZPARQL** (ZQK Pattern and Relational Query Language) is the declarative query engine built natively into the ZQK Knowledge Kernel. Inspired by SPARQL and GraphQL, ZPARQL allows autonomous agents and human developers to traverse, filter, and extract relational graph subtrees across Content-Addressable Storage (CAS) objects.

---

## 1. Core Syntax & Anatomy of a ZPARQL Query

A ZPARQL query specifies projection variables, graph match patterns, filtering constraints, and sorting or limit criteria:

```sparql
SELECT ?bli, ?title, ?status, ?plan
WHERE {
    ?bli a backlog_item .
    ?bli title ?title .
    ?bli status ?status .
    ?bli priority_plan_ref ?plan .
    FILTER (?status = "in_progress" || ?status = "testing")
}
ORDER BY ?title ASC
LIMIT 50
```

### Components:
- **`SELECT`**: Project specific fields or variables bound during graph pattern matching.
- **`WHERE`**: Triple match block where patterns correlate object identifiers, attributes, and relationships.
- **`FILTER`**: Boolean expressions and relational operators evaluating string matches, numerical thresholds, or regular expressions.
- **`ORDER BY` / `LIMIT` / `OFFSET`**: Stream pagination and sorting directives.

---

## 2. Graph Traversal and Relational Navigation

ZQK objects are inherently relational, connected by typed references (e.g. `criteria_refs`, `milestone_refs`, `priority_plan_ref`). ZPARQL allows multi-hop graph walks in a single query:

```sparql
SELECT ?plan_title, ?bli_id, ?crit_id
WHERE {
    ?plan a priority_plan .
    ?plan title ?plan_title .
    ?bli a backlog_item .
    ?bli priority_plan_ref ?plan .
    ?bli id ?bli_id .
    ?bli criteria_refs ?crit_id .
    ?crit a criteria .
    ?crit id ?crit_id .
    FILTER (?plan_title = "PRI-LAUNCH-GAP-REMEDIATION")
}
```

---

## 3. CLI Invocations and Execution

Queries can be evaluated directly via the ZQK CLI using inline query strings or query definition files:

```bash
# Inline execution
zqk query zparql 'SELECT ?id, ?status WHERE { ?id a backlog_item . ?id status ?status }'

# Query from file with JSON output
zqk query zparql --file queries/active_blockers.zparql --format json
```

---

## 4. Built-in Predicates and Functions

| Function / Filter | Description | Example |
| :--- | :--- | :--- |
| `BOUND(?var)` | True if the variable is defined and non-null | `FILTER (BOUND(?commit_hashes))` |
| `REGEX(?var, pattern)` | Regular expression pattern matching | `FILTER (REGEX(?title, "^(feat|fix):"))` |
| `IN(val, ...)` | Set membership check | `FILTER (?status IN ("planned", "testing"))` |
| `CONTAINS(str, sub)` | Substring inclusion check | `FILTER (CONTAINS(?description, "CAS"))` |

---

## Canonical References
- [ZQL Mutations Manual](file:///Users/lanceettl/zqk-public-candidate/docs/manual/ZQL_MUTATIONS.md)
- [Architecture Index](file:///Users/lanceettl/zqk-public-candidate/docs/INDEX.md)
