# Plan: Installer Embedding Model Selection

**Date:** 2026-01-22
**Task:** Update installer to support recommended embedding models from documentation
**Type:** Feature Enhancement
**Complexity:** Low (single file modification)

---

## Context

We've just completed comprehensive research and documentation on embedding model selection (see `docs/embedding-model-comparison.md`). The documentation recommends three superior models over the current default:

1. **`bge-m3`** (Best overall) - +47% retrieval performance
2. **`snowflake-arctic-embed-l-v2.0`** (Highest retrieval) - +57% retrieval performance
3. **`jina-embeddings-v3`** (Best semantic similarity) - +50% retrieval performance

However, the installer (`install.sh`) still:
- Hard-codes `nomic-embed-text` as the only offered model
- Does not set `REPO_SEARCH_VECTOR_DIMENSIONS` environment variable
- Provides no guidance on model selection trade-offs

**Goal:** Align the installer with the documented best practices to provide users with an informed choice during setup.

---

## Files to Modify

1. **`install.sh`** (lines 280-428)
   - Section: "Step 3: Semantic Search Setup"
   - Specifically: Ollama model selection logic (lines 348-389)

---

## Detailed Implementation Plan

### Step 1: Design Model Selection Menu

**Location:** After line 311 (when user selects Ollama provider)

**Add interactive menu:**
```
Select an embedding model:

  1) bge-m3 (RECOMMENDED)
     • Best overall for code search (+47% vs nomic)
     • Dimensions: 1024, Memory: 2.2 GB
     • Multilingual support (100+ languages)

  2) snowflake-arctic-embed-l-v2.0
     • Highest retrieval quality (+57% vs nomic)
     • Dimensions: 1024, Memory: 2.2 GB
     • Optimized for English codebases

  3) jina-embeddings-v3
     • Best semantic similarity (+50% vs nomic)
     • Dimensions: 1024, Memory: 1.1 GB (smaller!)
     • Great for code relationships

  4) nomic-embed-text (legacy/backward compatibility)
     • Current default (baseline performance)
     • Dimensions: 768, Memory: 522 MB
     • Smallest footprint

Your choice [1]:
```

### Step 2: Map User Selection to Ollama Model Names

**Create mapping variables:**

| Choice | `EMBEDDING_MODEL` | `VECTOR_DIMENSIONS` | Ollama Pull Command |
|--------|-------------------|---------------------|---------------------|
| 1 | `bge-m3` | 1024 | `ollama pull bge-m3` |
| 2 | `snowflake-arctic-embed` | 1024 | `ollama pull snowflake-arctic-embed` |
| 3 | `jina-embeddings-v3` | 1024 | `ollama pull jina/jina-embeddings-v3` |
| 4 | `nomic-embed-text` | 768 | `ollama pull nomic-embed-text` |

**Note:** Handle the `jina/` prefix correctly in Ollama commands vs environment variables.

### Step 3: Update Model Availability Check

**Current logic (lines 354-374):**
- Only checks for `nomic-embed-text`
- Only offers to pull `nomic-embed-text`

**New logic:**
- Check for the selected model using `EMBEDDING_MODEL` variable
- Offer to pull the selected model with correct Ollama name
- Show download size estimate for transparency

**Size estimates:**
- `bge-m3`: ~2.2 GB
- `snowflake-arctic-embed`: ~2.2 GB
- `jina-embeddings-v3`: ~1.1 GB
- `nomic-embed-text`: ~274 MB

### Step 4: Add Vector Dimensions to Config

**Current config generation (lines 918-923):**
```bash
# Ollama configuration
export REPO_SEARCH_OLLAMA_URL="$OLLAMA_URL"
export REPO_SEARCH_EMBEDDING_MODEL="$EMBEDDING_MODEL"
```

**Update to:**
```bash
# Ollama configuration
export REPO_SEARCH_OLLAMA_URL="$OLLAMA_URL"
export REPO_SEARCH_EMBEDDING_MODEL="$EMBEDDING_MODEL"
export REPO_SEARCH_VECTOR_DIMENSIONS="$VECTOR_DIMENSIONS"
```

