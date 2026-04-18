package runtime

import (
	"github.com/1024XEngineer/anyclaw/pkg/config"
	runtimecontext "github.com/1024XEngineer/anyclaw/pkg/runtime/context"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
)

func deriveAgentContextTokenBudget(llmMaxTokens int) int {
	return runtimecontext.DeriveAgentContextTokenBudget(llmMaxTokens)
}

func resolveEmbedder(cfg *config.Config, secretsSnap *secrets.RuntimeSnapshot) memory.EmbeddingProvider {
	return runtimecontext.ResolveEmbedder(cfg, secretsSnap)
}
