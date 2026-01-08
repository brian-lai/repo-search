You are my pair engineer. We are building a company-wide “Cursor-like” local indexing + hybrid retrieval tool for Claude Code via MCP.

Context
- Use the two markdown docs in this repo (in context/data/rough_plan/):
    1) “Company-Wide Claude Code Indexing Plan”
    2) “Phase 1 – Local Keyword Search MCP (repo-search)”
- Treat the company plan as the product spec, and the Phase 1 doc as the first implementation milestone.

Goals
- Engineers should be able to run a single command from any repo: `claude` (or a wrapper that execs `claude`) and get fast repo-aware retrieval.
- Local-first. No cloud dependencies required. Any semantic embeddings must be optional.
- Ship as a single Go binary `repo-search` (MCP stdio server) plus a CLI subcommand for indexing.
- Store per-repo state in `.repo_search/` and respect `.gitignore` + defaults.

Deliverables (execute in this repo)
PHASE 1 (must complete end-to-end)
- Create a Go project `repo-search/` with:
    - cmd/repo-search: MCP stdio server
    - internal/mcp: minimal JSON over stdio transport supporting `tools/list` and `tools/call`
    - internal/search/keyword: implement `search_keyword` backed by ripgrep (use `rg --json` if you can; otherwise basic `--line-number` parsing is acceptable for MVP)
    - internal/search/files: implement `get_file` with optional start/end line slicing
- Add `.mcp.json` at repo root to register the MCP server at project scope (for now OK to call `make mcp`).
- Add `bin/claude` wrapper that runs `make index` (no-op ok) and then `exec claude "$@"`.
- Add `Makefile` targets: build, mcp, index, doctor, clean.
- Add a README with setup and usage instructions.
- Add minimal tests for parsing and file slicing.

PHASE 2 (plan + stub after Phase 1 is working)
- Add symbol indexing via universal-ctags into SQLite (schema + indexer + `find_symbol` tool), but you may stub it in this pass if time.

Constraints
- Use modern CLI tooling; prefer “boring” solutions.
- Must work on macOS + Linux.
- Don’t introduce Docker/Milvus/Qdrant in Phase 1.
- Be careful about security: respect `.gitignore`, don’t index secrets, don’t execute arbitrary code.

Process
1) First, propose a short execution plan with file list and commands you’ll run.
2) Then implement Phase 1 end-to-end.
3) Run `make doctor`, `make mcp`, and a quick manual JSON request to verify `tools/list` and a `search_keyword` call.
4) Only after Phase 1 passes, outline Phase 2 work items (no deep implementation unless it’s quick).

Start now.