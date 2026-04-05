package handler

import (
	"net/http"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
	"shieldlink-panel/internal/service"

	"github.com/gin-gonic/gin"
)

func ListRules(c *gin.Context) {
	var items []model.DecorationRule
	q := database.DB.Preload("Upstream").Preload("Route").Preload("MergeServer").Order("priority desc, id asc")
	if uid := c.Query("upstream_id"); uid != "" {
		q = q.Where("upstream_id = ?", uid)
	}
	q.Find(&items)
	c.JSON(http.StatusOK, items)
}

func CreateRule(c *gin.Context) {
	var req model.DecorationRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Enabled = true
	database.DB.Create(&req)
	database.DB.Preload("Upstream").Preload("Route").Preload("MergeServer").First(&req, req.ID)
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusCreated, req)
}

func UpdateRule(c *gin.Context) {
	var item model.DecorationRule
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req struct {
		Name          *string `json:"name"`
		UpstreamID    *uint   `json:"upstream_id"`
		MatchPattern  *string `json:"match_pattern"`
		UAPattern     *string `json:"ua_pattern"`
		RouteID       *uint   `json:"route_id"`
		ServerIDs     *string `json:"server_ids"`
		Protocol      *string `json:"protocol"`
		Transport     *string `json:"transport"`
		MPTCP         *bool   `json:"mptcp"`
		Aggregate     *bool   `json:"aggregate"`
		MergeServerID *uint   `json:"merge_server_id"`
		Priority      *int    `json:"priority"`
		Enabled       *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.UpstreamID != nil {
		updates["upstream_id"] = *req.UpstreamID
	}
	if req.MatchPattern != nil {
		updates["match_pattern"] = *req.MatchPattern
	}
	if req.UAPattern != nil {
		updates["ua_pattern"] = *req.UAPattern
	}
	if req.RouteID != nil {
		updates["route_id"] = *req.RouteID
	}
	if req.ServerIDs != nil {
		updates["server_ids"] = *req.ServerIDs
	}
	if req.Protocol != nil {
		updates["protocol"] = *req.Protocol
	}
	if req.Transport != nil {
		updates["transport"] = *req.Transport
	}
	if req.MPTCP != nil {
		updates["mptcp"] = *req.MPTCP
	}
	if req.Aggregate != nil {
		updates["aggregate"] = *req.Aggregate
	}
	if req.MergeServerID != nil {
		updates["merge_server_id"] = *req.MergeServerID
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	database.DB.Model(&item).Updates(updates)
	database.DB.Preload("Upstream").Preload("Route").Preload("MergeServer").First(&item, item.ID)
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusOK, item)
}

func DeleteRule(c *gin.Context) {
	database.DB.Delete(&model.DecorationRule{}, c.Param("id"))
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
