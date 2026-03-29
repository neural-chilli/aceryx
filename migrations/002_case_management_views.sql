CREATE MATERIALIZED VIEW IF NOT EXISTS mv_cases_summary AS
SELECT
    tenant_id,
    case_type_id,
    date_trunc('week', created_at) AS period,
    status,
    count(*) AS case_count,
    avg(EXTRACT(EPOCH FROM (updated_at - created_at)) / 86400.0) AS avg_days
FROM cases
GROUP BY tenant_id, case_type_id, date_trunc('week', created_at), status;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_sla_compliance AS
SELECT
    c.tenant_id,
    date_trunc('week', cs.completed_at) AS period,
    count(*) AS total,
    count(*) FILTER (WHERE cs.completed_at <= cs.sla_deadline) AS within_sla
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE cs.state = 'completed' AND cs.sla_deadline IS NOT NULL
GROUP BY c.tenant_id, date_trunc('week', cs.completed_at);

CREATE INDEX IF NOT EXISTS idx_mv_cases_summary_tenant_period ON mv_cases_summary (tenant_id, period);
CREATE INDEX IF NOT EXISTS idx_mv_sla_compliance_tenant_period ON mv_sla_compliance (tenant_id, period);
