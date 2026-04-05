package invokers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type CaseStore interface {
	GetCaseData(ctx context.Context) (map[string]any, error)
	MergeCaseData(ctx context.Context, patch map[string]any) error
}

type CaseDataInvoker struct {
	caseStore CaseStore
	readOnly  bool
}

func NewCaseDataInvoker(caseStore CaseStore, readOnly bool) *CaseDataInvoker {
	return &CaseDataInvoker{caseStore: caseStore, readOnly: readOnly}
}

func (cdi *CaseDataInvoker) Invoke(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if cdi == nil || cdi.caseStore == nil {
		return nil, fmt.Errorf("case data invoker not configured")
	}
	var req struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("decode case data args: %w", err)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if req.Value == nil {
		data, err := cdi.caseStore.GetCaseData(ctx)
		if err != nil {
			return nil, err
		}
		value, _ := lookupPath(data, path)
		raw, _ := json.Marshal(map[string]any{"value": value})
		return raw, nil
	}
	if cdi.readOnly {
		return nil, fmt.Errorf("case data writes are disabled in read_only mode")
	}
	patch := map[string]any{}
	setPath(patch, path, req.Value)
	if err := cdi.caseStore.MergeCaseData(ctx, patch); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"status":"ok"}`), nil
}

func lookupPath(root map[string]any, path string) (any, bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	var cur any = root
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setPath(root map[string]any, path string, value any) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	cur := root
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
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
