package handler

import (
	"net/http"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"

	"github.com/gin-gonic/gin"
)

func Dashboard(c *gin.Context) {
	var upstreamCount, serverCount, mergeCount, routeCount, ruleCount int64
	database.DB.Model(&model.Upstream{}).Count(&upstreamCount)
	database.DB.Model(&model.DecryptServer{}).Count(&serverCount)
	database.DB.Model(&model.MergeServer{}).Count(&mergeCount)
	database.DB.Model(&model.Route{}).Count(&routeCount)
	database.DB.Model(&model.DecorationRule{}).Count(&ruleCount)

	c.JSON(http.StatusOK, gin.H{
		"upstreams":       upstreamCount,
		"decrypt_servers": serverCount,
		"merge_servers":   mergeCount,
		"routes":          routeCount,
		"rules":           ruleCount,
	})
}
