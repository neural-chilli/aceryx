package observability

import (
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_http_requests_total", Help: "Total HTTP requests by method/path/status"},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aceryx_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	CasesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "aceryx_cases_total", Help: "Cases by tenant and status"},
		[]string{"tenant_id", "status"},
	)
	CaseStepsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "aceryx_case_steps_total", Help: "Case steps by tenant and state"},
		[]string{"tenant_id", "state"},
	)
	DAGEvaluationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_dag_evaluations_total", Help: "Total DAG evaluations"},
		[]string{"tenant_id"},
	)
	DAGEvaluationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "aceryx_dag_evaluation_duration_seconds", Help: "DAG evaluation duration in seconds"},
		[]string{"tenant_id"},
	)
	StepExecutionDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "aceryx_step_execution_duration_seconds", Help: "Step execution duration in seconds"},
		[]string{"tenant_id", "step_type"},
	)

	TasksActiveTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "aceryx_tasks_active_total", Help: "Active tasks by tenant"},
		[]string{"tenant_id"},
	)
	TasksClaimedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_tasks_claimed_total", Help: "Claimed tasks by tenant"},
		[]string{"tenant_id"},
	)
	TasksCompletedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_tasks_completed_total", Help: "Completed tasks by tenant and outcome"},
		[]string{"tenant_id", "outcome"},
	)
	SLABreachesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_sla_breaches_total", Help: "SLA breaches by tenant"},
		[]string{"tenant_id"},
	)

	AgentInvocationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_agent_invocations_total", Help: "Agent invocations by tenant and model"},
		[]string{"tenant_id", "model"},
	)
	AgentDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "aceryx_agent_duration_seconds", Help: "Agent LLM duration in seconds"},
		[]string{"tenant_id"},
	)
	AgentTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_agent_tokens_total", Help: "Agent token counts by direction"},
		[]string{"tenant_id", "model", "direction"},
	)
	AgentConfidenceScore = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aceryx_agent_confidence_score",
			Help:    "Agent confidence score distribution",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		},
		[]string{"tenant_id"},
	)
	AgentEscalationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_agent_escalations_total", Help: "Agent low-confidence escalations"},
		[]string{"tenant_id"},
	)

	ConnectorCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "aceryx_connector_calls_total", Help: "Connector calls by status"},
		[]string{"tenant_id", "connector", "action", "status"},
	)
	ConnectorDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "aceryx_connector_duration_seconds", Help: "Connector call duration"},
		[]string{"tenant_id", "connector"},
	)

	DBPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "aceryx_db_pool_size", Help: "DB pool size by state"},
		[]string{"state"},
	)
	DBQueryDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "aceryx_db_query_duration_seconds", Help: "Database query duration"},
		[]string{"query_type"},
	)
	WebSocketConnectionsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "aceryx_websocket_connections_total", Help: "WebSocket connections by tenant"},
		[]string{"tenant_id"},
	)
	WorkerPoolUtilisation = promauto.NewGauge(
		prometheus.GaugeOpts{Name: "aceryx_worker_pool_utilisation", Help: "Worker pool utilisation (0-1)"},
	)
)

func ObserveHTTPRequest(method, path string, statusCode int, seconds float64) {
	status := strconv.Itoa(statusCode)
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDurationSeconds.WithLabelValues(method, path).Observe(seconds)
}

func UpdateDBPoolStats(db *sql.DB) {
	if db == nil {
		return
	}
	stats := db.Stats()
	DBPoolSize.WithLabelValues("idle").Set(float64(stats.Idle))
	DBPoolSize.WithLabelValues("in_use").Set(float64(stats.InUse))
	DBPoolSize.WithLabelValues("total").Set(float64(stats.OpenConnections))
}
