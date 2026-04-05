package rag

const DefaultReIndexCostGuardUSD = 50.0

func RequiresReIndexConfirmation(estimate CostEstimate, thresholdUSD float64) bool {
	if thresholdUSD <= 0 {
		thresholdUSD = DefaultReIndexCostGuardUSD
	}
	return estimate.EstimatedCostUSD > thresholdUSD
}
