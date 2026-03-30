CREATE MATERIALIZED VIEW IF NOT EXISTS mv_report_cases AS
SELECT
    c.id AS case_id,
    c.tenant_id,
    c.case_number,
    ct.name AS case_type,
    c.status,
    c.priority,
    c.created_at,
    c.updated_at,
    p_creator.name AS created_by_name,
    p_assigned.name AS assigned_to_name,
    EXTRACT(EPOCH FROM (
        CASE WHEN c.status IN ('completed', 'cancelled')
        THEN c.updated_at ELSE now() END - c.created_at
    )) / 86400 AS age_days,
    c.status = 'completed' AS is_closed,
    c.status = 'cancelled' AS is_cancelled
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
LEFT JOIN principals p_creator ON p_creator.id = c.created_by
LEFT JOIN principals p_assigned ON p_assigned.id = c.assigned_to;

CREATE INDEX IF NOT EXISTS idx_mv_report_cases_tenant ON mv_report_cases(tenant_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_report_steps AS
SELECT
    cs.case_id,
    c.tenant_id,
    c.case_number,
    cs.step_id,
    cs.state,
    cs.started_at,
    cs.completed_at,
    cs.assigned_to,
    p.name AS assigned_to_name,
    cs.sla_deadline,
    cs.sla_deadline IS NOT NULL AND cs.completed_at IS NOT NULL
        AND cs.completed_at <= cs.sla_deadline AS sla_met,
    cs.sla_deadline IS NOT NULL AND cs.completed_at IS NOT NULL
        AND cs.completed_at > cs.sla_deadline AS sla_breached,
    EXTRACT(EPOCH FROM (
        COALESCE(cs.completed_at, now()) - COALESCE(cs.started_at, now())
    )) / 3600 AS duration_hours,
    cs.result->>'outcome' AS outcome
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
LEFT JOIN principals p ON p.id = cs.assigned_to;

CREATE INDEX IF NOT EXISTS idx_mv_report_steps_tenant ON mv_report_steps(tenant_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_report_tasks AS
SELECT
    cs.case_id,
    c.tenant_id,
    c.case_number,
    cs.step_id,
    cs.state,
    cs.assigned_to,
    p.name AS assigned_to_name,
    cs.started_at,
    cs.completed_at,
    cs.sla_deadline,
    CASE
        WHEN cs.state = 'completed' AND cs.sla_deadline IS NOT NULL AND cs.completed_at <= cs.sla_deadline THEN 'met'
        WHEN cs.state = 'completed' AND cs.sla_deadline IS NOT NULL AND cs.completed_at > cs.sla_deadline THEN 'breached'
        WHEN cs.state = 'active' AND cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now() THEN 'breached'
        WHEN cs.state = 'active' AND cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now() + interval '1 hour' THEN 'warning'
        WHEN cs.state = 'active' THEN 'on_track'
        ELSE 'n/a'
    END AS sla_status,
    EXTRACT(EPOCH FROM (
        COALESCE(cs.completed_at, now()) - COALESCE(cs.started_at, now())
    )) / 3600 AS duration_hours,
    cs.result->>'outcome' AS outcome
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
LEFT JOIN principals p ON p.id = cs.assigned_to
WHERE cs.metadata->>'role' IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mv_report_tasks_tenant ON mv_report_tasks(tenant_id);

CREATE TABLE IF NOT EXISTS report_view_schemas (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    view_name   TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    columns     JSONB NOT NULL
);

COMMENT ON COLUMN report_view_schemas.columns IS
'[{"name":"case_number","type":"text","description":"Human readable case number"}]';

INSERT INTO report_view_schemas (view_name, description, columns)
VALUES
('mv_report_cases', 'Flattened case reporting view with structured non-JSON columns only', '[
  {"name":"case_id","type":"uuid","description":"Internal case identifier"},
  {"name":"tenant_id","type":"uuid","description":"Owning tenant identifier"},
  {"name":"case_number","type":"text","description":"Human-readable case number"},
  {"name":"case_type","type":"text","description":"Case type name"},
  {"name":"status","type":"text","description":"Current case status"},
  {"name":"priority","type":"integer","description":"Case priority"},
  {"name":"created_at","type":"timestamptz","description":"Case creation time"},
  {"name":"updated_at","type":"timestamptz","description":"Last case update time"},
  {"name":"created_by_name","type":"text","description":"Case creator display name"},
  {"name":"assigned_to_name","type":"text","description":"Current assignee display name"},
  {"name":"age_days","type":"numeric","description":"Case age in days"},
  {"name":"is_closed","type":"boolean","description":"True when case is completed"},
  {"name":"is_cancelled","type":"boolean","description":"True when case is cancelled"}
]'),
('mv_report_steps', 'Step execution summary across all workflow steps', '[
  {"name":"case_id","type":"uuid","description":"Internal case identifier"},
  {"name":"tenant_id","type":"uuid","description":"Owning tenant identifier"},
  {"name":"case_number","type":"text","description":"Human-readable case number"},
  {"name":"step_id","type":"text","description":"Workflow step identifier"},
  {"name":"state","type":"text","description":"Current step state"},
  {"name":"started_at","type":"timestamptz","description":"Step start time"},
  {"name":"completed_at","type":"timestamptz","description":"Step completion time"},
  {"name":"assigned_to","type":"uuid","description":"Assigned principal ID"},
  {"name":"assigned_to_name","type":"text","description":"Assigned principal name"},
  {"name":"sla_deadline","type":"timestamptz","description":"SLA deadline"},
  {"name":"sla_met","type":"boolean","description":"True if step completed within SLA"},
  {"name":"sla_breached","type":"boolean","description":"True if step completed after SLA"},
  {"name":"duration_hours","type":"numeric","description":"Elapsed step duration in hours"},
  {"name":"outcome","type":"text","description":"Step outcome value"}
]'),
('mv_report_tasks', 'Human task performance reporting view', '[
  {"name":"case_id","type":"uuid","description":"Internal case identifier"},
  {"name":"tenant_id","type":"uuid","description":"Owning tenant identifier"},
  {"name":"case_number","type":"text","description":"Human-readable case number"},
  {"name":"step_id","type":"text","description":"Task step identifier"},
  {"name":"state","type":"text","description":"Task state"},
  {"name":"assigned_to","type":"uuid","description":"Assigned principal ID"},
  {"name":"assigned_to_name","type":"text","description":"Assigned principal name"},
  {"name":"started_at","type":"timestamptz","description":"Task start time"},
  {"name":"completed_at","type":"timestamptz","description":"Task completion time"},
  {"name":"sla_deadline","type":"timestamptz","description":"Task SLA deadline"},
  {"name":"sla_status","type":"text","description":"SLA status (met, warning, breached, n/a)"},
  {"name":"duration_hours","type":"numeric","description":"Task duration in hours"},
  {"name":"outcome","type":"text","description":"Task decision outcome"}
]')
ON CONFLICT (view_name) DO UPDATE
SET description = EXCLUDED.description,
    columns = EXCLUDED.columns;

CREATE TABLE IF NOT EXISTS saved_reports (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    created_by        UUID NOT NULL REFERENCES principals(id),
    name              TEXT NOT NULL,
    description       TEXT,
    original_question TEXT,
    query_sql         TEXT NOT NULL,
    visualisation     TEXT NOT NULL DEFAULT 'table'
                      CHECK (visualisation IN ('table', 'bar', 'line', 'pie', 'number')),
    columns           JSONB NOT NULL DEFAULT '[]',
    parameters        JSONB NOT NULL DEFAULT '{}',
    is_published      BOOLEAN NOT NULL DEFAULT false,
    pinned            BOOLEAN NOT NULL DEFAULT false,
    schedule          TEXT,
    recipients        JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sr_tenant ON saved_reports(tenant_id, is_published);
CREATE INDEX IF NOT EXISTS idx_sr_creator ON saved_reports(created_by);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aceryx_reporter') THEN
        CREATE ROLE aceryx_reporter NOLOGIN;
    END IF;
END
$$;

GRANT SELECT ON mv_report_cases TO aceryx_reporter;
GRANT SELECT ON mv_report_steps TO aceryx_reporter;
GRANT SELECT ON mv_report_tasks TO aceryx_reporter;
GRANT SELECT ON report_view_schemas TO aceryx_reporter;
GRANT SELECT ON saved_reports TO aceryx_reporter;
