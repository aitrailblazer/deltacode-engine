---
trigger: always_on
always_on: true
description: DeltaCode AST Slicing and Token Conservation rule. Enforces structural context slicing before full file ingestion.
---

# DELTACODE AST CONTEXT SLICING & TOKEN CONSERVATION CONTRACT

Canonical implementation: [`cmd/deltacode`](../cmd/deltacode) |
[`pkg/astslicer`](../pkg/astslicer)

---

## 1. THE NON-NEGOTIABLE DELTACODE INVARIANT

> **"The expensive neural solver should not read the raw repository."**

1. **Mandatory AST Slicing**:
   Before reading, inspecting, or generating proposals for any file containing more than 100 lines of code (`.go`, `.py`), the agent MUST invoke the local Go AST slicer instead of calling `view_file` on the full file:
   ```bash
   go run ./cmd/deltacode slice \
     -root /path/to/repository \
     -path path/inside/repository.go \
     -target SymbolName
   ```
   Or use the root-bound MCP tool `slice_symbol`.

2. **Structural Folding Semantics**:
   - Preserves 100% of package declarations, imports, types, interfaces, and structs.
   - Preserves the target function/method in full.
   - Folds all non-target function bodies into minimal 1-line folded comments.
   - Reports measured byte and line reduction for each slice. Reduction depends
     on file structure and may be negative for very small files.

3. **Pure Go Systems Layer**:
   - Retrieval and slicing are implemented in Go and do not require a Python,
     Node.js, embedding-model, or database runtime.
   - The module requires a patched Go 1.25 toolchain (currently Go 1.25.13+).
