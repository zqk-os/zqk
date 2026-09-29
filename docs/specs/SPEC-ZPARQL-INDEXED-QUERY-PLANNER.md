# Technical Specification: ZPARQL Index-Accelerated Query Planner and Cycle-Safe Traversal Engine

**Document ID:** `SPEC-ZPARQL-INDEXED-QUERY-PLANNER`  
**Status:** Approved Architectural Specification  

---

## 1. Executive Summary & Vision

As the ZQK Knowledge Kernel scales to tens of thousands of objects (requirements, backlog items, test cases, criteria, telemetry runs), unindexed graph traversals and naive Breadth-First / Depth-First Searches incur $O(N)$ execution cost across the entire graph space $N$.

The **ZPARQL Query Planner and Traversal Engine** provides:
1. **Logical to Physical Plan Optimization**: Translates declarative pattern queries into optimized physical execution plans incorporating predicate pushdown and index seeks.
2. **$O(K)$ Subgraph Traversal Complexity**: Bounded complexity proportional to matched subgraph cardinality $K$ rather than total graph space $N$ via inverted edge indexes and kind indexes.
3. **Cycle-Safe and Depth-Bounded Traversal**: Strict cycle detection using visited-node bitsets and configurable maximum recursion depth limits ($D_{\max}$) to prevent infinite loops and stack exhaustion in cyclic graph topologies.

---

## 2. Query Planner Execution Contract

### 2.1 Logical & Physical Plan Representation
The planner translates high-level ZPARQL AST queries into a directed pipeline of physical operators:
- **IndexSeek**: Resolves seed nodes in $O(1)$ time via unique identifier or indexed property.
- **KindScan**: Scans only entities of the target kind via the inverted kind index.
- **EdgeExpand**: Navigates forward or backward relationships using adjacency indexes without scanning non-adjacent nodes.
- **Filter**: Evaluates predicate expressions pushed down as close as possible to the scan/expand operators.
- **Project**: Extracts and transforms returned properties.

### 2.2 Predicate Pushdown & Index Selection Contract
1. If a node pattern specifies a concrete `id` (`(n {id: "..."})`), the planner selects an `IndexSeek` operation with cost 1.
2. If a node pattern specifies a `kind` and property filters, property predicates are pushed down into the initial scan operator rather than evaluated post-join.
3. Edge traversals leverage directed index structures (`SourceID -> Relation -> TargetIDs`), avoiding graph-wide edge table scans.

---

## 3. $O(K)$ Complexity Bound via Secondary Indexes

In a graph $G = (V, E)$ with $|V| = N$ nodes and $|E| = M$ edges:
- **Naive Traversal**: Traverses all $N$ nodes and inspects all edges, yielding $O(N + M)$ complexity.
- **Indexed Traversal**: Given starting node $s$ and a traversal depth $d$ matching $K$ connected entities:
  $$\text{Complexity} = O(K) \quad \text{where } K \ll N$$
- The traversal engine tracks inspection operations. In an indexed lookup of $K$ records within a graph of $N = 10,000$ nodes, total operations executed MUST NOT exceed $C \times K$ (where constant $C < 10$), completely decoupling latency from total graph size $N$.

---

## 4. Cycle Safety & Negative Recursion Limits

### 4.1 Visited Set Tracking
To guarantee termination on cyclic graphs ($A \to B \to C \to A$):
1. The traversal engine maintains a `VisitedSet` tracking traversed node identifiers along each evaluation path.
2. If an edge expansion encounters an already visited node along the current branch:
   - In acyclic traversal mode: The edge is skipped without re-visiting the node.
   - In cyclic path query mode: The cycle is recorded and terminated.

### 4.2 Depth-Bounded Recursion & Fail-Closed Guard
1. **Maximum Recursion Depth ($D_{\max}$)**: Queries specify a maximum traversal hop limit (default: 16; configurable up to 64).
2. **Fail-Closed Abortion**: If a traversal branch attempts to exceed $D_{\max}$, the engine halts traversal immediately and raises `ERR_ZPARQL_CYCLIC_RECURSION_LIMIT`.
3. Stack exhaustion and infinite loops are strictly prohibited.
