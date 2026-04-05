package model

import "time"

type Route struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UpstreamID uint      `gorm:"index;not null" json:"upstream_id"`
	UUID       string    `gorm:"size:128;uniqueIndex;not null" json:"uuid"`
	Forward    string    `gorm:"size:256;not null" json:"forward"`
	Remark     string    `gorm:"size:256" json:"remark"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Upstream Upstream `gorm:"foreignKey:UpstreamID" json:"upstream,omitempty"`
}
