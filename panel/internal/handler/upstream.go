package handler

import (
	"net/http"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"

	"github.com/gin-gonic/gin"
)

func ListUpstreams(c *gin.Context) {
	var items []model.Upstream
	database.DB.Order("id asc").Find(&items)
	c.JSON(http.StatusOK, items)
}

func CreateUpstream(c *gin.Context) {
	var req model.Upstream
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := database.DB.Create(&req).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
		return
	}
	c.JSON(http.StatusCreated, req)
}

func UpdateUpstream(c *gin.Context) {
	var item model.Upstream
	if err := database.DB.First(&item, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	database.DB.Model(&item).Updates(req)
	database.DB.First(&item, item.ID)
	c.JSON(http.StatusOK, item)
}

func DeleteUpstream(c *gin.Context) {
	if err := database.DB.Delete(&model.Upstream{}, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
