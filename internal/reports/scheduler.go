package reports

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *Service) RunDueScheduledReports(ctx context.Context) error {
	for {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var (
			reportID uuid.UUID
			tenantID uuid.UUID
		)
		err = tx.QueryRowContext(ctx, `
SELECT id, tenant_id
FROM saved_reports
WHERE schedule IN ('daily','weekly','monthly')
  AND (
      last_run_at IS NULL
      OR (schedule = 'daily' AND last_run_at < now() - interval '1 day')
      OR (schedule = 'weekly' AND last_run_at < now() - interval '7 day')
      OR (schedule = 'monthly' AND last_run_at < now() - interval '1 month')
  )
ORDER BY COALESCE(last_run_at, to_timestamp(0))
FOR UPDATE SKIP LOCKED
LIMIT 1
`).Scan(&reportID, &tenantID)
		if err != nil {
			_ = tx.Rollback()
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE saved_reports SET last_run_at = now(), updated_at = now() WHERE id = $1`, reportID); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		report, err := s.GetReport(ctx, tenantID, reportID)
		if err != nil {
			s.logger.Printf("reports schedule: load report failed id=%s err=%v", reportID, err)
			continue
		}
		rows, cols, err := s.ExecuteSQL(ctx, tenantID, report.QuerySQL)
		if err != nil {
			s.logger.Printf("reports schedule: execute failed id=%s err=%v", reportID, err)
			continue
		}
		_ = s.sendScheduleEmail(ctx, tenantID, report, rows, cols)
	}
}

func (s *Service) defaultScheduleEmail(_ context.Context, _ uuid.UUID, report SavedReport, _ []map[string]any, _ []string) error {
	s.logger.Printf("scheduled report executed: %s (%s)", report.Name, report.ID)
	return nil
}
