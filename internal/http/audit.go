package httpfw

// HTTPAuditEntry captures non-sensitive outbound HTTP call metadata.
type HTTPAuditEntry struct {
	URL        string
	Method     string
	StatusCode int
	DurationMS int
	Truncated  bool
	Error      string
}
