CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_bases (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id),
    name                TEXT NOT NULL,
    description         TEXT,
    chunking_strategy   TEXT NOT NULL DEFAULT 'recursive'
                        CHECK (chunking_strategy IN ('fixed', 'semantic', 'recursive', 'sliding')),
    chunk_size          INTEGER NOT NULL DEFAULT 512,
    chunk_overlap       INTEGER NOT NULL DEFAULT 50,
    embedding_provider  TEXT NOT NULL DEFAULT 'openai',
    embedding_model     TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    embedding_dims      INTEGER NOT NULL DEFAULT 1536,
    vector_store        TEXT NOT NULL DEFAULT 'pgvector'
                        CHECK (vector_store IN ('pgvector', 'pinecone', 'weaviate', 'qdrant', 'milvus', 'chroma')),
    vector_store_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    document_count      INTEGER NOT NULL DEFAULT 0,
    chunk_count         INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'indexing', 'stale_embeddings', 'error')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

COMMENT ON COLUMN knowledge_bases.vector_store_config IS
'{"namespace":"string","index":"string"}';

CREATE TABLE IF NOT EXISTS knowledge_documents (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    vault_document_id UUID NOT NULL REFERENCES vault_documents(id),
    filename          TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    file_size         BIGINT,
    status            TEXT NOT NULL CHECK (status IN ('pending', 'extracting', 'chunking', 'embedding', 'ready', 'error')),
    chunk_count       INTEGER NOT NULL DEFAULT 0,
    error_message     TEXT,
    processing_ms     INTEGER,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_docs_kb ON knowledge_documents (knowledge_base_id);

CREATE TABLE IF NOT EXISTS document_chunks (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL,
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id       UUID NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    content           TEXT NOT NULL,
    token_count       INTEGER NOT NULL,
    metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding         vector(1536),
    embedding_model   TEXT NOT NULL,
    content_tsv       tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN document_chunks.metadata IS
'{"page_number":1,"section_title":"Policy","char_start":0,"char_end":100,"chunk_index":0,"strategy":"recursive"}';

CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON document_chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX IF NOT EXISTS idx_chunks_fts ON document_chunks USING gin(content_tsv);
CREATE INDEX IF NOT EXISTS idx_chunks_kb ON document_chunks (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_chunks_doc ON document_chunks (document_id);
CREATE INDEX IF NOT EXISTS idx_chunks_tenant ON document_chunks (tenant_id);
