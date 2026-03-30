package cases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/engine"
)

type Engine interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
	CancelCase(ctx context.Context, caseID uuid.UUID, actorID uuid.UUID, reason string) error
}

type CaseTypeService struct {
	db *sql.DB
}

type CaseService struct {
	db     *sql.DB
	engine Engine
}

type ReportsService struct {
	db              *sql.DB
	refreshInterval time.Duration
}

func NewCaseTypeService(db *sql.DB) *CaseTypeService {
	return &CaseTypeService{db: db}
}

func NewCaseService(db *sql.DB, eng Engine) *CaseService {
	return &CaseService{db: db, engine: eng}
}

func NewReportsService(db *sql.DB, refreshInterval time.Duration) *ReportsService {
	if refreshInterval <= 0 {
		refreshInterval = 5 * time.Minute
	}
	return &ReportsService{db: db, refreshInterval: refreshInterval}
}

func (s *CaseTypeService) RegisterCaseType(ctx context.Context, tenantID, createdBy uuid.UUID, name string, schema CaseTypeSchema) (CaseType, []ValidationError, error) {
	if name == "" {
		return CaseType{}, nil, fmt.Errorf("case type name is required")
	}
	if errs := validateSchemaDefinition(schema); len(errs) > 0 {
		return CaseType{}, errs, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("begin register case type tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0) + 1
FROM case_types
WHERE tenant_id = $1 AND name = $2
`, tenantID, name).Scan(&nextVersion); err != nil {
		return CaseType{}, nil, fmt.Errorf("resolve next case type version: %w", err)
	}

	rawSchema, err := json.Marshal(schema)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("marshal case type schema: %w", err)
	}

	var ct CaseType
	err = tx.QueryRowContext(ctx, `
INSERT INTO case_types (tenant_id, name, version, schema, status, created_by)
VALUES ($1, $2, $3, $4::jsonb, 'active', $5)
RETURNING id, tenant_id, name, version, schema, status, created_at, created_by
`, tenantID, name, nextVersion, string(rawSchema), createdBy).Scan(
		&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &rawSchema, &ct.Status, &ct.CreatedAt, &ct.CreatedBy,
	)
	if err != nil {
		return CaseType{}, nil, fmt.Errorf("insert case type: %w", err)
	}
	if err := json.Unmarshal(rawSchema, &ct.Schema); err != nil {
		return CaseType{}, nil, fmt.Errorf("unmarshal case type schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CaseType{}, nil, fmt.Errorf("commit register case type tx: %w", err)
	}
	return ct, nil, nil
}

func (s *CaseTypeService) ListCaseTypes(ctx context.Context, tenantID uuid.UUID, includeArchived bool) ([]CaseType, error) {
	query := `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1
`
	if !includeArchived {
		query += ` AND status = 'active'`
	}
	query += ` ORDER BY name, version DESC`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list case types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseType, 0)
	for rows.Next() {
		var ct CaseType
		var raw []byte
		if err := rows.Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan case type row: %w", err)
		}
		if err := json.Unmarshal(raw, &ct.Schema); err != nil {
			return nil, fmt.Errorf("decode case type schema: %w", err)
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}

func (s *CaseTypeService) GetCaseTypeByID(ctx context.Context, tenantID, id uuid.UUID) (CaseType, error) {
	var ct CaseType
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1 AND id = $2
`, tenantID, id).Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CaseType{}, sql.ErrNoRows
		}
		return CaseType{}, fmt.Errorf("get case type: %w", err)
	}
	if err := json.Unmarshal(raw, &ct.Schema); err != nil {
		return CaseType{}, fmt.Errorf("decode case type schema: %w", err)
	}
	return ct, nil
}

