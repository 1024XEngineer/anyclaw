package context

const DefaultAgentContextTokenFloor = 16384

func DeriveAgentContextTokenBudget(llmMaxTokens int) int {
	if llmMaxTokens <= 0 {
		return DefaultAgentContextTokenFloor
	}

	budget := llmMaxTokens * 2
	if budget < DefaultAgentContextTokenFloor {
		budget = DefaultAgentContextTokenFloor
	}
	return budget
}
