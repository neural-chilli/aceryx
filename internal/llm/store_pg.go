package llm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) RecordInvocation(ctx context.Context, inv Invocation) error {
	if s == nil || s.db == nil {
		return nil
	}
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO llm_invocations (
    id, tenant_id, provider_id, provider, model, purpose,
    input_tokens, output_tokens, total_tokens, duration_ms,
    status, error_message, cost_usd, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, NULLIF($12, ''), $13, $14
)
`, inv.ID, inv.TenantID, inv.ProviderID, inv.Provider, inv.Model, inv.Purpose,
		inv.InputTokens, inv.OutputTokens, inv.TotalTokens, inv.DurationMS,
		inv.Status, strings.TrimSpace(inv.ErrorMessage), inv.CostUSD, inv.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert llm invocation: %w", err)
	}
	return nil
}

func (s *Store) GetMonthlyUsage(ctx context.Context, tenantID uuid.UUID, yearMonth string) (MonthlyUsage, error) {
	if s == nil || s.db == nil {
		return MonthlyUsage{}, nil
	}
	out := MonthlyUsage{TenantID: tenantID, YearMonth: yearMonth}
	err := s.db.QueryRowContext(ctx, `
SELECT tenant_id, year_month, total_tokens, total_cost_usd, invocation_count
FROM llm_usage_monthly
WHERE tenant_id = $1 AND year_month = $2
`, tenantID, yearMonth).Scan(&out.TenantID, &out.YearMonth, &out.TotalTokens, &out.TotalCostUSD, &out.InvocationCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return out, nil
		}
		return MonthlyUsage{}, fmt.Errorf("query llm monthly usage: %w", err)
	}
	return out, nil
}

func (s *Store) UpdateMonthlyUsage(ctx context.Context, tenantID uuid.UUID, tokens int, costUSD float64) error {
	if s == nil || s.db == nil {
		return nil
	}
	yearMonth := time.Now().UTC().Format("2006-01")
	_, err := s.db.ExecContext(ctx, `
INSERT INTO llm_usage_monthly (tenant_id, year_month, total_tokens, total_cost_usd, invocation_count, updated_at)
VALUES ($1, $2, $3, $4, 1, now())
ON CONFLICT (tenant_id, year_month)
DO UPDATE SET
    total_tokens = llm_usage_monthly.total_tokens + EXCLUDED.total_tokens,
    total_cost_usd = llm_usage_monthly.total_cost_usd + EXCLUDED.total_cost_usd,
    invocation_count = llm_usage_monthly.invocation_count + 1,
    updated_at = now()
`, tenantID, yearMonth, tokens, costUSD)
	if err != nil {
		return fmt.Errorf("upsert llm monthly usage: %w", err)
	}
	return nil
}

func (s *Store) ListInvocations(ctx context.Context, tenantID uuid.UUID, opts ListOpts) ([]Invocation, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	since := opts.Since
	if since.IsZero() {
		since = time.Now().UTC().AddDate(0, 0, -30)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, provider_id, provider, model, purpose,
       COALESCE(input_tokens, 0), COALESCE(output_tokens, 0), COALESCE(total_tokens, 0),
       duration_ms, status, COALESCE(error_message, ''), COALESCE(cost_usd, 0), created_at
FROM llm_invocations
WHERE tenant_id = $1
  AND created_at >= $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4
`, tenantID, since, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query llm invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Invocation, 0)
	for rows.Next() {
		var inv Invocation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.ProviderID, &inv.Provider, &inv.Model, &inv.Purpose,
			&inv.InputTokens, &inv.OutputTokens, &inv.TotalTokens, &inv.DurationMS,
			&inv.Status, &inv.ErrorMessage, &inv.CostUSD, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan llm invocation: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate llm invocations: %w", err)
	}
	return out, nil
}

