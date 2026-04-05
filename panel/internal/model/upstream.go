package model

import "time"

type Upstream struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:128;not null" json:"name"`
	URL           string    `gorm:"size:512;not null" json:"url"`
	Domain        string    `gorm:"size:256;uniqueIndex;not null" json:"domain"`
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
