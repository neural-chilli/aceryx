package llm

import "testing"

func TestUsageResponseTypes(t *testing.T) {
	_ = ProviderTestResult{Status: "ok"}
	_ = UsageSummaryResponse{}
	_ = UsageDetailsResponse{}
	_ = UsageByPurposeResponse{}
}
