package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
)

var syncClient = &http.Client{Timeout: 10 * time.Second}

type routePayload struct {
	UUID    string `json:"uuid"`
	Forward string `json:"forward"`
}

// buildRoutePayloads builds route payloads for decrypt servers and merge servers.
// For decrypt servers: if a rule has aggregate + merge_server, forward → merge server address.
// For merge servers: forward → actual destination.
func buildRoutePayloads() (decryptRoutes []routePayload, mergeRoutes []routePayload) {
	var routes []model.Route
	database.DB.Where("enabled = ?", true).Find(&routes)

	// Load rules to check for aggregate/merge_server overrides
	var rules []model.DecorationRule
	database.DB.Where("enabled = ?", true).Find(&rules)

	// Load merge servers
	var mergeServers []model.MergeServer
	database.DB.Where("enabled = ?", true).Find(&mergeServers)
	mergeMap := make(map[uint]*model.MergeServer)
	for i := range mergeServers {
		mergeMap[mergeServers[i].ID] = &mergeServers[i]
	}

	// Build a map: route_id → merge server address (if aggregate is enabled)
	routeMergeAddr := make(map[uint]string)
	for _, r := range rules {
		if r.Aggregate && r.MergeServerID != nil {
			if ms, ok := mergeMap[*r.MergeServerID]; ok {
				routeMergeAddr[r.RouteID] = ensurePort(ms.Address, "19443")
			}
		}
	}

	for _, route := range routes {
		if mergeAddr, hasMerge := routeMergeAddr[route.ID]; hasMerge {
			// Aggregate route: decrypt server → merge server, merge server → actual forward
			decryptRoutes = append(decryptRoutes, routePayload{UUID: route.UUID, Forward: mergeAddr})
			mergeRoutes = append(mergeRoutes, routePayload{UUID: route.UUID, Forward: route.Forward})
		} else {
			// Direct route: decrypt server → actual forward
			decryptRoutes = append(decryptRoutes, routePayload{UUID: route.UUID, Forward: route.Forward})
		}
	}
	return
}

func ensureSyncPort(addr string, defaultPort string) string {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

func pushRoutes(adminAddr, adminToken string, routes []routePayload) error {
	if adminAddr == "" {
		return fmt.Errorf("no admin_addr")
	}

	body, _ := json.Marshal(map[string]interface{}{"routes": routes})

	url := fmt.Sprintf("http://%s/api/routes", adminAddr)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}

	resp, err := syncClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SyncRoutesToServer pushes routes to a specific decrypt server.
func SyncRoutesToServer(srv *model.DecryptServer) error {
	decryptRoutes, _ := buildRoutePayloads()
	if err := pushRoutes(srv.AdminAddr, srv.AdminToken, decryptRoutes); err != nil {
		database.DB.Model(srv).Update("status", "error")
		return err
	}
	database.DB.Model(srv).Update("status", "synced")
	return nil
}

// GetServerStatus queries a decrypt server's health.
func GetServerStatus(srv *model.DecryptServer) (map[string]interface{}, error) {
	if srv.AdminAddr == "" {
		return nil, fmt.Errorf("no admin_addr configured")
	}

	url := fmt.Sprintf("http://%s/api/status", srv.AdminAddr)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if srv.AdminToken != "" {
		req.Header.Set("Authorization", "Bearer "+srv.AdminToken)
	}

	resp, err := syncClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// SyncAllRoutes pushes routes to all enabled decrypt and merge servers.
func SyncAllRoutes() map[string]string {
	decryptRoutes, mergeRoutes := buildRoutePayloads()
	results := make(map[string]string)

	// Sync decrypt servers
	var decryptServers []model.DecryptServer
	database.DB.Where("enabled = ? AND admin_addr != ''", true).Find(&decryptServers)
	for _, srv := range decryptServers {
		if err := pushRoutes(srv.AdminAddr, srv.AdminToken, decryptRoutes); err != nil {
			database.DB.Model(&srv).Update("status", "error")
			results[srv.Name] = "error: " + err.Error()
		} else {
			database.DB.Model(&srv).Update("status", "synced")
			results[srv.Name] = "ok"
		}
	}

	// Sync merge servers (only if there are merge routes)
	if len(mergeRoutes) > 0 {
		var mergeServers []model.MergeServer
		database.DB.Where("enabled = ? AND admin_addr != ''", true).Find(&mergeServers)
		for _, srv := range mergeServers {
			if err := pushRoutes(srv.AdminAddr, srv.AdminToken, mergeRoutes); err != nil {
				database.DB.Model(&srv).Update("status", "error")
				results["merge:"+srv.Name] = "error: " + err.Error()
			} else {
				database.DB.Model(&srv).Update("status", "synced")
				results["merge:"+srv.Name] = "ok"
			}
		}
	}

	return results
}

// SyncAllRoutesAsync pushes routes to all servers in background.
func SyncAllRoutesAsync() {
	go func() {
		SyncAllRoutes()
		MarkSynced()
	}()
}
