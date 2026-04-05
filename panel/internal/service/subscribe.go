package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// FetchAndDecorate fetches subscription from upstream and injects shieldlink blocks.
func FetchAndDecorate(upstream *model.Upstream, originalPath, queryString, userAgent string) ([]byte, http.Header, int, error) {
	// Build upstream URL
	upstreamURL := strings.TrimRight(upstream.URL, "/") + originalPath
	if queryString != "" {
		upstreamURL += "?" + queryString
	}

	// Forward request to upstream
	req, err := http.NewRequest("GET", upstreamURL, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("fetch upstream: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read body: %w", err)
	}

	// If not 200 or not YAML-like, return as-is
	if resp.StatusCode != 200 || !looksLikeYAML(body) {
		return body, resp.Header, resp.StatusCode, nil
	}

	// Load decoration rules for this upstream
	var rules []model.DecorationRule
	database.DB.Preload("Route").
		Where("upstream_id = ? AND enabled = ?", upstream.ID, true).
		Order("priority desc, id asc").
		Find(&rules)

	if len(rules) == 0 {
		return body, resp.Header, resp.StatusCode, nil
	}

	// Filter rules by UA pattern
	activeRules := filterByUA(rules, userAgent)
	if len(activeRules) == 0 {
		return body, resp.Header, resp.StatusCode, nil
	}

	// Load decrypt servers
	var servers []model.DecryptServer
	database.DB.Where("enabled = ?", true).Find(&servers)
	serverMap := make(map[uint]*model.DecryptServer)
	for i := range servers {
		serverMap[servers[i].ID] = &servers[i]
	}

	// Load merge servers
	var mergeServers []model.MergeServer
	database.DB.Where("enabled = ?", true).Find(&mergeServers)
	mergeMap := make(map[uint]*model.MergeServer)
	for i := range mergeServers {
		mergeMap[mergeServers[i].ID] = &mergeServers[i]
	}

	// Inject shieldlink blocks
	modified, err := injectShieldLink(body, activeRules, serverMap, mergeMap)
	if err != nil {
		return body, resp.Header, resp.StatusCode, nil // fallback to original
	}

	return modified, resp.Header, resp.StatusCode, nil
}

func filterByUA(rules []model.DecorationRule, ua string) []model.DecorationRule {
	var result []model.DecorationRule
	for _, r := range rules {
		if r.UAPattern == "" {
			result = append(result, r)
			continue
		}
		re, err := regexp.Compile("(?i)" + r.UAPattern)
		if err != nil {
			continue
		}
		if re.MatchString(ua) {
			result = append(result, r)
		}
	}
	return result
}

// injectShieldLink uses line-level string processing instead of full YAML parse
// to avoid the extreme slowness of yaml.Unmarshal on large configs (500KB+).
// nameFromInline extracts the name value from an inline proxy line like:
//
//	- { name: '🇭🇰SS|香港B01|IPLC x3', type: ss, ... }
func nameFromInline(line string) string {
	// Find name: in the inline block
	idx := strings.Index(line, "name:")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[idx+5:])
	if len(rest) == 0 {
		return ""
	}

	// Handle quoted: name: 'xxx' or name: "xxx"
	quote := rest[0]
	if quote == '\'' || quote == '"' {
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			return ""
		}
		return rest[1 : end+1]
	}
	// Unquoted: name: xxx, ...  or name: xxx }
	end := strings.IndexAny(rest, ",}")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// nameFromMultiline extracts name from a multi-line YAML like:
//
//	- name: "HK-SS-01"
func nameFromMultiline(trimmed string) string {
	var raw string
	if strings.HasPrefix(trimmed, "- name:") {
		raw = strings.TrimSpace(trimmed[7:])
	} else if strings.HasPrefix(trimmed, "name:") {
		raw = strings.TrimSpace(trimmed[5:])
	}
	return strings.Trim(raw, "\"'")
}

func injectShieldLink(yamlData []byte, rules []model.DecorationRule, serverMap map[uint]*model.DecryptServer, mergeMap map[uint]*model.MergeServer) ([]byte, error) {
	type compiledRule struct {
		rule    model.DecorationRule
		pattern *regexp.Regexp
	}
	var compiled []compiledRule
	for _, r := range rules {
		re, err := regexp.Compile("(?i)" + r.MatchPattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledRule{rule: r, pattern: re})
	}
	if len(compiled) == 0 {
		return yamlData, nil
	}

	lines := strings.Split(string(yamlData), "\n")
	var result strings.Builder
	result.Grow(len(yamlData) + 4096)

	inProxiesSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track when we're inside the proxies: section
		if strings.HasPrefix(trimmed, "proxies:") {
			inProxiesSection = true
		} else if len(trimmed) > 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") && trimmed[0] != ' ' {
			// A new top-level key means we left the proxies section
			if inProxiesSection && !strings.HasPrefix(trimmed, "- ") {
				inProxiesSection = false
			}
		}

		// Case 1: Inline format  - { name: 'xxx', type: ss, ... }
		if inProxiesSection && strings.HasPrefix(trimmed, "- {") && strings.Contains(trimmed, "name:") && strings.Contains(trimmed, "type:") {
			nameVal := nameFromInline(trimmed)
			modified := false
			if nameVal != "" {
				for _, cr := range compiled {
					if !cr.pattern.MatchString(nameVal) {
						continue
					}
					slInline := buildSLBlockInline(cr.rule, serverMap, mergeMap)
					if slInline != "" {
						// Insert before the closing }
						lastBrace := strings.LastIndex(line, "}")
						if lastBrace >= 0 {
							result.WriteString(line[:lastBrace])
							result.WriteString(", ")
							result.WriteString(slInline)
							result.WriteString(" }")
							result.WriteByte('\n')
							modified = true
						}
					}
					break
				}
			}
			if !modified {
				result.WriteString(line)
				result.WriteByte('\n')
			}
			continue
		}

		// Case 2: Multi-line format  - name: "xxx" or  name: xxx
		result.WriteString(line)
		result.WriteByte('\n')

		if !strings.HasPrefix(trimmed, "- name:") && !strings.HasPrefix(trimmed, "name:") {
			continue
		}
		nameVal := nameFromMultiline(trimmed)
		if nameVal == "" {
			continue
		}
		for _, cr := range compiled {
			if !cr.pattern.MatchString(nameVal) {
				continue
			}
			slYAML := buildSLBlockYAML(cr.rule, serverMap, mergeMap)
			if slYAML != "" {
				indent := "    "
				if idx := strings.Index(line, "- name:"); idx >= 0 {
					indent = strings.Repeat(" ", idx+2)
				} else if idx := strings.Index(line, "name:"); idx >= 0 {
					indent = strings.Repeat(" ", idx)
				}
				for _, sl := range strings.Split(slYAML, "\n") {
					if sl != "" {
						result.WriteString(indent)
						result.WriteString(sl)
						result.WriteByte('\n')
					}
				}
			}
			break
		}
	}

	return []byte(result.String()), nil
}