func (s *CaseService) CreateCase(ctx context.Context, tenantID, createdBy uuid.UUID, req CreateCaseRequest) (Case, []ValidationError, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Case{}, nil, fmt.Errorf("begin create case tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ct, err := resolveLatestActiveCaseTypeTx(ctx, tx, tenantID, req.CaseType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Case{}, nil, fmt.Errorf("invalid case_type: %s", req.CaseType)
		}
		return Case{}, nil, err
	}

	validation := ValidateCaseData(ct.Schema, req.Data)
	if len(validation) > 0 {
		return Case{}, validation, nil
	}

	workflowID, workflowVersion, astRaw, err := resolveLatestPublishedWorkflowTx(ctx, tx, tenantID, ct.Name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Case{}, nil, fmt.Errorf("no published workflow for case type %s", ct.Name)
		}
		return Case{}, nil, err
	}

	caseNumber, err := generateCaseNumberTx(ctx, tx, tenantID, ct.Name)
	if err != nil {
		return Case{}, nil, err
	}

	rawData, err := json.Marshal(req.Data)
	if err != nil {
		return Case{}, nil, fmt.Errorf("marshal case data: %w", err)
	}

	var c Case
	err = tx.QueryRowContext(ctx, `
INSERT INTO cases (
    tenant_id, case_type_id, case_number, status, data, created_by, priority, workflow_id, workflow_version
) VALUES ($1, $2, $3, 'open', $4::jsonb, $5, $6, $7, $8)
RETURNING id, tenant_id, case_type_id, case_number, status, data, created_at, updated_at, created_by, assigned_to, due_at, priority, version, workflow_id, workflow_version
`, tenantID, ct.ID, caseNumber, string(rawData), createdBy, req.Priority, workflowID, workflowVersion).Scan(
		&c.ID, &c.TenantID, &c.CaseTypeID, &c.CaseNumber, &c.Status, &rawData, &c.CreatedAt, &c.UpdatedAt,
		&c.CreatedBy, &c.AssignedTo, &c.DueAt, &c.Priority, &c.Version, &c.WorkflowID, &c.WorkflowVersion,
	)
	if err != nil {
		return Case{}, nil, fmt.Errorf("insert case row: %w", err)
	}
	c.CaseType = ct.Name
	if err := json.Unmarshal(rawData, &c.Data); err != nil {
		return Case{}, nil, fmt.Errorf("decode case data: %w", err)
	}

	stepIDs, err := parseStepIDs(astRaw)
	if err != nil {
		return Case{}, nil, err
	}
	for _, stepID := range stepIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, events, retry_count)
VALUES ($1, $2, 'pending', '[]'::jsonb, 0)
`, c.ID, stepID); err != nil {
			return Case{}, nil, fmt.Errorf("insert case step %s: %w", stepID, err)
		}
	}

	if err := audit.RecordCaseEventTx(ctx, tx, c.ID, "", "case", createdBy, "human", "created", map[string]interface{}{
		"case_number": c.CaseNumber,
		"case_type":   c.CaseType,
	}); err != nil {
		return Case{}, nil, err
	}

	if err := audit.CommitTx(tx); err != nil {
		return Case{}, nil, fmt.Errorf("commit create case tx: %w", err)
	}

	if s.engine != nil {
		_ = s.engine.EvaluateDAG(ctx, c.ID)
	}

	return c, nil, nil
}

func (s *CaseService) GetCase(ctx context.Context, tenantID, caseID uuid.UUID) (Case, error) {
	var c Case
	var rawData []byte
	var ctName string
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.tenant_id, c.case_type_id, c.case_number, c.status, c.data, c.created_at, c.updated_at,
       c.created_by, c.assigned_to, c.due_at, c.priority, c.version, c.workflow_id, c.workflow_version, ct.name
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1 AND c.id = $2
`, tenantID, caseID).Scan(
		&c.ID, &c.TenantID, &c.CaseTypeID, &c.CaseNumber, &c.Status, &rawData,
		&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.AssignedTo, &c.DueAt, &c.Priority,
		&c.Version, &c.WorkflowID, &c.WorkflowVersion, &ctName,
	)
	if err != nil {
		return Case{}, err
	}
	c.CaseType = ctName
	_ = json.Unmarshal(rawData, &c.Data)

	steps, err := s.loadCaseSteps(ctx, caseID)
	if err != nil {
		return Case{}, err
	}
	c.Steps = steps

	events, err := s.loadCaseEvents(ctx, caseID)
	if err != nil {
		return Case{}, err
	}
	c.Events = events

	docs, err := s.loadCaseDocuments(ctx, caseID)
	if err != nil {
		return Case{}, err
	}
	c.Documents = docs

	return c, nil
}

