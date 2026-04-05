package rag

import "testing"

func TestEstimateReIndexCost(t *testing.T) {
	kb := &KnowledgeBase{ChunkCount: 500, ChunkSize: 400}
	estimate := EstimateReIndexCost(kb, ModelPricing{InputPer1MTokensUSD: 0.1})
	if estimate.TotalChunks != 500 {
		t.Fatalf("expected 500 chunks, got %d", estimate.TotalChunks)
	}
	if estimate.EstimatedTokens != 200000 {
		t.Fatalf("expected 200000 tokens, got %d", estimate.EstimatedTokens)
	}
	if estimate.EstimatedCostUSD <= 0 {
		t.Fatalf("expected positive estimated cost")
	}
}

func TestRequiresReIndexConfirmation(t *testing.T) {
	if !RequiresReIndexConfirmation(CostEstimate{EstimatedCostUSD: 51}, 50) {
		t.Fatalf("expected confirmation required")
	}
	if RequiresReIndexConfirmation(CostEstimate{EstimatedCostUSD: 10}, 50) {
		t.Fatalf("expected confirmation not required")
	}
}
