package channels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DedupChecker struct{}

func NewDedupChecker() *DedupChecker {
	return &DedupChecker{}
}

func (dc *DedupChecker) Check(ctx context.Context, tx TxStore, tenantID, caseTypeID, channelID uuid.UUID, config DedupConfig, data json.RawMessage) (bool, *uuid.UUID, error) {
	strategy := strings.TrimSpace(strings.ToLower(config.Strategy))
	if strategy == "" || tx == nil {
		return false, nil, nil
	}

	window := 24 * time.Hour
	if config.WindowMins > 0 {
		window = time.Duration(config.WindowMins) * time.Minute
	}
	recent, err := tx.FindRecentEvents(ctx, tenantID, channelID, time.Now().Add(-window).UTC())
	if err != nil {
		return false, nil, err
	}

	switch strategy {
	case "field_hash":
		targetHash := dedupHash(config.Fields, data)
		if targetHash == "" {
			return false, nil, nil
		}
		for _, evt := range recent {
			if dedupHash(config.Fields, evt.RawPayload) == targetHash {
				return true, evt.CaseID, nil
			}
		}
		return false, nil, nil
	case "time_window":
		targetKey := dedupKey(config.Fields, data)
		if targetKey == "" {
			return false, nil, nil
		}
		for _, evt := range recent {
			if dedupKey(config.Fields, evt.RawPayload) == targetKey {
				return true, evt.CaseID, nil
			}
		}
		return false, nil, nil
	case "case_match":
		caseID, err := tx.FindCaseByFields(ctx, tenantID, caseTypeID, config.Fields, data)
		if err != nil {
			return false, nil, err
		}
		if caseID != nil {
			return false, caseID, nil
		}
		return false, nil, nil
	default:
		return false, nil, fmt.Errorf("unsupported dedup strategy: %s", strategy)
	}
}

func dedupHash(fields []string, raw json.RawMessage) string {
	if len(fields) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	pieces := make([]string, 0, len(fields))
	for _, field := range fields {
		if v, ok := lookupPath(payload, normalizePayloadPath(field)); ok {
			pieces = append(pieces, fmt.Sprint(v))
		}
	}
	if len(pieces) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(pieces, "|")))
	return hex.EncodeToString(sum[:])
}

func dedupKey(fields []string, raw json.RawMessage) string {
	if len(fields) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	pieces := make([]string, 0, len(fields))
	for _, field := range fields {
		if v, ok := lookupPath(payload, normalizePayloadPath(field)); ok {
			pieces = append(pieces, fmt.Sprint(v))
		}
	}
	return strings.Join(pieces, "|")
}
