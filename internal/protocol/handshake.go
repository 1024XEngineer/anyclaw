package protocol

// ClientInfo identifies the connecting client binary or app.
type ClientInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

// DeviceIdentity represents a signed device identity during connect.
type DeviceIdentity struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

// ConnectParams is the first request payload expected by the gateway.
type ConnectParams struct {
	MinProtocol int            `json:"minProtocol"`
	MaxProtocol int            `json:"maxProtocol"`
	Client      ClientInfo     `json:"client"`
	Role        string         `json:"role"`
	Scopes      []string       `json:"scopes"`
	Caps        []string       `json:"caps,omitempty"`
	Commands    []string       `json:"commands,omitempty"`
	Permissions map[string]any `json:"permissions,omitempty"`
	Locale      string         `json:"locale,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
	Device      DeviceIdentity `json:"device"`
}

// ConnectChallenge is emitted before the client sends the connect request.
type ConnectChallenge struct {
	Nonce string `json:"nonce"`
	TS    int64  `json:"ts"`
}

// HelloOK is returned when the gateway accepts a connection.
type HelloOK struct {
	Type     string        `json:"type"`
	Protocol int           `json:"protocol"`
	Policy   GatewayPolicy `json:"policy"`
}

// GatewayPolicy contains dynamic connection policy values.
type GatewayPolicy struct {
	TickIntervalMS int `json:"tickIntervalMs"`
}
