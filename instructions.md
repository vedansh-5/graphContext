# Project Instructions & Engineering Guidelines

This document tracks all project-specific directives, architecture rules, and workflow preferences provided by the user. Refer to this file before planning, implementing, or creating PRs.

---

## 1. Incremental Delivery & PR Strategy
* **Small, Bite-Sized PRs**: Never bundle an entire milestone into a single massive PR. Break milestones down into small, independently reviewable tasks.
* **Complete Deliverables per PR**: Each PR must update the project as a whole:
  * Minimal, focused code changes.
  * Accompanying unit/integration tests (`go test ./...` must pass).
  * Updated tracking documentation (`progress.md`).
* **Step-by-Step Progress**: Complete and verify each PR before advancing to the next task.
* **Concise PR Descriptions**: Every PR must include a brief technical description covering what changed and the underlying technical details.

---

## 2. Code Style & Software Engineering Principles
* **DRY (Don't Repeat Yourself)**: Eliminate duplicated logic across parsers, resolvers, stores, and MCP tools.
* **Minimal Comments & Self-Documenting Code**:
  * Do not write noisy or obvious comments.
  * Name functions and variables explicitly after their behavior and responsibilities so the code explains itself.
* **Deep Explanations When Presenting Code**: When discussing architectural choices or presenting code blocks, always address:
  1. *Why this approach?*
  2. *How it benefits us?*
  3. *What alternatives were considered and skipped?*

---

## 3. Architectural Invariants
* **Strict Stdio Isolation (Zero Stdout Pollution)**:
  * `os.Stdout` is strictly reserved for the MCP JSON-RPC wire protocol.
  * Never write logging or progress messages to standard output in server packages.
  * Route all diagnostic logs and debugging traces to `os.Stderr` (via `fmt.Fprintf(os.Stderr, ...)` or standard logger).
* **Deterministic Responses**:
  * Same repository state must yield byte-identical answers.
  * Always sort slices (`sort.Slice`, `sort.Strings`) after map iterations or set collections to counter Go's randomized map iteration order.
* **External Storage Cache**:
  * SQLite database files must never live inside the analyzed repository (which pollutes user working trees and triggers file watchers).
  * Store all databases in the user's cache directory: `~/.cache/graphcontext/<repo_hash>/graph.db`.
* **Honest Confidence Tiers**:
  * Edge confidence must be explicitly recorded: `exact`, `ambiguous`, `name_match`, or `unknown`.
  * Surface caveats in tool responses when approximations occur.

---

## 4. Milestone 2: Change Intelligence Breakdown
Milestone 2 bridges the static code graph with Git development workflows. It is broken into small, sequential PRs:

1. **M2.1: Unified Git Diff Parser & AST Symbol Mapping (`pkg/diff`)**
   * Parse unified diff hunks and line ranges (`@@ -l,s +l,s @@`).
   * Intersect modified line ranges with AST node spans (`StartLine..EndLine`) in `pkg/store` to identify affected functions, methods, and classes.
2. **M2.2: Predictive Test Selection (`pkg/testselect`)**
   * Traverse `ReverseReach` from modified AST symbols to finding reaching test nodes (`is_test: true` or `test_` prefixes).
   * Return the minimal, ranked set of tests that execute changed paths.
3. **M2.3: Architectural Boundary Linter (`pkg/archlint`)**
   * Declarative layer boundary rules (e.g. `domain` cannot import `infra`).
   * Analyze new edges introduced in the diff and flag boundary violations before commit.
4. **M2.4: MCP Reasoning Integration**
   * Wire `diff_impact` and git-aware change intelligence into `pkg/mcp_server`.
   * End-to-end integration tests and progress board update.