func (s *Store) UsageByPurpose(ctx context.Context, tenantID uuid.UUID, since time.Time) ([]PurposeUsage, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if since.IsZero() {
		since = time.Now().UTC().AddDate(0, -1, 0)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT purpose,
       COALESCE(SUM(total_tokens), 0) AS total_tokens,
       COALESCE(SUM(cost_usd), 0) AS total_cost_usd,
       COUNT(*) AS invocation_count
FROM llm_invocations
WHERE tenant_id = $1
  AND created_at >= $2
GROUP BY purpose
ORDER BY purpose
`, tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("query llm usage by purpose: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PurposeUsage, 0)
	for rows.Next() {
		var item PurposeUsage
		if err := rows.Scan(&item.Purpose, &item.TotalTokens, &item.TotalCostUSD, &item.InvocationCount); err != nil {
			return nil, fmt.Errorf("scan llm usage by purpose: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate llm usage by purpose: %w", err)
	}
	return out, nil
}

func (s *Store) ListProviders(ctx context.Context, tenantID uuid.UUID) ([]LLMProviderConfig, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, provider, name, COALESCE(endpoint_url, ''), api_key_secret,
       default_model, max_tokens, temperature, is_default, is_fallback, enabled,
       COALESCE(model_map, '{}'::jsonb), COALESCE(model_pricing, '{}'::jsonb),
       requests_per_min, tenant_requests_per_min,
       monthly_token_budget, monthly_cost_budget,
       COALESCE(azure_api_version, ''), COALESCE(azure_deployment, ''), azure,
       created_at, updated_at
FROM llm_provider_configs
WHERE tenant_id = $1
ORDER BY created_at DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query llm providers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]LLMProviderConfig, 0)
	for rows.Next() {
		cfg, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate llm providers: %w", err)
	}
	return out, nil
}

func (s *Store) GetProvider(ctx context.Context, tenantID uuid.UUID, configID uuid.UUID) (LLMProviderConfig, error) {
	if s == nil || s.db == nil {
		return LLMProviderConfig{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, provider, name, COALESCE(endpoint_url, ''), api_key_secret,
       default_model, max_tokens, temperature, is_default, is_fallback, enabled,
       COALESCE(model_map, '{}'::jsonb), COALESCE(model_pricing, '{}'::jsonb),
       requests_per_min, tenant_requests_per_min,
       monthly_token_budget, monthly_cost_budget,
       COALESCE(azure_api_version, ''), COALESCE(azure_deployment, ''), azure,
       created_at, updated_at
FROM llm_provider_configs
WHERE tenant_id = $1 AND id = $2
`, tenantID, configID)
	cfg, err := scanProvider(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return LLMProviderConfig{}, err
		}
		return LLMProviderConfig{}, fmt.Errorf("query llm provider: %w", err)
	}
	return cfg, nil
}

func (s *Store) CreateProvider(ctx context.Context, config LLMProviderConfig) (LLMProviderConfig, error) {
	if s == nil || s.db == nil {
		return config, nil
	}
	if config.ID == uuid.Nil {
		config.ID = uuid.New()
	}
	modelMapRaw, _ := json.Marshal(config.ModelMap)
	pricingRaw, _ := json.Marshal(config.ModelPricing)
	row := s.db.QueryRowContext(ctx, `
INSERT INTO llm_provider_configs (
    id, tenant_id, provider, name, endpoint_url, api_key_secret,
    default_model, max_tokens, temperature, is_default, is_fallback, enabled,
    model_map, model_pricing, requests_per_min, tenant_requests_per_min,
    monthly_token_budget, monthly_cost_budget,
    azure_api_version, azure_deployment, azure,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, NULLIF($5, ''), $6,
    $7, $8, $9, $10, $11, $12,
    $13::jsonb, $14::jsonb, $15, $16,
    $17, $18,
    NULLIF($19, ''), NULLIF($20, ''), $21,
    now(), now()
)
RETURNING id, tenant_id, provider, name, COALESCE(endpoint_url, ''), api_key_secret,
          default_model, max_tokens, temperature, is_default, is_fallback, enabled,
          COALESCE(model_map, '{}'::jsonb), COALESCE(model_pricing, '{}'::jsonb),
          requests_per_min, tenant_requests_per_min,
          monthly_token_budget, monthly_cost_budget,
          COALESCE(azure_api_version, ''), COALESCE(azure_deployment, ''), azure,
          created_at, updated_at
`, config.ID, config.TenantID, config.Provider, config.Name, config.EndpointURL, config.APIKeySecret,
		config.DefaultModel, config.MaxTokens, config.Temperature, config.IsDefault, config.IsFallback, config.Enabled,
		string(modelMapRaw), string(pricingRaw), config.RequestsPerMin, config.TenantRPM,
		config.MonthlyTokenBudget, config.MonthlyCostBudget,
		config.AzureAPIVersion, config.AzureDeployment, config.Azure)
	created, err := scanProvider(row)
	if err != nil {
		return LLMProviderConfig{}, fmt.Errorf("insert llm provider: %w", err)
	}
	return created, nil
}

