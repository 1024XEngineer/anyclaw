package runtime

import (
	"github.com/anyclaw/anyclaw/pkg/config"
	runtimecontext "github.com/anyclaw/anyclaw/pkg/runtime/context"
	"github.com/anyclaw/anyclaw/pkg/state/memory"
	"github.com/anyclaw/anyclaw/pkg/state/policy/secrets"
)

func deriveAgentContextTokenBudget(llmMaxTokens int) int {
	return runtimecontext.DeriveAgentContextTokenBudget(llmMaxTokens)
}

func resolveEmbedder(cfg *config.Config, secretsSnap *secrets.RuntimeSnapshot) memory.EmbeddingProvider {
	return runtimecontext.ResolveEmbedder(cfg, secretsSnap)
}
