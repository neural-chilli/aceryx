package agentic

import "time"

type ConstraintEnforcer struct {
	limits     ReasoningLimits
	startTime  time.Time
	iteration  int
	toolCalls  int
	tokensUsed int
}

func NewConstraintEnforcer(limits ReasoningLimits) *ConstraintEnforcer {
	limits.ApplyDefaults()
	return &ConstraintEnforcer{
		limits:    limits,
		startTime: time.Now(),
	}
}

func (ce *ConstraintEnforcer) CheckIteration() string {
	if ce == nil {
		return ""
	}
	if ce.iteration >= ce.limits.MaxIterations {
		return "You have reached the maximum number of reasoning iterations. You must conclude now with your best assessment."
	}
	return ""
}

func (ce *ConstraintEnforcer) CheckToolCalls() string {
	if ce == nil {
		return ""
	}
	if ce.toolCalls >= ce.limits.MaxToolCalls {
		return "Tool call limit reached. You cannot make more tool calls. Conclude with available information."
	}
	return ""
}

func (ce *ConstraintEnforcer) CheckTokenBudget(tokensUsed int) (warning string, hardStop bool) {
	if ce == nil {
		return "", false
	}
	ce.tokensUsed = tokensUsed
	if ce.tokensUsed >= ce.limits.MaxTokens {
		return "", true
	}
	if ce.tokensUsed >= int(float64(ce.limits.MaxTokens)*0.8) {
		return "80% of token budget consumed. Conclude soon.", false
	}
	return "", false
}

func (ce *ConstraintEnforcer) CheckTimeout() bool {
	if ce == nil {
		return false
	}
	return time.Since(ce.startTime) >= ce.limits.Timeout
}

func (ce *ConstraintEnforcer) IncrementIteration() {
	if ce != nil {
		ce.iteration++
	}
}

func (ce *ConstraintEnforcer) IncrementToolCalls() {
	if ce != nil {
		ce.toolCalls++
	}
}

func (ce *ConstraintEnforcer) SetTokensUsed(tokens int) {
	if ce != nil {
		ce.tokensUsed = tokens
	}
}