func (s *Store) UpdateProvider(ctx context.Context, config LLMProviderConfig) (LLMProviderConfig, error) {
	if s == nil || s.db == nil {
		return config, nil
	}
	modelMapRaw, _ := json.Marshal(config.ModelMap)
	pricingRaw, _ := json.Marshal(config.ModelPricing)
	row := s.db.QueryRowContext(ctx, `
UPDATE llm_provider_configs
SET provider = $3,
    name = $4,
    endpoint_url = NULLIF($5, ''),
    api_key_secret = $6,
    default_model = $7,
    max_tokens = $8,
    temperature = $9,
    is_default = $10,
    is_fallback = $11,
    enabled = $12,
    model_map = $13::jsonb,
    model_pricing = $14::jsonb,
    requests_per_min = $15,
    tenant_requests_per_min = $16,
    monthly_token_budget = $17,
    monthly_cost_budget = $18,
    azure_api_version = NULLIF($19, ''),
    azure_deployment = NULLIF($20, ''),
    azure = $21,
    updated_at = now()
WHERE tenant_id = $1
  AND id = $2
RETURNING id, tenant_id, provider, name, COALESCE(endpoint_url, ''), api_key_secret,
          default_model, max_tokens, temperature, is_default, is_fallback, enabled,
          COALESCE(model_map, '{}'::jsonb), COALESCE(model_pricing, '{}'::jsonb),
          requests_per_min, tenant_requests_per_min,
          monthly_token_budget, monthly_cost_budget,
          COALESCE(azure_api_version, ''), COALESCE(azure_deployment, ''), azure,
          created_at, updated_at
`, config.TenantID, config.ID, config.Provider, config.Name, config.EndpointURL, config.APIKeySecret,
		config.DefaultModel, config.MaxTokens, config.Temperature, config.IsDefault, config.IsFallback, config.Enabled,
		string(modelMapRaw), string(pricingRaw), config.RequestsPerMin, config.TenantRPM,
		config.MonthlyTokenBudget, config.MonthlyCostBudget,
		config.AzureAPIVersion, config.AzureDeployment, config.Azure)
	updated, err := scanProvider(row)
	if err != nil {
		return LLMProviderConfig{}, fmt.Errorf("update llm provider: %w", err)
	}
	return updated, nil
}

func (s *Store) DeleteProvider(ctx context.Context, tenantID uuid.UUID, configID uuid.UUID) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM llm_provider_configs
WHERE tenant_id = $1
  AND id = $2
`, tenantID, configID)
	if err != nil {
		return fmt.Errorf("delete llm provider: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProvider(row scanner) (LLMProviderConfig, error) {
	var (
		cfg        LLMProviderConfig
		modelMap   json.RawMessage
		pricingMap json.RawMessage
	)
	err := row.Scan(
		&cfg.ID,
		&cfg.TenantID,
		&cfg.Provider,
		&cfg.Name,
		&cfg.EndpointURL,
		&cfg.APIKeySecret,
		&cfg.DefaultModel,
		&cfg.MaxTokens,
		&cfg.Temperature,
		&cfg.IsDefault,
		&cfg.IsFallback,
		&cfg.Enabled,
		&modelMap,
		&pricingMap,
		&cfg.RequestsPerMin,
		&cfg.TenantRPM,
		&cfg.MonthlyTokenBudget,
		&cfg.MonthlyCostBudget,
		&cfg.AzureAPIVersion,
		&cfg.AzureDeployment,
		&cfg.Azure,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return LLMProviderConfig{}, err
	}
	cfg.ModelMap = normalizeJSONMap(modelMap)
	cfg.ModelPricing = normalizePricingMap(pricingMap)
	return cfg, nil
}
