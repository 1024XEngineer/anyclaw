package security

// Role describes the kind of caller connected to the gateway.
type Role string

const (
	RoleOperator Role = "operator"
	RoleNode     Role = "node"
)

const (
	ScopeOperatorRead        = "operator.read"
	ScopeOperatorWrite       = "operator.write"
	ScopeOperatorAdmin       = "operator.admin"
	ScopeOperatorApprovals   = "operator.approvals"
	ScopeOperatorPairing     = "operator.pairing"
	ScopeOperatorTalkSecrets = "operator.talk.secrets"
)