This is **critical** because:
- PostgreSQL pgvector tables are created with fixed dimensions
- Mismatch between model output (1024) and table schema (768) will cause errors
- Users upgrading from nomic need explicit dimensions set

### Step 5: Handle Custom Model Option

**Preserve existing functionality:**
- After the 4 predefined choices, allow "Other" option
- Prompt for custom model name
- Prompt for vector dimensions (default: 1024)
- Warn user that custom models are not validated

### Step 6: Update Success Messages

**After model pull succeeds:**
```
✓ bge-m3 model downloaded successfully (2.2 GB)
→ Vector dimensions: 1024
→ Expected performance: +47% better retrieval than nomic-embed-text
→ See docs/embedding-model-comparison.md for details
```

---

## Implementation Steps (Detailed)

### Modification Block 1: Model Selection Menu (after line 311)

```bash
# After: EMBEDDING_PROVIDER="ollama"
echo ""
print_section "Embedding Model Selection"

echo "Select an embedding model for code search:"
echo ""
echo -e "  ${GREEN}${BOLD}1)${NC} bge-m3 ${YELLOW}(RECOMMENDED)${NC}"
info "Best overall performance (+47% vs nomic)"
info "Dimensions: 1024, Memory: 2.2 GB, Context: 8K tokens"
echo ""
echo -e "  ${GREEN}${BOLD}2)${NC} snowflake-arctic-embed-l-v2.0"
info "Highest retrieval quality (+57% vs nomic)"
info "Dimensions: 1024, Memory: 2.2 GB, Context: 8K tokens"
echo ""
echo -e "  ${GREEN}${BOLD}3)${NC} jina-embeddings-v3"
info "Best semantic similarity (+50% vs nomic)"
info "Dimensions: 1024, Memory: 1.1 GB, Context: 8K tokens"
echo ""
echo -e "  ${GREEN}${BOLD}4)${NC} nomic-embed-text ${YELLOW}(legacy)${NC}"
info "Backward compatibility, smallest footprint"
info "Dimensions: 768, Memory: 522 MB, Context: 8K tokens"
echo ""
echo -e "  ${GREEN}${BOLD}5)${NC} Custom model"
info "Specify your own Ollama-compatible model"
echo ""
info "See docs/embedding-model-comparison.md for detailed comparison"
echo ""

read -p "$(prompt "Your choice [1]")" MODEL_CHOICE
MODEL_CHOICE=${MODEL_CHOICE:-1}

# Set model variables based on choice
case $MODEL_CHOICE in
    1)
        EMBEDDING_MODEL="bge-m3"
        OLLAMA_MODEL_NAME="bge-m3"
        VECTOR_DIMENSIONS="1024"
        MODEL_SIZE="2.2 GB"
        ;;
    2)
        EMBEDDING_MODEL="snowflake-arctic-embed"
        OLLAMA_MODEL_NAME="snowflake-arctic-embed"
        VECTOR_DIMENSIONS="1024"
        MODEL_SIZE="2.2 GB"
        ;;
    3)
        EMBEDDING_MODEL="jina-embeddings-v3"
        OLLAMA_MODEL_NAME="jina/jina-embeddings-v3"
        VECTOR_DIMENSIONS="1024"
        MODEL_SIZE="1.1 GB"
        ;;
    4)
        EMBEDDING_MODEL="nomic-embed-text"
        OLLAMA_MODEL_NAME="nomic-embed-text"
        VECTOR_DIMENSIONS="768"
        MODEL_SIZE="274 MB"
        ;;
    5)
        read -p "$(prompt "Enter Ollama model name")" EMBEDDING_MODEL
        OLLAMA_MODEL_NAME="$EMBEDDING_MODEL"
        read -p "$(prompt "Enter vector dimensions [1024]")" VECTOR_DIMENSIONS
        VECTOR_DIMENSIONS=${VECTOR_DIMENSIONS:-1024}
        MODEL_SIZE="unknown"
        warn "Custom models are not validated - ensure compatibility"
        ;;
    *)
        error "Invalid choice"
        exit 1
        ;;
esac

success "Selected: $EMBEDDING_MODEL (dimensions: $VECTOR_DIMENSIONS)"
```

