package nodes

import "context"

// Invoke finds a node and executes a command against it.
func (r *Registry) Invoke(ctx context.Context, nodeID string, command Command) (Result, error) {
	node, ok := r.Get(nodeID)
	if !ok {
		return Result{Error: "node not found"}, nil
	}
	return node.Invoke(ctx, command)
}
