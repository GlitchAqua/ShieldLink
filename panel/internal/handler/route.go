package handler

import (
	"net/http"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
	"shieldlink-panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ListRoutes(c *gin.Context) {
	var items []model.Route
	q := database.DB.Preload("Upstream").Order("id asc")
	if uid := c.Query("upstream_id"); uid != "" {
		q = q.Where("upstream_id = ?", uid)
	}
	q.Find(&items)
	c.JSON(http.StatusOK, items)
}

func CreateRoute(c *gin.Context) {
	var req struct {
		UpstreamID uint   `json:"upstream_id" binding:"required"`
		Forward    string `json:"forward" binding:"required"`
		Remark     string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	route := model.Route{
		UpstreamID: req.UpstreamID,
		UUID:       "sl-" + uuid.New().String()[:8],
		Forward:    req.Forward,
		Remark:     req.Remark,
		Enabled:    true,
	}
	database.DB.Create(&route)
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusCreated, route)
}

func UpdateRoute(c *gin.Context) {
	var item model.Route
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req map[string]interface{}
	c.ShouldBindJSON(&req)
	delete(req, "uuid") // UUID cannot be changed
	database.DB.Model(&item).Updates(req)
	database.DB.Preload("Upstream").First(&item, item.ID)
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusOK, item)
}

func DeleteRoute(c *gin.Context) {
	database.DB.Delete(&model.Route{}, c.Param("id"))
	service.SyncAllRoutesAsync()
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func SyncAllRoutes(c *gin.Context) {
	results := service.SyncAllRoutes()
	c.JSON(http.StatusOK, results)
}