### Modification Block 2: Update Model Check (replace lines 354-374)

```bash
# Check for selected model
if curl -s http://localhost:11434/api/tags | grep -q "$EMBEDDING_MODEL"; then
    success "$EMBEDDING_MODEL model is available"
else
    warn "$EMBEDDING_MODEL model not found"
    echo ""
    info "The $EMBEDDING_MODEL model is required for semantic search."
    info "Download size: $MODEL_SIZE"
    echo ""
    read -p "$(prompt "Pull $EMBEDDING_MODEL now? [Y/n]")" PULL_MODEL
    PULL_MODEL=${PULL_MODEL:-Y}
    if [[ $PULL_MODEL =~ ^[Yy] ]]; then
        echo ""
        info "Downloading model (this may take several minutes)..."
        if ollama pull "$OLLAMA_MODEL_NAME"; then
            success "Model downloaded successfully"
            success "Vector dimensions: $VECTOR_DIMENSIONS"
        else
            error "Failed to download model"
            warn "You can download it later with: ${BOLD}ollama pull $OLLAMA_MODEL_NAME${NC}"
        fi
    fi
fi
```

### Modification Block 3: Update Config Generation (add after line 922)

```bash
# Ollama configuration
export REPO_SEARCH_OLLAMA_URL="$OLLAMA_URL"
export REPO_SEARCH_EMBEDDING_MODEL="$EMBEDDING_MODEL"
export REPO_SEARCH_VECTOR_DIMENSIONS="$VECTOR_DIMENSIONS"  # ADD THIS LINE
```

### Modification Block 4: Remove Custom Model Prompt (lines 386-388)

**DELETE these lines (now handled by menu option 5):**
```bash
# Custom model?
read -p "$(prompt "Embedding model [nomic-embed-text]")" EMBEDDING_MODEL
EMBEDDING_MODEL=${EMBEDDING_MODEL:-nomic-embed-text}
```

---

## Edge Cases & Error Handling

### 1. Ollama Not Running
- Current handling is fine (warns user to start Ollama)
- Model check will gracefully fail
- User can pull model later

### 2. Model Already Exists
- Check passes silently ✓
- No duplicate download

### 3. Model Pull Fails
- Show error message
- Provide manual command
- Allow installation to continue (can pull later)

### 4. Invalid Custom Model
- Warn user before proceeding
- Installation continues (validated at runtime)

### 5. Port Conflicts
- Existing logic handles Ollama port issues
- No changes needed

---

## Testing Checklist

### Manual Testing Scenarios

1. **Fresh install with default (bge-m3)**
   - Select option 1
   - Verify model pulls successfully
   - Check config has `VECTOR_DIMENSIONS=1024`

2. **Fresh install with each recommended model**
   - Test options 2, 3
   - Verify correct Ollama model names used
   - Verify dimensions set correctly

3. **Fresh install with legacy (nomic)**
   - Select option 4
   - Verify `VECTOR_DIMENSIONS=768`
   - Should work for backward compatibility

4. **Custom model path**
   - Select option 5
   - Enter custom model name
   - Enter custom dimensions
   - Verify stored correctly in config

5. **Ollama not running**
   - Stop Ollama before install
   - Verify graceful handling
   - Check warning messages

6. **Model already downloaded**
   - Pre-pull `bge-m3`
   - Run installer
   - Should skip download

7. **Model download failure**
   - Simulate network failure
   - Verify error handling
   - Check manual command shown

### Config Validation

After installation, verify `~/.config/repo-search/config.env` contains:
```bash
export REPO_SEARCH_EMBEDDING_PROVIDER="ollama"
export REPO_SEARCH_OLLAMA_URL="http://localhost:11434"
export REPO_SEARCH_EMBEDDING_MODEL="bge-m3"  # or selected model
export REPO_SEARCH_VECTOR_DIMENSIONS="1024"  # or appropriate value
```

### Integration Testing

1. Run installer → select bge-m3
2. Run `repo-search embed .` in test repo
3. Verify embeddings table created with 1024 dimensions
4. Run `repo-search search "test query"`
5. Verify results returned successfully