// buildSLBlockInline returns an inline YAML fragment like:
// shieldlink: { uuid: "xxx", servers: [{ address: "1.2.3.4:19443", enabled: true }], protocol: tcp, transport: h2 }
// ensurePort appends the default shieldlink port if the address has no port.
func ensurePort(addr string, defaultPort string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

func buildSLBlockInline(rule model.DecorationRule, serverMap map[uint]*model.DecryptServer, mergeMap map[uint]*model.MergeServer) string {
	if rule.Route.UUID == "" {
		return ""
	}

	var serverIDs []uint
	json.Unmarshal([]byte(rule.ServerIDs), &serverIDs)

	var srvParts []string
	for _, sid := range serverIDs {
		srv, ok := serverMap[sid]
		if !ok || !srv.Enabled {
			continue
		}
		srvParts = append(srvParts, fmt.Sprintf("{ address: \"%s\", enabled: true }", ensurePort(srv.Address, "19443")))
	}
	if len(srvParts) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("uuid: \"%s\"", rule.Route.UUID))
	parts = append(parts, fmt.Sprintf("servers: [%s]", strings.Join(srvParts, ", ")))
	parts = append(parts, fmt.Sprintf("protocol: %s", rule.Protocol))
	if rule.Protocol == "tcp" && rule.Transport != "" {
		parts = append(parts, fmt.Sprintf("transport: %s", rule.Transport))
	}
	if rule.MPTCP {
		parts = append(parts, "mptcp: true")
	}
	if rule.Aggregate {
		parts = append(parts, "aggregate: true")
	}

	return "shieldlink: { " + strings.Join(parts, ", ") + " }"
}

// buildSLBlockYAML returns a YAML snippet for the shieldlink block.
func buildSLBlockYAML(rule model.DecorationRule, serverMap map[uint]*model.DecryptServer, mergeMap map[uint]*model.MergeServer) string {
	if rule.Route.UUID == "" {
		return ""
	}

	var serverIDs []uint
	json.Unmarshal([]byte(rule.ServerIDs), &serverIDs)

	var serverLines []string
	for _, sid := range serverIDs {
		srv, ok := serverMap[sid]
		if !ok || !srv.Enabled {
			continue
		}
		serverLines = append(serverLines, fmt.Sprintf("    - address: \"%s\"\n      enabled: true", ensurePort(srv.Address, "19443")))
	}
	if len(serverLines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("shieldlink:\n  uuid: \"%s\"\n  servers:\n", rule.Route.UUID))
	for _, sl := range serverLines {
		b.WriteString("  ")
		b.WriteString(sl)
		b.WriteByte('\n')
	}
	b.WriteString(fmt.Sprintf("  protocol: %s\n", rule.Protocol))
	if rule.Protocol == "tcp" && rule.Transport != "" {
		b.WriteString(fmt.Sprintf("  transport: %s\n", rule.Transport))
	}
	if rule.MPTCP {
		b.WriteString("  mptcp: true\n")
	}
	if rule.Aggregate {
		b.WriteString("  aggregate: true\n")
	}
	return b.String()
}

func buildSLBlock(rule model.DecorationRule, serverMap map[uint]*model.DecryptServer, mergeMap map[uint]*model.MergeServer) map[string]interface{} {
	if rule.Route.UUID == "" {
		return nil
	}

	// Parse server IDs
	var serverIDs []uint
	json.Unmarshal([]byte(rule.ServerIDs), &serverIDs)

	var servers []map[string]interface{}
	for _, sid := range serverIDs {
		srv, ok := serverMap[sid]
		if !ok || !srv.Enabled {
			continue
		}
		servers = append(servers, map[string]interface{}{
			"address": ensurePort(srv.Address, "19443"),
			"enabled": true,
		})
	}
	if len(servers) == 0 {
		return nil
	}

	block := map[string]interface{}{
		"uuid":     rule.Route.UUID,
		"servers":  servers,
		"protocol": rule.Protocol,
	}
	if rule.Protocol == "tcp" {
		block["transport"] = rule.Transport
	}
	if rule.MPTCP {
		block["mptcp"] = true
	}
	if rule.Aggregate {
		block["aggregate"] = true
	}
	return block
}

func looksLikeYAML(data []byte) bool {
	// Check the entire file (not just first 200 bytes) for proxy section markers
	s := string(data[:min(len(data), len(data))])
	return strings.Contains(s, "proxies:") || strings.Contains(s, "proxy-groups:")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
