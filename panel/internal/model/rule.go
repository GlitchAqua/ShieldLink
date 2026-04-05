package model

import "time"

type DecorationRule struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UpstreamID    uint      `gorm:"index;not null" json:"upstream_id"`
	Name          string    `gorm:"size:128;not null" json:"name"`
	MatchPattern  string    `gorm:"size:256;not null" json:"match_pattern"`
	UAPattern     string    `gorm:"size:256" json:"ua_pattern"`
	RouteID       uint      `gorm:"not null" json:"route_id"`
	ServerIDs     string    `gorm:"size:512;not null" json:"server_ids"`
	Protocol      string    `gorm:"size:16;not null;default:tcp" json:"protocol"`
	Transport     string    `gorm:"size:16;not null;default:h2" json:"transport"`
	MPTCP         bool      `gorm:"default:false" json:"mptcp"`
	Aggregate     bool      `gorm:"default:false" json:"aggregate"`
	MergeServerID *uint     `json:"merge_server_id"`
	Priority      int       `gorm:"default:0" json:"priority"`
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	Upstream    Upstream       `gorm:"foreignKey:UpstreamID" json:"upstream,omitempty"`
	Route       Route          `gorm:"foreignKey:RouteID" json:"route,omitempty"`
	MergeServer *MergeServer   `gorm:"foreignKey:MergeServerID" json:"merge_server,omitempty"`
}
