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

### M1 (Part 2) — MCP Server & Core Reasoning Tools
- PR: [#6](https://github.com/vedansh-5/graphContext/pull/6)
- Branch: `feat/mcp-tools`
- Package: `pkg/mcp_server`
- Deliverables:
  - Uniform JSON-RPC response envelope (`Envelope`: `answer`, `caveats`, `stats`, `graph_meta`).
  - Stdio MCP server with per-project session cache (`store.Store` + `indexer.EnsureFresh` + in-memory `analysis.Graph`).
  - 6 Core Reasoning Tools:
    1. `search_symbols(query, kind?, limit)`: FTS5 matching with in-memory fallback.
    2. `get_context(symbol, radius?, include_source?)`: Symbol-centered context pack with 1-hop callers, callees, inheritance, and optional source snippet.
    3. `get_task_context(task, limit?)`: Context engine scoring task prompt tokens, expanding bounded neighborhoods, and returning ranked context packs.
    4. `impact_of_change(symbol, change_type?, max_depth?, limit?)`: Reverse reachability impact set, affected test files, entrypoints, and dangling references on delete.
    5. `trace(from, to?, direction?, max_depth?, limit?)`: Call-tree execution trace (with recursion detection) or call paths between two symbols.
    6. `repo_overview(analysis?, level?, top?)`: Architecture map with Mermaid diagram, SCC circular dependencies, dead code candidates, and module coupling metrics.
  - Comprehensive end-to-end test suite in `pkg/mcp_server/server_test.go`.

---

## Active Milestone

### M2 — Change Intelligence
- Goals:
  - Diff-aware impact analysis (`git diff` parsing to identify modified AST symbols).
  - Smart test selection based on reverse reachability from changed symbols.
  - Architectural boundary rules and linter.

---

## Roadmap

- **M3 — Daemon**: Debounced file watcher, hot graph, idle eviction.
- **M4 — Validation & Benchmarks**: Token-cost and resolver accuracy benchmarks.
- **M5 — Research Artifact**: Mutation-derived agent evaluation.
