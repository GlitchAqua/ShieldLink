package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Config struct {
	Mode          string        `json:"mode"`
	Listen        string        `json:"listen"`
	UUID          string        `json:"uuid"`
	Protocol      string        `json:"protocol"`
	Transport     string        `json:"transport"`
	TLS           TLSConfig     `json:"tls"`
	Forward       string        `json:"forward"`
	Servers       []ServerEntry `json:"servers"`      // client mode: list of decrypt servers
	IPPassthrough bool          `json:"ip_passthrough"`
	MPTCP         bool          `json:"mptcp"`
	Log           LogConfig     `json:"log"`
	API           APIConfig     `json:"api"`
	Reassembly    MergeConfig   `json:"reassembly"`
}

type TLSConfig struct {
	AutoCert bool   `json:"auto_cert"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

type LogConfig struct {
	Enabled bool   `json:"enabled"`
	Level   string `json:"level"`
	File    string `json:"file"`
}

type APIConfig struct {
	URL          string `json:"url"`
	Token        string `json:"token"`
	PollInterval int    `json:"poll_interval"` // seconds, default 60
}

type MergeConfig struct {
	BufferSize int `json:"buffer_size"`
	Timeout    int `json:"timeout"`
}

// ClientConfig is the config for client mode.
type ClientConfig struct {
	Listen        string         `json:"listen"`
	UUID          string         `json:"uuid"`
	Protocol      string         `json:"protocol"`
	Transport     string         `json:"transport"`
	Servers       []ServerEntry  `json:"servers"`
	IPPassthrough bool           `json:"ip_passthrough"`
}

type ServerEntry struct {
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
}

// ToClientConfig converts the generic Config to a ClientConfig.
func (c *Config) ToClientConfig() *ClientConfig {
	var servers []ServerEntry
	for _, s := range c.Servers {
		servers = append(servers, ServerEntry{Address: s.Address, Enabled: s.Enabled})
	}
	return &ClientConfig{
		Listen:        c.Listen,
		UUID:          c.UUID,
		Protocol:      c.Protocol,
		Transport:     c.Transport,
		Servers:       servers,
		IPPassthrough: c.IPPassthrough,
	}
}

// Load reads config from a local file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return parse(data)
}

// LoadFromAPI fetches config from an API server.
// endpoint: full URL, e.g. https://api.example.com/api/shieldlink/config?mode=server&node_id=1
// token: Bearer token for authentication
func LoadFromAPI(endpoint string, token string) (*Config, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parse(data)
}

// StartConfigPoller periodically fetches config from API and calls onChange when it changes.
func StartConfigPoller(apiURL, token string, interval time.Duration, onChange func(*Config)) {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	go func() {
		var lastJSON string
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			cfg, err := LoadFromAPI(apiURL, token)
			if err != nil {
				// Log will be handled by caller
				continue
			}

			data, _ := json.Marshal(cfg)
			currentJSON := string(data)
			if currentJSON != lastJSON {
				lastJSON = currentJSON
				onChange(cfg)
			}
		}
	}()
}

func parse(data []byte) (*Config, error) {
	cfg := &Config{
		Protocol:  "tcp",
		Transport: "h2",
		Log: LogConfig{
			Enabled: true,
			Level:   "info",
		},
		Reassembly: MergeConfig{
			BufferSize: 4 * 1024 * 1024,
			Timeout:    5,
		},
		API: APIConfig{
			PollInterval: 60,
		},
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.UUID == "" {
		return nil, fmt.Errorf("uuid is required")
	}
	if cfg.Mode != "server" && cfg.Mode != "merge" && cfg.Mode != "client" {
		return nil, fmt.Errorf("mode must be 'server', 'merge', or 'client'")
	}
	if cfg.Listen == "" {
		return nil, fmt.Errorf("listen is required")
	}
	if cfg.Forward == "" && cfg.Mode != "client" {
		return nil, fmt.Errorf("forward is required")
	}

	return cfg, nil
}