---

## Risks & Mitigations

### Risk 1: Breaking Existing Installations
**Impact:** Medium
**Likelihood:** Low
**Mitigation:**
- Option 4 preserves `nomic-embed-text` default behavior
- Config changes are additive (new `VECTOR_DIMENSIONS` var)
- Existing configs without dimensions will use model defaults

### Risk 2: Large Model Downloads Fail
**Impact:** Low (can retry later)
**Likelihood:** Medium (network issues, disk space)
**Mitigation:**
- Show size estimates upfront
- Allow skipping download during install
- Provide manual pull command
- Model pull happens at embed time anyway

### Risk 3: Ollama Model Name Mismatch
**Impact:** High (model pull fails)
**Likelihood:** Low
**Mitigation:**
- Carefully map `EMBEDDING_MODEL` (env var) to `OLLAMA_MODEL_NAME` (pull command)
- Test each model option
- Special handling for `jina/jina-embeddings-v3` prefix

### Risk 4: Dimension Mismatch Errors
**Impact:** High (embedding fails)
**Likelihood:** Medium (if users have existing tables)
**Mitigation:**
- **This is the most critical risk**
- Users with existing SQLite/PostgreSQL embeddings tables will have dimension mismatch
- Solution: Document in migration guide (already exists in docs)
- Consider adding warning in installer: "Changing models requires re-embedding your codebase"

### Risk 5: User Confusion About Model Choice
**Impact:** Low (any choice works)
**Likelihood:** Medium
**Mitigation:**
- Clear menu descriptions
- Highlight recommended option
- Reference documentation
- Default to best option (bge-m3)

---

## Success Criteria

✅ **Functional Requirements:**
1. Installer presents 5 model options (3 recommended + 1 legacy + 1 custom)
2. Selected model is correctly pulled via Ollama
3. `REPO_SEARCH_VECTOR_DIMENSIONS` is set in config
4. Config file includes correct model name and dimensions
5. Backward compatibility maintained (nomic option works)

✅ **User Experience:**
1. Clear, concise model descriptions
2. Default to recommended option (bge-m3)
3. Show download size estimates
4. Link to documentation for details
5. Graceful error handling

✅ **Technical Correctness:**
1. Ollama model names are correct (especially jina/ prefix)
2. Vector dimensions match model outputs
3. Config variables are properly exported
4. No breaking changes to existing functionality

✅ **Documentation Alignment:**
1. Model recommendations match `docs/embedding-model-comparison.md`
2. Performance claims are accurate (+47%, +57%, +50%)
3. Specifications match (dimensions, memory, context length)

---

## Post-Implementation Validation

After implementation, verify:

1. **Run installer end-to-end** for each model option
2. **Check generated configs** have all required variables
3. **Test embedding generation** with each model
4. **Verify dimension consistency** between config and database schema
5. **Update documentation** if any discrepancies found

---

## Dependencies

- ✅ `docs/embedding-model-comparison.md` - Already complete
- ✅ Ollama models available in registry - Already published
- ✅ repo-search supports `REPO_SEARCH_VECTOR_DIMENSIONS` env var - Need to verify

**Action Item:** Check if `REPO_SEARCH_VECTOR_DIMENSIONS` environment variable is already implemented in the codebase.

---

## Estimated Effort

- **Planning:** 30 minutes ✅ (this document)
- **Implementation:** 45 minutes (modify install.sh)
- **Testing:** 30 minutes (test all 5 options)
- **Documentation Review:** 15 minutes (verify alignment)

**Total:** ~2 hours

---

## Related Work

- `docs/embedding-model-comparison.md` - Model research and recommendations
- `docs/installation.md` - May need updates to reflect new installer flow
- PR for embedding documentation (pending)

---

## Notes

- This is a **quality-of-life improvement** that helps users make informed choices
- **No breaking changes** - legacy option preserved
- **High user value** - +47-57% better search quality with one menu selection
- **Low risk** - single file modification, well-defined scope

---

**Ready to Execute:** ✅
**Blockers:** None
**Questions:** None - plan is complete and unambiguous