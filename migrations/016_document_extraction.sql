CREATE TABLE IF NOT EXISTS extraction_schemas (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    description TEXT,
    fields      JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

COMMENT ON COLUMN extraction_schemas.fields IS
    '[{name,type,description,required,validation}]';

CREATE TABLE IF NOT EXISTS extraction_jobs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    case_id        UUID NOT NULL REFERENCES cases(id),
    step_id        TEXT NOT NULL,
    document_id    UUID NOT NULL REFERENCES vault_documents(id),
    schema_id      UUID NOT NULL REFERENCES extraction_schemas(id),
    model_used     TEXT NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'review', 'accepted', 'rejected')),
    confidence     NUMERIC(4,3),
    extracted_data JSONB,
    raw_response   JSONB,
    processing_ms  INTEGER,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN extraction_jobs.extracted_data IS
    'Structured extraction output JSON mapped by schema fields.';

COMMENT ON COLUMN extraction_jobs.raw_response IS
    'Raw provider response JSON payload for diagnostics.';

CREATE INDEX IF NOT EXISTS idx_extraction_jobs_case
    ON extraction_jobs (case_id);
CREATE INDEX IF NOT EXISTS idx_extraction_jobs_status
    ON extraction_jobs (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_extraction_jobs_schema
    ON extraction_jobs (schema_id);

CREATE TABLE IF NOT EXISTS extraction_fields (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES extraction_jobs(id) ON DELETE CASCADE,
    field_name      TEXT NOT NULL,
    extracted_value TEXT,
    confidence      NUMERIC(4,3) NOT NULL,
    source_text     TEXT,
    page_number     INTEGER,
    bbox_x          NUMERIC(6,4),
    bbox_y          NUMERIC(6,4),
    bbox_width      NUMERIC(6,4),
    bbox_height     NUMERIC(6,4),
    status          TEXT NOT NULL DEFAULT 'extracted'
                    CHECK (status IN ('extracted', 'confirmed', 'corrected', 'rejected')),
    corrected_value TEXT,
    CHECK (bbox_x IS NULL OR (bbox_x >= 0 AND bbox_x <= 1)),
    CHECK (bbox_y IS NULL OR (bbox_y >= 0 AND bbox_y <= 1)),
    CHECK (bbox_width IS NULL OR (bbox_width >= 0 AND bbox_width <= 1)),
    CHECK (bbox_height IS NULL OR (bbox_height >= 0 AND bbox_height <= 1))
);

CREATE INDEX IF NOT EXISTS idx_extraction_fields_job
    ON extraction_fields (job_id);

CREATE TABLE IF NOT EXISTS extraction_corrections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    job_id          UUID NOT NULL REFERENCES extraction_jobs(id) ON DELETE CASCADE,
    field_name      TEXT NOT NULL,
    original_value  TEXT,
    corrected_value TEXT,
    confidence      NUMERIC(4,3),
    model_used      TEXT,
    corrected_by    UUID NOT NULL REFERENCES principals(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_extraction_corrections_job
    ON extraction_corrections (job_id);
CREATE INDEX IF NOT EXISTS idx_extraction_corrections_schema
    ON extraction_corrections (tenant_id, created_at DESC);
