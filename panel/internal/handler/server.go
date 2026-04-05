package handler

import (
	"net"
	"net/http"
	"time"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
	"shieldlink-panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// deriveAdminAddr extracts IP from "ip:port" and appends admin port 19480.
func deriveAdminAddr(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	return net.JoinHostPort(host, "19480")
}

// deriveSSHHost extracts IP from "ip:port" if ssh_host is empty.
func deriveSSHHost(address, sshHost string) string {
	if sshHost != "" {
		return sshHost
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}

func generateToken() string {
	return uuid.New().String()
}

// ==================== Decrypt Servers ====================

func ListDecryptServers(c *gin.Context) {
	var items []model.DecryptServer
	database.DB.Order("id asc").Find(&items)
	c.JSON(http.StatusOK, items)
}

func CreateDecryptServer(c *gin.Context) {
	var req model.DecryptServer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.AdminAddr = deriveAdminAddr(req.Address)
	req.AdminToken = generateToken()
	req.SSHHost = deriveSSHHost(req.Address, req.SSHHost)
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	database.DB.Create(&req)
	c.JSON(http.StatusCreated, req)
}

func UpdateDecryptServer(c *gin.Context) {
	var item model.DecryptServer
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req map[string]interface{}
	c.ShouldBindJSON(&req)
	// If address changed, re-derive admin_addr and ssh_host
	if addr, ok := req["address"].(string); ok && addr != "" {
		req["admin_addr"] = deriveAdminAddr(addr)
		if _, hasSsh := req["ssh_host"]; !hasSsh || req["ssh_host"] == "" {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			req["ssh_host"] = host
		}
	}
	database.DB.Model(&item).Updates(req)
	database.DB.First(&item, item.ID)
	c.JSON(http.StatusOK, item)
}

func DeleteDecryptServer(c *gin.Context) {
	database.DB.Delete(&model.DecryptServer{}, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func SyncDecryptServer(c *gin.Context) {
	var srv model.DecryptServer
	if err := database.DB.First(&srv, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err := service.SyncRoutesToServer(&srv); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "synced"})
}

func GetDecryptServerStatus(c *gin.Context) {
	var srv model.DecryptServer
	if err := database.DB.First(&srv, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	status, err := service.GetServerStatus(&srv)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// ==================== Merge Servers ====================

func ListMergeServers(c *gin.Context) {
	var items []model.MergeServer
	database.DB.Order("id asc").Find(&items)
	c.JSON(http.StatusOK, items)
}

func CreateMergeServer(c *gin.Context) {
	var req model.MergeServer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.AdminAddr = deriveAdminAddr(req.Address)
	req.AdminToken = generateToken()
	req.SSHHost = deriveSSHHost(req.Address, req.SSHHost)
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	database.DB.Create(&req)
	c.JSON(http.StatusCreated, req)
}

func UpdateMergeServer(c *gin.Context) {
	var item model.MergeServer
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req map[string]interface{}
	c.ShouldBindJSON(&req)
	if addr, ok := req["address"].(string); ok && addr != "" {
		req["admin_addr"] = deriveAdminAddr(addr)
		if _, hasSsh := req["ssh_host"]; !hasSsh || req["ssh_host"] == "" {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			req["ssh_host"] = host
		}
	}
	database.DB.Model(&item).Updates(req)
	database.DB.First(&item, item.ID)
	c.JSON(http.StatusOK, item)
}

func DeleteMergeServer(c *gin.Context) {
	database.DB.Delete(&model.MergeServer{}, c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ==================== Check & Auto-Install ====================

func checkInstallAndUpdate(c *gin.Context, sshInfo model.SSHInfo, updateFn func(status string)) {
	result, err := service.CheckAndInstall(sshInfo)
	if err != nil {
		updateFn("error")
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	status := "installed"
	if !result.Installed {
		status = "not_installed"
	}
	updateFn(status)
	c.JSON(http.StatusOK, result)
}

func CheckInstallDecryptServer(c *gin.Context) {
	var srv model.DecryptServer
	if err := database.DB.First(&srv, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	now := time.Now()
	checkInstallAndUpdate(c, &srv, func(s string) {
		database.DB.Model(&srv).Updates(map[string]interface{}{
			"install_status":  s,
			"last_checked_at": now,
		})
		// Sync routes after successful install check
		if s == "installed" {
			service.SyncRoutesToServer(&srv)
		}
	})
}

func CheckInstallMergeServer(c *gin.Context) {
	var srv model.MergeServer
	if err := database.DB.First(&srv, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	now := time.Now()
	checkInstallAndUpdate(c, &srv, func(s string) {
		database.DB.Model(&srv).Updates(map[string]interface{}{
			"install_status":  s,
			"last_checked_at": now,
		})
	})
}

func CheckInstallAllDecryptServers(c *gin.Context) {
	var servers []model.DecryptServer
	database.DB.Where("enabled = ? AND ssh_host != ''", true).Find(&servers)

	now := time.Now()
	results := make(map[string]interface{})
	for _, srv := range servers {
		result, err := service.CheckAndInstall(&srv)
		if err != nil {
			database.DB.Model(&srv).Updates(map[string]interface{}{"install_status": "error", "last_checked_at": now})
			results[srv.Name] = gin.H{"error": err.Error()}
		} else {
			status := "installed"
			if !result.Installed {
				status = "not_installed"
			}
			database.DB.Model(&srv).Updates(map[string]interface{}{"install_status": status, "last_checked_at": now})
			results[srv.Name] = result
		}
	}
	c.JSON(http.StatusOK, results)
}

func CheckInstallAllMergeServers(c *gin.Context) {
	var servers []model.MergeServer
	database.DB.Where("enabled = ? AND ssh_host != ''", true).Find(&servers)

	now := time.Now()
	results := make(map[string]interface{})
	for _, srv := range servers {
		result, err := service.CheckAndInstall(&srv)
		if err != nil {
			database.DB.Model(&srv).Updates(map[string]interface{}{"install_status": "error", "last_checked_at": now})
			results[srv.Name] = gin.H{"error": err.Error()}
		} else {
			status := "installed"
			if !result.Installed {
				status = "not_installed"
			}
			database.DB.Model(&srv).Updates(map[string]interface{}{"install_status": status, "last_checked_at": now})
			results[srv.Name] = result
		}
	}
	c.JSON(http.StatusOK, results)
}
