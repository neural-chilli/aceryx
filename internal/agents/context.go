package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

func (a *AgentExecutor) ResolveContext(ctx context.Context, tenantID, caseID uuid.UUID, caseData map[string]any, stepResults map[string]any, sources []ContextSource) (*AssembledContext, error) {
	ctxData := &AssembledContext{Case: map[string]any{}, Steps: map[string]any{}, Knowledge: []KnowledgeResult{}, Vault: []VaultDocResult{}, Meta: ContextSnapshotMeta{Sources: []SourceSnapshot{}}}
	if len(sources) == 0 {
		ctxData.Case = caseData
		ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "case", Bytes: approxJSONBytes(caseData)})
		ctxData.Meta.TotalSizeBytes = approxJSONBytes(caseData)
		return ctxData, nil
	}

	largestSource := ""
	largestBytes := 0
	totalBytes := 0

	for _, src := range sources {
		sourceCtx, cancel := context.WithTimeout(ctx, a.sourceTimeout)
		var (
			bytes int
			refs  []string
		)
		srcName := strings.TrimSpace(src.Source)
		switch srcName {
		case "case":
			resolved := caseData
			if len(src.Fields) > 0 {
				resolved = pickMapFields(caseData, src.Fields)
			}
			ctxData.Case = mergeMap(ctxData.Case, resolved)
			bytes = approxJSONBytes(resolved)
		case "steps":
			resolved := pickStepFields(stepResults, src.Fields)
			ctxData.Steps = mergeMap(ctxData.Steps, resolved)
			bytes = approxJSONBytes(resolved)
		case "knowledge":
			query := connectors.ResolveTemplateString(src.Query, map[string]any{"case": caseData, "steps": stepResults, "now": time.Now().UTC().Format(time.RFC3339)})
			topK := src.TopK
			if topK <= 0 {
				topK = defaultKnowledgeTopK
			}
			results, err := a.queryKnowledge(sourceCtx, tenantID, query, src.Collection, topK)
			cancel()
			if err != nil {
				return nil, err
			}
			ctxData.Knowledge = append(ctxData.Knowledge, results...)
			for _, item := range results {
				refs = append(refs, item.DocumentID.String())
			}
			bytes = approxJSONBytes(results)
			ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "knowledge", Bytes: bytes, Refs: refs})
			totalBytes += bytes
			if bytes > largestBytes {
				largestBytes = bytes
				largestSource = "knowledge"
			}
			if totalBytes > a.contextMaxBytes {
				return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
			}
			continue
		case "vault":
			results, err := a.queryVaultDocuments(sourceCtx, tenantID, caseID, src.DocumentTypes)
			cancel()
			if err != nil {
				return nil, err
			}
			ctxData.Vault = append(ctxData.Vault, results...)
			for _, item := range results {
				refs = append(refs, item.DocumentID.String())
			}
			bytes = approxJSONBytes(results)
			ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: "vault", Bytes: bytes, Refs: refs})
			totalBytes += bytes
			if bytes > largestBytes {
				largestBytes = bytes
				largestSource = "vault"
			}
			if totalBytes > a.contextMaxBytes {
				return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
			}
			continue
		default:
			cancel()
			return nil, fmt.Errorf("unsupported context source %q", src.Source)
		}
		cancel()

		totalBytes += bytes
		ctxData.Meta.Sources = append(ctxData.Meta.Sources, SourceSnapshot{Source: srcName, Bytes: bytes, Refs: refs})
		if bytes > largestBytes {
			largestBytes = bytes
			largestSource = srcName
		}
		if totalBytes > a.contextMaxBytes {
			return nil, fmt.Errorf("assembled context exceeds %d bytes; largest source: %s", a.contextMaxBytes, largestSource)
		}
	}
	ctxData.Meta.TotalSizeBytes = totalBytes
	return ctxData, nil
}

func (a *AgentExecutor) loadCaseAndSteps(ctx context.Context, caseID uuid.UUID) (uuid.UUID, string, string, map[string]any, map[string]any, error) {
	var (
		tenantID   uuid.UUID
		caseNumber string
		caseType   string
		caseDataB  []byte
	)
	err := a.db.QueryRowContext(ctx, `
SELECT c.tenant_id, c.case_number, ct.name, c.data
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.id = $1
`, caseID).Scan(&tenantID, &caseNumber, &caseType, &caseDataB)
	if err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("load case context: %w", err)
	}
	caseData := map[string]any{}
	if len(caseDataB) > 0 {
		if err := json.Unmarshal(caseDataB, &caseData); err != nil {
			return uuid.Nil, "", "", nil, nil, fmt.Errorf("decode case context data: %w", err)
		}
	}

	rows, err := a.db.QueryContext(ctx, `
SELECT step_id, COALESCE(result, '{}'::jsonb)
FROM case_steps
WHERE case_id = $1
`, caseID)
	if err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("load step context: %w", err)
	}
	defer func() { _ = rows.Close() }()

	steps := map[string]any{}
	for rows.Next() {
		var sid string
		var raw []byte
		if err := rows.Scan(&sid, &raw); err != nil {
			return uuid.Nil, "", "", nil, nil, fmt.Errorf("scan step context: %w", err)
		}
		decoded := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return uuid.Nil, "", "", nil, nil, fmt.Errorf("decode step context result %s: %w", sid, err)
			}
		}
		steps[sid] = map[string]any{"result": decoded}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, "", "", nil, nil, fmt.Errorf("iterate step context: %w", err)
	}
	return tenantID, caseNumber, caseType, caseData, steps, nil
}

func pickMapFields(src map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return cloneMap(src)
	}
	out := map[string]any{}
	for _, field := range fields {
		if v, ok := lookupDotPath(src, field); ok {
			setDotPath(out, field, v)
		}
	}
	return out
}

func pickStepFields(stepResults map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return cloneMap(stepResults)
	}
	out := map[string]any{}
	for _, field := range fields {
		parts := strings.Split(field, ".")
		if len(parts) == 0 {
			continue
		}
		stepID := parts[0]
		rest := strings.Join(parts[1:], ".")
		stepDataRaw, ok := stepResults[stepID]
		if !ok {
			continue
		}
		stepData, _ := stepDataRaw.(map[string]any)
		if rest == "" {
			out[stepID] = stepData
			continue
		}
		if v, ok := lookupDotPath(stepData, rest); ok {
			stepOut, _ := out[stepID].(map[string]any)
			if stepOut == nil {
				stepOut = map[string]any{}
				out[stepID] = stepOut
			}
			setDotPath(stepOut, rest, v)
		}
	}
	return out
}

func lookupDotPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := nextMap[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func setDotPath(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := root
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}

func mergeMap(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			existing, _ := dst[k].(map[string]any)
			dst[k] = mergeMap(existing, vMap)
			continue
		}
		dst[k] = v
	}
	return dst
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if mv, ok := v.(map[string]any); ok {
			out[k] = cloneMap(mv)
		} else {
			out[k] = v
		}
	}
	return out
}

func approxJSONBytes(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}
