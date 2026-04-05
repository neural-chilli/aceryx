package triggers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Checkpointer interface {
	Save(ctx context.Context, triggerID uuid.UUID, key, value string) error
	Load(ctx context.Context, triggerID uuid.UUID, key string) (string, error)
	DeleteAll(ctx context.Context, triggerID uuid.UUID) error
}

type CheckpointRecord struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CheckpointReader interface {
	List(ctx context.Context, triggerID uuid.UUID) ([]CheckpointRecord, error)
}

type PostgresCheckpointer struct {
	db *pgxpool.Pool
}

func NewPostgresCheckpointer(db *pgxpool.Pool) *PostgresCheckpointer {
	return &PostgresCheckpointer{db: db}
}

func (p *PostgresCheckpointer) Save(ctx context.Context, triggerID uuid.UUID, key, value string) error {
	if p == nil || p.db == nil {
		return nil
	}
	_, err := p.db.Exec(ctx, `
INSERT INTO trigger_checkpoints (trigger_id, checkpoint_key, checkpoint_value)
VALUES ($1, $2, $3)
ON CONFLICT (trigger_id, checkpoint_key)
DO UPDATE SET checkpoint_value = EXCLUDED.checkpoint_value, updated_at = now()
`, triggerID, key, value)
	if err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

func (p *PostgresCheckpointer) Load(ctx context.Context, triggerID uuid.UUID, key string) (string, error) {
	if p == nil || p.db == nil {
		return "", nil
	}
	var value string
	err := p.db.QueryRow(ctx, `
SELECT checkpoint_value
FROM trigger_checkpoints
WHERE trigger_id = $1 AND checkpoint_key = $2
`, triggerID, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (p *PostgresCheckpointer) List(ctx context.Context, triggerID uuid.UUID) ([]CheckpointRecord, error) {
	if p == nil || p.db == nil {
		return nil, nil
	}
	rows, err := p.db.Query(ctx, `
SELECT checkpoint_key, checkpoint_value
FROM trigger_checkpoints
WHERE trigger_id = $1
ORDER BY checkpoint_key
`, triggerID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()
	out := make([]CheckpointRecord, 0)
	for rows.Next() {
		var row CheckpointRecord
		if err := rows.Scan(&row.Key, &row.Value); err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoints: %w", err)
	}
	return out, nil
}

func (p *PostgresCheckpointer) DeleteAll(ctx context.Context, triggerID uuid.UUID) error {
	if p == nil || p.db == nil {
		return nil
	}
	_, err := p.db.Exec(ctx, `DELETE FROM trigger_checkpoints WHERE trigger_id = $1`, triggerID)
	if err != nil {
		return fmt.Errorf("delete checkpoints: %w", err)
	}
	return nil
}
