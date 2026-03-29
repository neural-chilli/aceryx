package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func genesisHash(caseID uuid.UUID) string {
	sum := sha256.Sum256([]byte("aceryx:genesis:" + caseID.String()))
	return hex.EncodeToString(sum[:])
}

func eventHash(prev, eventType, actorID, action string, data json.RawMessage, createdAt time.Time) string {
	payload := prev + eventType + actorID + action + string(data) + createdAt.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// RecordCaseEventTx appends an audit event in case_events within an open transaction.
func RecordCaseEventTx(
	ctx context.Context,
	tx *sql.Tx,
	caseID uuid.UUID,
	stepID string,
	eventType string,
	actorID uuid.UUID,
	actorType string,
	action string,
	data map[string]interface{},
) error {
	var prev string
	err := tx.QueryRowContext(ctx, `
SELECT event_hash
FROM case_events
WHERE case_id = $1
ORDER BY created_at DESC
LIMIT 1
`, caseID).Scan(&prev)
	if err != nil {
		if err == sql.ErrNoRows {
			prev = genesisHash(caseID)
		} else {
			return fmt.Errorf("load previous event hash: %w", err)
		}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	now := time.Now().UTC()
	eh := eventHash(prev, eventType, actorID.String(), action, raw, now)

	if _, err := tx.ExecContext(ctx, `
INSERT INTO case_events (
    case_id, step_id, event_type, actor_id, actor_type, action, data, created_at, prev_event_hash, event_hash
) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
`, caseID, stepID, eventType, actorID, actorType, action, string(raw), now, prev, eh); err != nil {
		return fmt.Errorf("insert case event: %w", err)
	}
	return nil
}
