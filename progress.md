# graphContext — Progress Board

## Completed Milestones

### M0.1 — Storage Foundation (Schema v3)
- PR: [#2](https://github.com/vedansh-5/graphContext/pull/2)
- Package: `pkg/store`
- Deliverables:
  - SQLite schema v3: nodes, edges, file content hashes, metadata tables.
  - Batched transactional write engine.
  - Typed reads (`[]Node`, `[]Edge`).
  - FTS5 subword tokenization and symbol search.

### M0.2 — Language Layer (Pass 1)
- PR: [#2](https://github.com/vedansh-5/graphContext/pull/2)
- Package: `pkg/lang`
- Deliverables:
  - Language-neutral intermediate representation (`FileIR`, `ImportRef`, `Ref`, `TypeFacts`).
  - Extension registry and shared tree-sitter AST helpers.
  - Go plugin (`pkg/lang/golang`): functions, methods, receiver types, struct field types, interface method sets, imports, calls.
  - Python plugin (`pkg/lang/python`): functions, classes, decorators, inheritance, type annotations, `__init__` constructor facts.
  - TypeScript/JavaScript plugin (`pkg/lang/typescript`): functions, classes, interfaces, type aliases, class fields, imports/re-exports, `new` expressions.

### M0.3 (Part 1) — Cross-File Reference Resolver (Pass 2)
- PR: [#3](https://github.com/vedansh-5/graphContext/pull/3)
- Package: `pkg/resolver`
- Deliverables:
  - Global symbol & module indexing across multiple files.
  - Receiver type propagation (`this`, `self`, `super`, local variables, selector chains like `r.db`).
  - Base class and interface hierarchy traversal.
  - Tiered resolution engine with confidence levels (`exact`, `ambiguous`, `name_match`, `unknown`).
  - Structural interface satisfaction discovery (method set subset matching for Go and TypeScript).
  - Deterministic sorting and resolution rate tracking.

### M0.3 (Part 2) — Incremental Indexing Pipeline (Pass 3)
- PR: [#4](https://github.com/vedansh-5/graphContext/pull/4)
- Package: `pkg/indexer`
- Deliverables:
  - 3-pass incremental pipeline coordinator (`EnsureFresh`).
  - SHA256 content hashing to skip unchanged files on subsequent runs.
  - Atomic transactional batch commits to SQLite store.
  - Automatic purging and edge cascading for deleted files.

### M1 (Part 1) — In-Memory Graph & Analysis Engine
- PR: [#5](https://github.com/vedansh-5/graphContext/pull/5)
- Package: `pkg/analysis`
- Deliverables:
  - Bi-directional in-memory graph (`Out` and `In` adjacency lists).
  - `ReverseReach`: layered BFS over incoming edges for impact and caller analysis.
  - `ForwardReach`: layered BFS over outgoing edges for dependency analysis.
  - `Trace`: forward execution call-tree generator with cycle detection and budget caps.
  - `Neighborhood`: subgraph extraction around seed symbols with radius limits.
  - `PathsBetween`: path enumeration between symbols.
  - `DegreeMetrics`: in/out degree calculation.
  - `SCCs`: Tarjan's Strongly Connected Components algorithm for circular dependency detection.
  - `DeadCandidates`: multi-source BFS from root entrypoints (`main`, routes, tests) for unreachable code detection.
  - `Condense`: module-level quotient graph with coupling weights and Mermaid diagram generation.

---

## Active Milestone

### M1 (Part 2) — MCP Server & Core Reasoning Tools
- Branch: `feat/mcp-tools`
- Package: `pkg/mcp_server`
- Goals:
  - Uniform JSON-RPC response envelope (`answer`, `provenance`, `confidence`, `stats`, `graph_meta`).
  - Stdio MCP server with session management.
  - 6 Core Reasoning Tools:
    1. `search_symbols(query, kind?, limit)`: FTS5 entrypoint.
    2. `get_context(symbol, radius, include_source?)`: Symbol-centered context pack.
    3. `get_task_context(task, limit)`: Context Engine (task string -> ranked, bounded context pack).
    4. `impact_of_change(symbol | diff, change_type, max_depth)`: Reverse reachability impact set + affected tests.
    5. `trace(from, to?, direction, max_depth, level)`: Call-tree execution trace + paths between.
    6. `repo_overview(analysis, level)`: Architecture map, cycles, dead code candidates, coupling metrics.

---

## Roadmap

- **M2 — Change Intelligence**: Diff-aware impact analysis, test selection, architecture rules.
- **M3 — Daemon**: Debounced file watcher, hot graph, idle eviction.
- **M4 — Validation & Benchmarks**: Token-cost and resolver accuracy benchmarks.
- **M5 — Research Artifact**: Mutation-derived agent evaluation.
