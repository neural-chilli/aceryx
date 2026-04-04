package cases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/engine"
	"github.com/neural-chilli/aceryx/internal/notify"
	"github.com/neural-chilli/aceryx/internal/observability"
)

func (s *CaseService) CreateCase(ctx context.Context, tenantID, createdBy uuid.UUID, req CreateCaseRequest) (Case, []ValidationError, error) {
	start := time.Now()
	defer func() {
		observability.DBQueryDurationSeconds.WithLabelValues("case_write").Observe(time.Since(start).Seconds())
	}()
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
INSERT INTO case_steps (case_id, step_id, state, result, events, error, retry_count, draft_data, metadata)
VALUES ($1, $2, 'pending', '{}'::jsonb, '[]'::jsonb, '{}'::jsonb, 0, '{}'::jsonb, '{}'::jsonb)
`, c.ID, stepID); err != nil {
			return Case{}, nil, fmt.Errorf("insert case step %s: %w", stepID, err)
		}
	}

	if err := s.audit.RecordCaseEventTx(ctx, tx, c.ID, "", "case", createdBy, "human", "created", map[string]interface{}{
		"case_number": c.CaseNumber,
		"case_type":   c.CaseType,
	}); err != nil {
		return Case{}, nil, err
	}

	if err := s.audit.CommitTx(tx); err != nil {
		return Case{}, nil, fmt.Errorf("commit create case tx: %w", err)
	}

	if s.engine != nil {
		_ = s.engine.EvaluateDAG(ctx, c.ID)
	}
	s.updateCaseStatusMetrics(ctx, tenantID)
	slog.InfoContext(ctx, "case created",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"case_id", c.ID.String(),
			"case_number", c.CaseNumber,
		)...,
	)

	return c, nil, nil
}

func (s *CaseService) GetCase(ctx context.Context, tenantID, caseID uuid.UUID) (Case, error) {
	start := time.Now()
	defer func() {
		observability.DBQueryDurationSeconds.WithLabelValues("case_read").Observe(time.Since(start).Seconds())
	}()
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
	defer func() { _ = s.audit.RollbackTx(tx) }()

	var (
		rawData   []byte
		rawSchema []byte
		version   int
	)
	err = tx.QueryRowContext(ctx, `
SELECT c.data, c.version, ct.schema
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1 AND c.id = $2
FOR UPDATE
`, tenantID, caseID).Scan(&rawData, &version, &rawSchema)
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
	if err := s.audit.RecordCaseEventTx(ctx, tx, caseID, "", "case", actorID, "human", "updated", map[string]interface{}{"diff": diff}); err != nil {
		return PatchResult{}, nil, err
	}

	if err := s.audit.CommitTx(tx); err != nil {
		return PatchResult{}, nil, fmt.Errorf("commit patch case tx: %w", err)
	}

	out, err := s.GetCase(ctx, tenantID, caseID)
	if err != nil {
		return PatchResult{}, nil, err
	}
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
	defer func() { _ = s.audit.RollbackTx(tx) }()

	if _, err := tx.ExecContext(ctx, `
UPDATE cases
SET status = 'completed', updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, caseID); err != nil {
		return fmt.Errorf("set case completed: %w", err)
	}

	if err := s.audit.RecordCaseEventTx(ctx, tx, caseID, "", "case", actorID, "human", "closed", map[string]interface{}{"reason": reason}); err != nil {
		return err
	}

	if err := s.audit.CommitTx(tx); err != nil {
		return err
	}
	s.updateCaseStatusMetrics(ctx, tenantID)
	slog.InfoContext(ctx, "case closed",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"case_id", caseID.String(),
			"actor_id", actorID.String(),
		)...,
	)
	if s.notify != nil {
		caseNumber, creator, creatorEmail, nerr := s.caseCreator(ctx, tenantID, caseID)
		if nerr == nil {
			_ = s.notify.Notify(ctx, notify.NotifyEvent{
				Type:       "case_completed",
				TenantID:   tenantID,
				CaseID:     caseID,
				CaseNumber: caseNumber,
				Recipients: []notify.Recipient{{PrincipalID: creator, Email: creatorEmail, Channels: []string{"email", "websocket"}}},
				Data:       map[string]any{"reason": reason},
			})
		}
	}
	return nil
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
	if err := s.engine.CancelCase(ctx, caseID, actorID, reason); err != nil {
		return err
	}
	s.updateCaseStatusMetrics(ctx, tenantID)
	slog.InfoContext(ctx, "case cancelled",
		append(observability.RequestAttrs(ctx),
			"tenant_id", tenantID.String(),
			"case_id", caseID.String(),
			"actor_id", actorID.String(),
		)...,
	)
	if s.notify != nil {
		caseNumber, recipients, nerr := s.caseCancellationRecipients(ctx, tenantID, caseID)
		if nerr == nil && len(recipients) > 0 {
			_ = s.notify.Notify(ctx, notify.NotifyEvent{
				Type:       "case_cancelled",
				TenantID:   tenantID,
				CaseID:     caseID,
				CaseNumber: caseNumber,
				Recipients: recipients,
				Data:       map[string]any{"reason": reason},
			})
		}
	}
	return nil
}

func (s *CaseService) updateCaseStatusMetrics(ctx context.Context, tenantID uuid.UUID) {
	statuses := []string{"open", "in_progress", "completed", "cancelled"}
	for _, status := range statuses {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE tenant_id = $1 AND status = $2`, tenantID, status).Scan(&count); err == nil {
			observability.CasesTotal.WithLabelValues(tenantID.String(), status).Set(float64(count))
		}
	}
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
