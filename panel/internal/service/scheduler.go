package service

import (
	"log"
	"sync"
	"time"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
)

const syncInterval = 5 * time.Minute

var lastSyncTime time.Time
var syncMu sync.Mutex

// StartCheckScheduler starts background goroutines:
// 1. Check install status per server's check_interval
// 2. Fallback route sync every 5 minutes
func StartCheckScheduler() {
	// Check install loop (every 5s polling)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			checkDueServers()
		}
	}()

	// Fallback sync loop (every 5 minutes)
	go func() {
		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()
		for range ticker.C {
			syncMu.Lock()
			if time.Since(lastSyncTime) >= syncInterval {
				log.Println("[scheduler] fallback route sync")
				SyncAllRoutes()
				lastSyncTime = time.Now()
			}
			syncMu.Unlock()
		}
	}()

	log.Println("Scheduler started: install check (5s poll), route sync (5min fallback)")
}

// MarkSynced records that a sync just happened (called by SyncAllRoutesAsync).
func MarkSynced() {
	syncMu.Lock()
	lastSyncTime = time.Now()
	syncMu.Unlock()
}

func checkDueServers() {
	now := time.Now()

	var decryptServers []model.DecryptServer
	database.DB.Where("enabled = ? AND ssh_host != '' AND ssh_password != '' AND check_interval > 0", true).Find(&decryptServers)

	for _, srv := range decryptServers {
		if !isDue(srv.LastCheckedAt, srv.CheckInterval, now) {
			continue
		}

		result, err := CheckAndInstall(&srv)
		status := "installed"
		if err != nil {
			log.Printf("[scheduler] %s: check error: %v", srv.Name, err)
			status = "error"
		} else if !result.Installed {
			log.Printf("[scheduler] %s: %s - %s", srv.Name, result.Action, result.Error)
			status = "not_installed"
		}

		database.DB.Model(&srv).Updates(map[string]interface{}{
			"install_status":  status,
			"last_checked_at": now,
		})
	}

	// Merge servers
	var mergeServers []model.MergeServer
	database.DB.Where("enabled = ? AND ssh_host != '' AND ssh_password != '' AND check_interval > 0", true).Find(&mergeServers)

	for _, srv := range mergeServers {
		if !isDue(srv.LastCheckedAt, srv.CheckInterval, now) {
			continue
		}
		result, err := CheckAndInstall(&srv)
		status := "installed"
		if err != nil {
			log.Printf("[scheduler] %s: check error: %v", srv.Name, err)
			status = "error"
		} else if !result.Installed {
			log.Printf("[scheduler] %s: %s - %s", srv.Name, result.Action, result.Error)
			status = "not_installed"
		}
		database.DB.Model(&srv).Updates(map[string]interface{}{
			"install_status":  status,
			"last_checked_at": now,
		})
	}
}

func isDue(lastChecked *time.Time, intervalSeconds int, now time.Time) bool {
	if intervalSeconds <= 0 {
		return false
	}
	if lastChecked == nil {
		return true
	}
	return now.Sub(*lastChecked) >= time.Duration(intervalSeconds)*time.Second
}
