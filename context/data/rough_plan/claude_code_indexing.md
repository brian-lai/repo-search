# Company-Wide Claude Code Indexing Plan

This document defines a **single, durable solution** for improving Claude Code performance across the company by adding **local codebase indexing + hybrid retrieval** via MCP.

The goal is to give Claude Code **Cursor-like speed and grounding** with minimal friction for engineers.

---

## Executive Summary

We will ship a **single Go-based local tool** (`codetect`) that:

* Indexes a repository locally (incremental)
* Exposes retrieval capabilities to Claude Code via MCP (stdio)
* Requires **one command** for engineers: `claude`
* Degrades gracefully based on what tooling is available (ctags, Ollama, etc.)

This solution is:

* Local-first
* Secure by default
* Language-agnostic
* Easy to roll out company-wide

---

## Design Principles

1. **Local-first**: no cloud services required
2. **Single binary**: easy install, easy updates
3. **One-command UX**: `cd repo && claude`
4. **Graceful degradation**: features enable themselves when dependencies exist
5. **Incremental indexing**: fast startup, no full reindex unless needed
6. **Transparent**: engineers can inspect and debug behavior

---

## High-Level Architecture

```text
Engineer
  │
  │ claude
  ▼
Claude Code (local)
  │
  │ MCP (stdio)
  ▼
codetect (Go binary)
  │
  ├─ Keyword search (ripgrep)
  ├─ Symbol index (ctags → SQLite)
  ├─ Semantic index (optional: Ollama embeddings)
  └─ File retrieval
```

The cloud LLM never accesses the filesystem or index directly.

---

## Phased Implementation Plan

### Phase 0 — Spine (Required)

**Timeline:** immediate

Deliver the core harness that everything else builds on.

**Capabilities**

* MCP stdio server
* `search_keyword` (ripgrep)
* `get_file`
* Repo-scoped indexing directory (`.codetect/`)
* `.gitignore` + default ignores
* `bin/claude` wrapper
* `doctor` command

**Outcome**

* Immediate reduction in Claude latency and file thrashing
* Baseline improvement for all engineers

---

### Phase 2 — Symbol Indexing (Default On)

**Timeline:** next

Add structural understanding of the codebase.

**Implementation**

* Use `universal-ctags --output-format=json`
* Parse symbols into SQLite
* Incremental updates based on file mtime/hash

**New MCP Tools**

* `find_symbol(name)`
* `list_defs_in_file(path)`
* (optional) `find_references(name)`

**Outcome**

* Cursor-like navigation
* Faster answers to "where is this defined / used"
* Strong gains for Go, TS, Java, Python repos

---

### Phase 3 — Semantic Search (Optional, Pluggable)

**Timeline:** after Phase 2 stabilizes

Add conceptual retrieval when keywords/symbols aren’t enough.

**Implementation**

* Chunk code (function-level preferred)
* Generate embeddings via Ollama (local)
* Store vectors in SQLite (or optional Qdrant)

**New MCP Tools**

* `search_semantic(query)`
* `hybrid_search(query)` (keyword + symbol + semantic)

**Enablement Rules**

* Enabled automatically if Ollama is detected
* Disabled by default otherwise

**Outcome**

* Better answers to conceptual questions
* Strong support for large or poorly-named codebases

---

## Feature Matrix (End State)

| Feature           | Phase 0 | Phase 2 | Phase 3 |
| ----------------- | ------- | ------- | ------- |
| Keyword search    | ✅       | ✅       | ✅       |
| File retrieval    | ✅       | ✅       | ✅       |
| Symbol navigation | ❌       | ✅       | ✅       |
| Semantic search   | ❌       | ❌       | ✅       |
| Hybrid ranking    | ❌       | ⚠️      | ✅       |

---

## MCP Tool Contract (Stable)

These tool names and shapes are considered stable for company usage:

* `search_keyword`
* `get_file`
* `find_symbol`
* `search_semantic`
* `hybrid_search`

Backward compatibility will be preserved.

---

## Installation & Rollout Strategy

### Distribution

* Single Go binary
* Installed via:

  * Homebrew (preferred)
  * Internal artifact store
  * Curl installer fallback

### MCP Configuration

* User-scope MCP install (one-time)
* Project `.mcp.json` optional but supported

### Developer Experience

* No repo modification required
* Optional `bin/claude` wrapper provided
* Works with existing Claude Code installs

---

## Indexing Strategy

* Index stored in `.codetect/` (gitignored)
* Incremental updates using:

  * file mtime
  * file size
  * optional content hash
* Schema versioned for safe upgrades

---

## Security & Privacy

* No network access required
* No code sent to third parties
* Only retrieved snippets are passed to the LLM
* Secrets respected via `.gitignore`

---

## Success Criteria

We consider the project successful when:

* Engineers report Claude answers are faster and more grounded
* Claude stops repeatedly opening irrelevant files
* Most repos see benefit with zero config
* The tool is boring to maintain

---

## Non-Goals

* Replacing Cursor or IDEs
* Perfect semantic understanding
* Building a full language server

---

## Next Steps

1. Finalize Phase 0 + Phase 2 API
2. Implement symbol indexing
3. Pilot with a small group of engineers
4. Roll out company-wide
5. Evaluate Phase 3 adoption

