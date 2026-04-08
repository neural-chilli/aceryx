# Spec 032 — RAG Infrastructure — Deep Gap Audit

- Spec file: `docs/specs/032-rag-infrastructure.md`
- Audit date: 2026-04-06
- Confidence: high (routes/handlers/rag stores/pipeline reviewed)

## Current Status
Strong partial implementation. Core KB CRUD, document pipeline, pgvector storage, and hybrid search are in place. Major gaps are around embedding/provider architecture parity, multi-store support, and full spec governance/audit semantics.

## Evidence Snapshot

### Implemented
- DB schema exists for `knowledge_bases`, `knowledge_documents`, `document_chunks` with indexes and tenant-aware columns (`migrations/011_rag_infrastructure.sql`).
- Full API surface from spec is routed under `/api/v1/knowledge-bases/...` including CRUD, documents, reindex, stats, and search (`api/routes.go`, `api/handlers/rag.go`).
- Ingestion pipeline stages are implemented with status transitions and background worker (`pending -> extracting -> chunking -> embedding -> ready/error`) (`internal/rag/pipeline.go`, `internal/rag/worker.go`).
- Chunking strategies exist (`fixed`, `recursive`, `semantic`, `sliding`) in splitter implementation.
- Hybrid search with RRF exists for pgvector store, with fallback behavior in search service (`internal/rag/store/pgvector.go`, `internal/rag/search.go`).
- Reindex endpoint includes cost estimate and confirmation guard.
- Stale embedding protection exists: model mismatch sets KB status to `stale_embeddings` and blocks uploads.
- RAG context source integration exists for agentic/tool paths (`internal/rag/step.go`, agentic rag invoker).

### Missing or Divergent From Spec
- Runtime uses `HashEmbedder` by default in router (`api/routes.go`) rather than LLM adapter embeddings per spec intent.
- Vector store implementations beyond pgvector are not present in runtime wiring (Pinecone/Weaviate/Qdrant/Milvus/Chroma listed in schema but not implemented).
- `chunkID` is not plain SHA-256 content-addressed ID as spec describes; implementation derives a UUID via SHA1 namespace from hashed payload.
- No explicit `embedding_version` field in chunk metadata contract/storage.
- Embedding compatibility checks currently focus on model mismatch; dimensions mismatch governance is not clearly enforced.
- Search-result audit logging (query, parameters, ranked result set) for workflow-triggered RAG searches is not evident in current RAG service path.
- Loader support is narrower than spec narrative: no OCR/image path currently visible.
- No frontend KB management/discovery UX found in `frontend/src`.

## DB

### Implemented
- Core RAG tables and indexes from spec are present and usable.

### Gaps
- `document_chunks.embedding` is fixed at `vector(1536)` in schema, while KB allows variable `embedding_dims`; that mismatch will constrain model changes.
- Spec-described embedding versioning metadata is not persisted explicitly.

## Backend

### Implemented
- Endpoint coverage is strong and tenant-aware.
- Worker + pipeline + search stack is production-shaped.

### Gaps
- Spec-aligned embedder/provider strategy is not the default path yet.
- External vector store backends are not implemented.
- Retrieval reproducibility/audit detail is incomplete.

## Frontend

### Implemented
- None evident for KB admin/search management.

### Gaps
- Missing UI for KB CRUD, uploads, indexing state, search testing, and reindex cost confirmation.

## AI Builder Understanding

### Implemented
- RAG is available in agentic tooling and context-source runtime paths.

### Gaps
- No builder-first configuration surfaces for KB-backed context sources; likely requires manual YAML knowledge.

## Workflow Usability Right Now
Backend-capable for API-driven RAG usage, but end-user workflow authoring/operations are hindered by absent frontend/admin surfaces and missing multi-backend embedding parity.

## Functional Completeness
Partially complete against spec 032.

## Intuitiveness
Low for operators (API-first), moderate for developers.

## Priority Actions

### P0
1. Replace default `HashEmbedder` wiring with LLM-adapter-based embedder resolution per tenant/KB config.
2. Resolve embedding dimension contract mismatch (schema/runtime) and enforce model+dims reindex rules.
3. Add retrieval audit logging payloads for workflow-triggered RAG searches (query params + result ranks/chunk IDs).
4. Add at least one additional non-pgvector backend or explicitly narrow spec/roadmap.

### P1
1. Add KB admin frontend (CRUD/upload/reindex/status/search diagnostics).
2. Add embedding version metadata (`embedding_version`) and expose in chunk/search payloads.
3. Add OCR/image ingestion path or reduce documented support claims.
