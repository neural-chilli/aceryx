package integration

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func openAndWaitForDB(ctx context.Context, dsn string, timeout time.Duration) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pingErr := db.PingContext(pingCtx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		if time.Now().After(deadline) {
			_ = db.Close()
			return nil, fmt.Errorf("ping db: %w", pingErr)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
