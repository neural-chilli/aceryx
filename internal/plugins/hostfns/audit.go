package hostfns

import (
	"encoding/json"
	"sync"
	"time"
)

type AuditMode string

const (
	AuditModeSummary AuditMode = "summary"
	AuditModeFull    AuditMode = "full"
	AuditModeSampled AuditMode = "sampled"
)

type AuditEntry struct {
	Function   string         `json:"function"`
	DurationMS int64          `json:"duration_ms"`
	Error      bool           `json:"error"`
	Args       map[string]any `json:"args,omitempty"`
}

type AuditSummary struct {
	Function      string `json:"function"`
	CallCount     int    `json:"call_count"`
	TotalDuration int64  `json:"total_duration_ms"`
	Errors        int    `json:"errors"`
}

type Auditor struct {
	mu         sync.Mutex
	mode       AuditMode
	maxEntries int
	sampleRate int
	calls      []AuditEntry
	summary    map[string]*AuditSummary
	count      int
}

func NewAuditor(mode string, maxEntries, sampleRate int) *Auditor {
	aMode := AuditMode(mode)
	if aMode != AuditModeFull && aMode != AuditModeSampled {
		aMode = AuditModeSummary
	}
	if maxEntries <= 0 {
		maxEntries = 50
	}
	if sampleRate <= 0 {
		sampleRate = 10
	}
	return &Auditor{
		mode:       aMode,
		maxEntries: maxEntries,
		sampleRate: sampleRate,
		summary:    map[string]*AuditSummary{},
	}
}

func (a *Auditor) Record(fn string, started time.Time, err error, args map[string]any) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	d := time.Since(started).Milliseconds()
	row := a.summary[fn]
	if row == nil {
		row = &AuditSummary{Function: fn}
		a.summary[fn] = row
	}
	row.CallCount++
	row.TotalDuration += d
	if err != nil {
		row.Errors++
	}
	a.count++

	include := false
	switch a.mode {
	case AuditModeFull:
		include = true
	case AuditModeSampled:
		include = a.count%a.sampleRate == 0
	}
	if include && len(a.calls) < a.maxEntries {
		a.calls = append(a.calls, AuditEntry{
			Function:   fn,
			DurationMS: d,
			Error:      err != nil,
			Args:       args,
		})
	}
}

func (a *Auditor) JSON() []byte {
	if a == nil {
		return []byte("[]")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	switch a.mode {
	case AuditModeFull:
		raw, _ := json.Marshal(a.calls)
		return raw
	default:
		items := make([]*AuditSummary, 0, len(a.summary))
		for _, row := range a.summary {
			items = append(items, row)
		}
		raw, _ := json.Marshal(items)
		return raw
	}
}
