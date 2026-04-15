package routing

import "fmt"

func normalizeAccountID(accountID string) string {
	if accountID == "" {
		return "default"
	}
	return accountID
}

// BuildMainSessionKey creates the canonical main-session key for an agent/account pair.
func BuildMainSessionKey(agentID string, accountID string) string {
	return fmt.Sprintf("agent:%s:account:%s:main", agentID, normalizeAccountID(accountID))
}

// BuildPeerSessionKey creates the canonical peer-scoped session key.
func BuildPeerSessionKey(agentID string, accountID string, peerKind string, peerID string) string {
	if peerKind == "" {
		peerKind = "direct"
	}
	if peerID == "" {
		return BuildMainSessionKey(agentID, accountID)
	}
	return fmt.Sprintf("agent:%s:account:%s:%s:%s", agentID, normalizeAccountID(accountID), peerKind, peerID)
}
