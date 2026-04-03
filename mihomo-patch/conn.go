package shieldlink

// ServerConfig represents a single ShieldLink server endpoint.
type ServerConfig struct {
	Address string `proxy:"address"`
	Enabled bool   `proxy:"enabled,omitempty"`
}

// ClientConfig holds ShieldLink client options parsed from node config.
type ClientConfig struct {
	UUID          string         `proxy:"uuid"`
	Servers       []ServerConfig `proxy:"servers"`
	Protocol      string         `proxy:"protocol,omitempty"`  // "tcp" or "udp"
	Transport     string         `proxy:"transport,omitempty"` // "h2" or "ws"
	IPPassthrough bool           `proxy:"ip-passthrough,omitempty"`
	MPTCP         bool           `proxy:"mptcp,omitempty"`
	LogLevel      string         `proxy:"log-level,omitempty"`
	Aggregate     bool           `proxy:"aggregate,omitempty"`
	MergeAddress  string         `proxy:"merge-address,omitempty"`
}
