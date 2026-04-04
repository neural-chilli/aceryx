package llm

import "time"

type ProviderTestResult struct {
	Status string `json:"status"`
}

type UsageSummaryResponse struct {
	MonthlyUsage MonthlyUsage `json:"monthly_usage"`
}

type UsageDetailsResponse struct {
	Invocations []Invocation `json:"invocations"`
}

type UsageByPurposeResponse struct {
	Items []PurposeUsage `json:"items"`
	Since time.Time      `json:"since"`
}
