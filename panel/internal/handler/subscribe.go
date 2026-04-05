package handler

import (
	"net/http"
	"strings"

	"shieldlink-panel/internal/database"
	"shieldlink-panel/internal/model"
	"shieldlink-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// HandleSubscribe is the subscription proxy endpoint.
// It matches the request Host to an upstream and proxies the subscription.
func HandleSubscribe(c *gin.Context) {
	host := c.Request.Host
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	var upstream model.Upstream
	if err := database.DB.Where("domain = ? AND enabled = ?", host, true).First(&upstream).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown domain: " + host})
		return
	}

	// Build the original path (everything after the domain)
	originalPath := c.Request.URL.Path
	queryString := c.Request.URL.RawQuery
	userAgent := c.Request.UserAgent()

	body, headers, statusCode, err := service.FetchAndDecorate(&upstream, originalPath, queryString, userAgent)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// Forward relevant headers (excluding content-type, we fix it below)
	forwardHeaders := []string{
		"subscription-userinfo",
		"profile-update-interval",
		"content-disposition",
		"profile-title",
		"profile-web-page-url",
	}
	for _, h := range forwardHeaders {
		if v := headers.Get(h); v != "" {
			c.Header(h, v)
		}
	}

	// Fix Content-Type: upstream often returns text/html which breaks clients.
	contentType := "text/plain; charset=utf-8"
	if len(body) > 0 {
		trimmed := strings.TrimSpace(string(body[:min(len(body), 200)]))
		if strings.HasPrefix(trimmed, "proxies:") || strings.HasPrefix(trimmed, "mixed-port:") || strings.HasPrefix(trimmed, "port:") || strings.HasPrefix(trimmed, "allow-lan:") {
			contentType = "text/yaml; charset=utf-8"
		}
	}

	c.Data(statusCode, contentType, body)
}
