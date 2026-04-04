package reports

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) StartViewRefreshTicker(ctx context.Context) {
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.RefreshMaterializedViews(ctx); err != nil {
				s.logger.Printf("reports: refresh materialized views failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) StartScheduleTicker(ctx context.Context) {
	ticker := time.NewTicker(s.scheduleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.RunDueScheduledReports(ctx); err != nil {
				s.logger.Printf("reports: run due scheduled reports failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) RefreshMaterializedViews(ctx context.Context) error {
	for _, view := range []string{"mv_report_cases", "mv_report_steps", "mv_report_tasks"} {
		if _, err := s.db.ExecContext(ctx, `REFRESH MATERIALIZED VIEW `+view); err != nil {
			return fmt.Errorf("refresh %s: %w", view, err)
		}
	}
	return nil
}