func (s *CaseService) ListCases(ctx context.Context, tenantID uuid.UUID, filter ListCasesFilter) ([]Case, error) {
	page, perPage := normalizePage(filter.Page, filter.PerPage)
	query := `
SELECT c.id, c.tenant_id, c.case_type_id, c.case_number, c.status, c.data, c.created_at, c.updated_at,
       c.created_by, c.assigned_to, c.due_at, c.priority, c.version, c.workflow_id, c.workflow_version, ct.name
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
`
	args := []interface{}{tenantID}
	idx := 2

	if len(filter.Statuses) > 0 {
		query += fmt.Sprintf(" AND c.status = ANY($%d)", idx)
		args = append(args, pqStringArray(filter.Statuses))
		idx++
	}
	if filter.CaseType != "" {
		query += fmt.Sprintf(" AND ct.name = $%d", idx)
		args = append(args, filter.CaseType)
		idx++
	}
	if filter.AssignedNone {
		query += " AND c.assigned_to IS NULL"
	} else if filter.AssignedTo != nil {
		query += fmt.Sprintf(" AND c.assigned_to = $%d", idx)
		args = append(args, *filter.AssignedTo)
		idx++
	}
	if filter.Priority != nil {
		query += fmt.Sprintf(" AND c.priority >= $%d", idx)
		args = append(args, *filter.Priority)
		idx++
	}
	if filter.CreatedAfter != nil {
		query += fmt.Sprintf(" AND c.created_at >= $%d", idx)
		args = append(args, *filter.CreatedAfter)
		idx++
	}
	if filter.CreatedBefore != nil {
		query += fmt.Sprintf(" AND c.created_at <= $%d", idx)
		args = append(args, *filter.CreatedBefore)
		idx++
	}
	if filter.DueBefore != nil {
		query += fmt.Sprintf(" AND c.due_at <= $%d", idx)
		args = append(args, *filter.DueBefore)
		idx++
	}

	query += " ORDER BY " + safeCaseSort(filter.SortBy, filter.SortDir)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cases query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cases := make([]Case, 0)
	for rows.Next() {
		var c Case
		var raw []byte
		if err := rows.Scan(&c.ID, &c.TenantID, &c.CaseTypeID, &c.CaseNumber, &c.Status, &raw,
			&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.AssignedTo, &c.DueAt, &c.Priority, &c.Version,
			&c.WorkflowID, &c.WorkflowVersion, &c.CaseType); err != nil {
			return nil, fmt.Errorf("scan list case row: %w", err)
		}
		_ = json.Unmarshal(raw, &c.Data)
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

func (s *CaseService) UpdateCaseData(ctx context.Context, tenantID, caseID, actorID uuid.UUID, patch map[string]interface{}, expectedVersion int) (PatchResult, []ValidationError, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PatchResult{}, nil, fmt.Errorf("begin patch case tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

	var (
		rawData    []byte
		rawSchema  []byte
		version    int
		caseTypeID uuid.UUID
		caseType   string
	)
	err = tx.QueryRowContext(ctx, `
SELECT c.data, c.version, c.case_type_id, ct.name, ct.schema
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1 AND c.id = $2
FOR UPDATE
`, tenantID, caseID).Scan(&rawData, &version, &caseTypeID, &caseType, &rawSchema)
	if err != nil {
		return PatchResult{}, nil, err
	}

	if version != expectedVersion {
		return PatchResult{}, nil, engine.ErrCaseDataConflict
	}

	var before map[string]interface{}
	if err := json.Unmarshal(rawData, &before); err != nil {
		return PatchResult{}, nil, fmt.Errorf("decode existing case data: %w", err)
	}

	var schema CaseTypeSchema
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		return PatchResult{}, nil, fmt.Errorf("decode case schema: %w", err)
	}

	if forbidden := validateManualSourcePatch(schema, patch); len(forbidden) > 0 {
		return PatchResult{}, forbidden, nil
	}

	merged := DeepMerge(before, patch)
	validation := ValidateCaseData(schema, merged)
	if len(validation) > 0 {
		return PatchResult{}, validation, nil
	}

	rawMerged, _ := json.Marshal(merged)
	res, err := tx.ExecContext(ctx, `
UPDATE cases
SET data = $3::jsonb, version = version + 1, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND version = $4
`, tenantID, caseID, string(rawMerged), expectedVersion)
	if err != nil {
		return PatchResult{}, nil, fmt.Errorf("update case data: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return PatchResult{}, nil, engine.ErrCaseDataConflict
	}

	diff := ComputeFieldDiff(before, merged)
	if err := audit.RecordCaseEventTx(ctx, tx, caseID, "", "case", actorID, "human", "updated", map[string]interface{}{"diff": diff}); err != nil {
		return PatchResult{}, nil, err
	}

	if err := audit.CommitTx(tx); err != nil {
		return PatchResult{}, nil, fmt.Errorf("commit patch case tx: %w", err)
	}

	out, err := s.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return PatchResult{}, nil, err
	}

	_ = caseTypeID
	_ = caseType
	return PatchResult{Case: out, Diff: diff}, nil, nil
}

func (s *CaseService) CloseCase(ctx context.Context, tenantID, caseID, actorID uuid.UUID, reason string) error {
	var activeSteps int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM case_steps cs
JOIN cases c ON c.id = cs.case_id
WHERE c.tenant_id = $1 AND c.id = $2 AND cs.state = 'active'
`, tenantID, caseID).Scan(&activeSteps); err != nil {
		return fmt.Errorf("check active steps before close: %w", err)
	}
	if activeSteps > 0 {
		return fmt.Errorf("cannot close case with active steps: %d", activeSteps)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin close case tx: %w", err)
	}
	defer func() { _ = audit.RollbackTx(tx) }()

	if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET status = 'completed', updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, caseID); err != nil {
		return fmt.Errorf("set case completed: %w", err)
	}

	if err := audit.RecordCaseEventTx(ctx, tx, caseID, "", "case", actorID, "human", "closed", map[string]interface{}{"reason": reason}); err != nil {
		return err
	}

	return audit.CommitTx(tx)
}

func (s *CaseService) CancelCase(ctx context.Context, tenantID, caseID, actorID uuid.UUID, reason string) error {
	if s.engine == nil {
		return fmt.Errorf("engine is not configured")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id=$1 AND id=$2`, tenantID, caseID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	return s.engine.CancelCase(ctx, caseID, actorID, reason)
}

func (s *CaseService) SearchCases(ctx context.Context, tenantID uuid.UUID, allowedCaseTypeIDs []uuid.UUID, filter SearchFilter) ([]SearchResult, error) {
	page, perPage := normalizePage(filter.Page, filter.PerPage)

	query := `
SELECT c.id, c.case_number, ct.name, c.status,
       ts_headline('english', c.data::text, plainto_tsquery('english', $2)) AS headline,
       c.created_at, c.updated_at, c.version
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
  AND to_tsvector('english', c.data::text) @@ plainto_tsquery('english', $2)
`
	args := []interface{}{tenantID, filter.Query}
	idx := 3

	if len(allowedCaseTypeIDs) > 0 {
		query += fmt.Sprintf(" AND c.case_type_id = ANY($%d)", idx)
		args = append(args, pqUUIDArray(allowedCaseTypeIDs))
		idx++
	}
	if filter.CaseType != "" {
		query += fmt.Sprintf(" AND ct.name = $%d", idx)
		args = append(args, filter.CaseType)
		idx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND c.status = $%d", idx)
		args = append(args, filter.Status)
		idx++
	}

	query += fmt.Sprintf(" ORDER BY c.updated_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search cases query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SearchResult, 0)
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.CaseID, &r.CaseNumber, &r.CaseType, &r.Status, &r.Headline, &r.CreatedAt, &r.UpdatedAt, &r.CaseVersion); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *CaseService) Dashboard(ctx context.Context, tenantID uuid.UUID, filter DashboardFilter) ([]DashboardRow, error) {
	page, perPage := normalizePage(filter.Page, filter.PerPage)

	query := `
SELECT c.id, c.case_number, ct.name, c.status, c.assigned_to, c.priority, c.created_at, c.updated_at,
       COALESCE(
           CASE
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) IS NULL THEN 'n/a'
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() THEN 'breached'
             WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() + interval '24 hour' THEN 'warning'
             ELSE 'on_track'
           END,
           'n/a'
       ) AS sla_status,
       COALESCE(MAX(cs.step_id) FILTER (WHERE cs.state = 'active'), '') AS current_step
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
LEFT JOIN case_steps cs ON cs.case_id = c.id
WHERE c.tenant_id = $1
`
	args := []interface{}{tenantID}
	idx := 2

	if len(filter.Statuses) > 0 {
		query += fmt.Sprintf(" AND c.status = ANY($%d)", idx)
		args = append(args, pqStringArray(filter.Statuses))
		idx++
	}
	if filter.CaseType != "" {
		query += fmt.Sprintf(" AND ct.name = $%d", idx)
		args = append(args, filter.CaseType)
		idx++
	}
	if filter.AssignedNone {
		query += " AND c.assigned_to IS NULL"
	} else if filter.AssignedTo != nil {
		query += fmt.Sprintf(" AND c.assigned_to = $%d", idx)
		args = append(args, *filter.AssignedTo)
		idx++
	}
	if filter.OlderThanDays != nil {
		query += fmt.Sprintf(" AND c.created_at < now() - ($%d || ' days')::interval", idx)
		args = append(args, strconv.Itoa(*filter.OlderThanDays))
		idx++
	}
	if filter.Priority != nil {
		query += fmt.Sprintf(" AND c.priority >= $%d", idx)
		args = append(args, *filter.Priority)
		idx++
	}
	if filter.CreatedAfter != nil {
		query += fmt.Sprintf(" AND c.created_at >= $%d", idx)
		args = append(args, *filter.CreatedAfter)
		idx++
	}
	if filter.CreatedBefore != nil {
		query += fmt.Sprintf(" AND c.created_at <= $%d", idx)
		args = append(args, *filter.CreatedBefore)
		idx++
	}

	query += " GROUP BY c.id, ct.name"
	if filter.SLAStatus != "" {
		query += fmt.Sprintf(" HAVING COALESCE(CASE WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) IS NULL THEN 'n/a' WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() THEN 'breached' WHEN MIN(cs.sla_deadline) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL) < now() + interval '24 hour' THEN 'warning' ELSE 'on_track' END, 'n/a') = $%d", idx)
		args = append(args, filter.SLAStatus)
		idx++
	}

	query += " ORDER BY " + safeDashboardSort(filter.SortBy, filter.SortDir)
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DashboardRow, 0)
	for rows.Next() {
		var row DashboardRow
		if err := rows.Scan(&row.CaseID, &row.CaseNumber, &row.CaseType, &row.Status, &row.AssignedTo, &row.Priority, &row.CreatedAt, &row.UpdatedAt, &row.SLAStatus, &row.CurrentStep); err != nil {
			return nil, fmt.Errorf("scan dashboard row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) StartRefreshTicker(ctx context.Context) {
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = r.RefreshMaterializedViews(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *ReportsService) RefreshMaterializedViews(ctx context.Context) error {
	for _, view := range []string{"mv_cases_summary", "mv_sla_compliance"} {
		if _, err := r.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW `+view); err != nil {
			return fmt.Errorf("refresh materialized view %s: %w", view, err)
		}
	}
	return nil
}

func (r *ReportsService) CasesSummary(ctx context.Context, tenantID uuid.UUID, weeks int) ([]CasesSummaryRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT period, period + interval '6 day' AS period_end,
       SUM(case_count) FILTER (WHERE status='open') AS opened,
       SUM(case_count) FILTER (WHERE status='completed') AS closed,
       SUM(case_count) FILTER (WHERE status='cancelled') AS cancelled,
       COALESCE(AVG(avg_days), 0)
FROM mv_cases_summary
WHERE tenant_id = $1 AND period >= date_trunc('week', now() - ($2 || ' week')::interval)
GROUP BY period
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]CasesSummaryRow, 0)
	for rows.Next() {
		var row CasesSummaryRow
		if err := rows.Scan(&row.PeriodStart, &row.PeriodEnd, &row.Opened, &row.Closed, &row.Cancelled, &row.AvgDaysToClose); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Ageing(ctx context.Context, tenantID uuid.UUID, thresholds []int) ([]AgeingBracket, error) {
	if len(thresholds) == 0 {
		thresholds = []int{7, 14, 30}
	}
	sort.Ints(thresholds)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, created_at
FROM cases
WHERE tenant_id = $1 AND status IN ('open', 'in_progress')
`, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	b := make([]AgeingBracket, 0, len(thresholds)+1)
	for i := 0; i < len(thresholds); i++ {
		if i == 0 {
			b = append(b, AgeingBracket{Label: "0-" + strconv.Itoa(thresholds[i]) + " days"})
		} else {
			b = append(b, AgeingBracket{Label: strconv.Itoa(thresholds[i-1]) + "-" + strconv.Itoa(thresholds[i]) + " days"})
		}
	}
	b = append(b, AgeingBracket{Label: strconv.Itoa(thresholds[len(thresholds)-1]) + "+ days"})

	now := time.Now()
	for rows.Next() {
		var id uuid.UUID
		var created time.Time
		if err := rows.Scan(&id, &created); err != nil {
			return nil, err
		}
		days := int(now.Sub(created).Hours() / 24)
		idx := len(b) - 1
		for i, th := range thresholds {
			if days <= th {
				idx = i
				break
			}
		}
		b[idx].Count++
		b[idx].CaseIDs = append(b[idx].CaseIDs, id)
	}
	return b, rows.Err()
}

func (r *ReportsService) SLACompliance(ctx context.Context, tenantID uuid.UUID, weeks int) ([]SLAComplianceRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT period, total, within_sla
FROM mv_sla_compliance
WHERE tenant_id = $1 AND period >= date_trunc('week', now() - ($2 || ' week')::interval)
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]SLAComplianceRow, 0)
	for rows.Next() {
		var row SLAComplianceRow
		if err := rows.Scan(&row.PeriodStart, &row.TotalTasks, &row.CompletedWithinSLA); err != nil {
			return nil, err
		}
		if row.TotalTasks > 0 {
			row.ComplianceRate = float64(row.CompletedWithinSLA) / float64(row.TotalTasks)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) CasesByStage(ctx context.Context, tenantID uuid.UUID, caseType string) ([]StageRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT cs.step_id, COUNT(*)
FROM cases c
JOIN case_steps cs ON cs.case_id = c.id
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1
  AND c.status IN ('open', 'in_progress')
  AND cs.state = 'active'
  AND ($2 = '' OR ct.name = $2)
GROUP BY cs.step_id
ORDER BY cs.step_id
`, tenantID, caseType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]StageRow, 0)
	for rows.Next() {
		var row StageRow
		if err := rows.Scan(&row.Stage, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Workload(ctx context.Context, tenantID uuid.UUID) ([]WorkloadRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT p.id, p.name,
       COUNT(*) FILTER (WHERE cs.state = 'active') AS active_tasks,
       COUNT(*) FILTER (WHERE cs.state = 'active' AND cs.sla_deadline IS NOT NULL AND cs.sla_deadline < now()) AS breached_sla
FROM principals p
LEFT JOIN case_steps cs ON cs.assigned_to = p.id
WHERE p.tenant_id = $1
GROUP BY p.id, p.name
ORDER BY p.name
`, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WorkloadRow, 0)
	for rows.Next() {
		var row WorkloadRow
		if err := rows.Scan(&row.PrincipalID, &row.Name, &row.ActiveTasks, &row.BreachedSLA); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ReportsService) Decisions(ctx context.Context, tenantID uuid.UUID, weeks int) ([]DecisionRow, error) {
	if weeks <= 0 {
		weeks = 12
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT date_trunc('week', created_at) AS period,
       COUNT(*) FILTER (WHERE actor_type = 'agent' AND ((event_type = 'agent' AND action = 'completed') OR (event_type = 'case' AND action = 'updated'))),
       COUNT(*) FILTER (WHERE actor_type = 'human' AND ((event_type = 'task' AND action = 'completed') OR (event_type = 'case' AND action = 'updated'))),
       COUNT(*) FILTER (WHERE event_type = 'agent' AND action = 'escalated')
FROM case_events ce
JOIN cases c ON c.id = ce.case_id
WHERE c.tenant_id = $1 AND ce.created_at >= date_trunc('week', now() - ($2 || ' week')::interval)
GROUP BY date_trunc('week', created_at)
ORDER BY period
`, tenantID, weeks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]DecisionRow, 0)
	for rows.Next() {
		var row DecisionRow
		if err := rows.Scan(&row.PeriodStart, &row.AgentDecisions, &row.HumanDecisions, &row.AgentEscalations); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func validateSchemaDefinition(schema CaseTypeSchema) []ValidationError {
	errs := make([]ValidationError, 0)
	for field, def := range schema.Fields {
		errs = append(errs, validateSchemaField(field, def)...)
	}
	return errs
}

func validateSchemaField(path string, def SchemaField) []ValidationError {
	validTypes := map[string]bool{"string": true, "number": true, "integer": true, "boolean": true, "object": true, "array": true, "text": true}
	errs := make([]ValidationError, 0)
	if def.Type == "" || !validTypes[def.Type] {
		errs = append(errs, ValidationError{Field: path, Rule: "type", Message: "unsupported field type in schema"})
	}
	for k, child := range def.Properties {
		errs = append(errs, validateSchemaField(path+"."+k, child)...)
	}
	return errs
}

func resolveLatestActiveCaseTypeTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, name string) (CaseType, error) {
	var ct CaseType
	var raw []byte
	err := tx.QueryRowContext(ctx, `
SELECT id, tenant_id, name, version, schema, status, created_at, created_by
FROM case_types
WHERE tenant_id = $1 AND name = $2 AND status = 'active'
ORDER BY version DESC
LIMIT 1
`, tenantID, name).Scan(&ct.ID, &ct.TenantID, &ct.Name, &ct.Version, &raw, &ct.Status, &ct.CreatedAt, &ct.CreatedBy)
	if err != nil {
		return CaseType{}, err
	}
	if err := json.Unmarshal(raw, &ct.Schema); err != nil {
		return CaseType{}, err
	}
	return ct, nil
}

func resolveLatestPublishedWorkflowTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, caseType string) (uuid.UUID, int, []byte, error) {
	var workflowID uuid.UUID
	var version int
	var ast []byte
	err := tx.QueryRowContext(ctx, `
SELECT w.id, wv.version, wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.tenant_id = $1
  AND w.case_type = $2
  AND wv.status = 'published'
ORDER BY wv.version DESC
LIMIT 1
`, tenantID, caseType).Scan(&workflowID, &version, &ast)
	if err != nil {
		return uuid.Nil, 0, nil, err
	}
	return workflowID, version, ast, nil
}

func parseStepIDs(astRaw []byte) ([]string, error) {
	var ast struct {
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(astRaw, &ast); err != nil {
		return nil, fmt.Errorf("decode workflow ast: %w", err)
	}
	out := make([]string, 0, len(ast.Steps))
	for _, st := range ast.Steps {
		if st.ID != "" {
			out = append(out, st.ID)
		}
	}
	return out, nil
}

func generateCaseNumberTx(ctx context.Context, tx *sql.Tx, tenantID uuid.UUID, caseType string) (string, error) {
	prefix := formatCasePrefix(caseType)
	var next int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO case_number_sequences (tenant_id, case_type, last_number)
VALUES ($1, $2, 1)
ON CONFLICT (tenant_id, case_type)
DO UPDATE SET last_number = case_number_sequences.last_number + 1
RETURNING last_number
`, tenantID, caseType).Scan(&next)
	if err != nil {
		return "", fmt.Errorf("generate case number sequence: %w", err)
	}
	return fmt.Sprintf("%s-%06d", prefix, next), nil
}

func formatCasePrefix(caseType string) string {
	parts := strings.Split(caseType, "_")
	if len(parts) == 1 {
		parts = strings.Split(caseType, "-")
	}
	if len(parts) == 1 {
		parts = strings.Fields(caseType)
	}
	prefix := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		r := []rune(strings.ToUpper(p))
		prefix += string(r[0])
	}
	if prefix == "" {
		clean := regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(strings.ToUpper(caseType), "")
		if len(clean) >= 4 {
			prefix = clean[:4]
		} else {
			prefix = clean
		}
	}
	if len(prefix) < 2 {
		prefix = strings.ToUpper(caseType)
	}
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return prefix
}

func safeCaseSort(sortBy, sortDir string) string {
	allowed := map[string]string{
		"created_at":  "c.created_at",
		"updated_at":  "c.updated_at",
		"priority":    "c.priority",
		"due_at":      "c.due_at",
		"case_number": "c.case_number",
		"status":      "c.status",
	}
	col, ok := allowed[sortBy]
	if !ok {
		col = "c.updated_at"
	}
	dir := strings.ToUpper(sortDir)
	if dir != "ASC" {
		dir = "DESC"
	}
	return col + " " + dir
}

func safeDashboardSort(sortBy, sortDir string) string {
	allowed := map[string]string{
		"case_number": "c.case_number",
		"created_at":  "c.created_at",
		"updated_at":  "c.updated_at",
		"priority":    "c.priority",
		"status":      "c.status",
	}
	col, ok := allowed[sortBy]
	if !ok {
		col = "c.updated_at"
	}
	dir := strings.ToUpper(sortDir)
	if dir != "ASC" {
		dir = "DESC"
	}
	return col + " " + dir
}

func normalizePage(page, perPage int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func validateManualSourcePatch(schema CaseTypeSchema, patch map[string]interface{}) []ValidationError {
	flat := map[string]interface{}{}
	flattenMap("", patch, flat)
	errs := make([]ValidationError, 0)
	for path := range flat {
		if path == "" {
			continue
		}
		if field, ok := resolveSchemaField(schema, path); ok {
			source := strings.ToLower(field.Source)
			if source == "agent" || source == "external" {
				errs = append(errs, ValidationError{Field: path, Rule: "source", Message: "field is not manually editable"})
			}
		}
	}
	return errs
}

func resolveSchemaField(schema CaseTypeSchema, path string) (SchemaField, bool) {
	parts := strings.Split(path, ".")
	cur, ok := schema.Fields[parts[0]]
	if !ok {
		return SchemaField{}, false
	}
	for i := 1; i < len(parts); i++ {
		if cur.Type != "object" {
			return SchemaField{}, false
		}
		next, ok := cur.Properties[parts[i]]
		if !ok {
			return SchemaField{}, false
		}
		cur = next
	}
	return cur, true
}

func (s *CaseService) loadCaseSteps(ctx context.Context, caseID uuid.UUID) ([]CaseStep, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, step_id, state, started_at, completed_at, result, events, error, assigned_to, sla_deadline, retry_count, draft_data, metadata
FROM case_steps
WHERE case_id = $1
ORDER BY step_id
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseStep, 0)
	for rows.Next() {
		var step CaseStep
		if err := rows.Scan(&step.ID, &step.StepID, &step.State, &step.StartedAt, &step.CompletedAt,
			&step.Result, &step.Events, &step.Error, &step.AssignedTo, &step.SLADeadline,
			&step.RetryCount, &step.DraftData, &step.Metadata); err != nil {
			return nil, fmt.Errorf("scan case step: %w", err)
		}
		out = append(out, step)
	}
	return out, rows.Err()
}

func (s *CaseService) loadCaseEvents(ctx context.Context, caseID uuid.UUID) ([]CaseEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, COALESCE(step_id, ''), event_type, actor_id, actor_type, action, data, created_at, prev_event_hash, event_hash
FROM case_events
WHERE case_id = $1
ORDER BY created_at
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseEvent, 0)
	for rows.Next() {
		var (
			event CaseEvent
			raw   []byte
		)
		if err := rows.Scan(&event.ID, &event.StepID, &event.EventType, &event.ActorID, &event.ActorType, &event.Action, &raw, &event.CreatedAt, &event.PrevEventHash, &event.EventHash); err != nil {
			return nil, fmt.Errorf("scan case event: %w", err)
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &event.Data)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *CaseService) loadCaseDocuments(ctx context.Context, caseID uuid.UUID) ([]CaseDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, filename, mime_type, size_bytes, uploaded_by, uploaded_at, deleted_at
FROM vault_documents
WHERE case_id = $1
ORDER BY uploaded_at DESC
`, caseID)
	if err != nil {
		return nil, fmt.Errorf("load case documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseDocument, 0)
	for rows.Next() {
		var d CaseDocument
		if err := rows.Scan(&d.ID, &d.Filename, &d.MimeType, &d.SizeBytes, &d.UploadedBy, &d.UploadedAt, &d.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan case document: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type pqStringArray []string

type pqUUIDArray []uuid.UUID
