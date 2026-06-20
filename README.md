# CodeGraphContext MCP Server

A high-performance, local Model Context Protocol (MCP) server written in Go that parses Python codebases into a queryable relational graph (Nodes and Edges) inside SQLite. It allows AI agents to dynamically inspect function relationships, dependencies, and call hierarchies in real-time.

---

## Current Features & Functionality

1. **Dynamic Workspace Isolation:**
   - Instead of global databases or hardcoded configuration paths, the server dynamically accepts a `project_path` from the AI client at runtime.
   - Spawns and manages a private SQLite database (`graph.db`) inside the target codebase's directory, ensuring strict workspace isolation without crossover or interference.

2. **AST Parsing via Tree-sitter:**
   - Utilizes `smacker/go-tree-sitter` and the official Python grammar to build precise Abstract Syntax Trees (ASTs).
   - Extracts all Python function definitions with exact byte offsets.
   - Extracts function calls utilizing parent-node AST traversal, allowing it to correctly resolve containing callers regardless of nesting depth (e.g., inside loops, conditional statements, or try-catch blocks).

3. **Concurrent Crawler & Parser Workers:**
   - Implements a fast producer-consumer model using Go channels.
   - A single-threaded walker traverses the directory tree and feeds `.py` files into a buffered queue.
   - A pool of 4 concurrent worker goroutines pull files from the queue, parse their ASTs, and write nodes/edges to SQLite.

4. **Robust Relational Storage:**
   - Uses SQLite in **Write-Ahead Logging (WAL) Mode** for safe concurrent read/write access.
   - Stores functions as nodes and calls as edges, mapping cross-file dependencies using global abstract identifiers (`abstract:calleeName`) to satisfy foreign key constraints.

---

## Current Architecture & Implementation

```
├── main.go                     # Lightweight server entry point
├── go.mod                      # Module definition & dependencies
├── pkg/
│   ├── crawler/
│   │   └── walker.go           # Directory walker (producer)
│   ├── parser/
│   │   └── python.go           # Tree-sitter AST parser
│   ├── storage/
│   │   ├── sqlite.go           # SQLite database setup (WAL mode)
│   │   └── crud.go             # Node/Edge upsert and query functions
│   └── mcp_server/
│       └── server.go           # Stdio-based MCP server & tool definitions
```

* **Communication Protocol:** Stdio-based JSON-RPC (Model Context Protocol). All non-JSON logs and debug statements are routed to `os.Stderr` to avoid polluting the JSON-RPC standard output stream.
* **Exposed Tools:**
  * `get_callers(target_function, project_path)`: Scans the codebase on the first call, initializes the local database, and returns all functions calling `target_function`.

---

## Planned Features

* **Multi-Language Support:**
  * Add parsers for Go (`go-tree-sitter/go`), TypeScript/JavaScript, and Java to map cross-language microservice calls or multi-language monorepos.
* **Incremental Scanning:**
  * Store file modification times (`mtime`) or file hashes in SQLite. Skip parsing unchanged files to make rescans near-instantaneous on large codebases.
* **Rich AST Relationship Mapping:**
  * Parse class hierarchies (inheritance, interface implementation).
  * Parse variable assignments and symbol usage to find where global/struct variables are read or written.
* **Extended Query Capability:**
  * Add a `get_callees` tool (find what functions a target function calls).
  * Add a `find_path` tool (find the execution path between Function A and Function B).
  * Cycle detection (flag circular recursion in the call graph).
