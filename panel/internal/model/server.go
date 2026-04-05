package model

import "time"

type DecryptServer struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:128;not null" json:"name"`
	Address       string    `gorm:"size:256;not null" json:"address"`
	Protocol      string    `gorm:"size:16;not null;default:tcp" json:"protocol"`
	AdminAddr     string    `gorm:"size:256" json:"admin_addr"`
	AdminToken    string    `gorm:"size:256" json:"admin_token"`
	SSHHost       string    `gorm:"size:256" json:"ssh_host"`
	SSHPort       int       `gorm:"default:22" json:"ssh_port"`
	SSHUser       string    `gorm:"size:128;default:root" json:"ssh_user"`
	SSHPassword   string    `gorm:"size:256" json:"ssh_password"`
	CheckInterval int       `gorm:"default:0" json:"check_interval"`
	InstallStatus string    `gorm:"size:32;default:unknown" json:"install_status"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	Status        string    `gorm:"size:32;default:unknown" json:"status"`
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MergeServer struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:128;not null" json:"name"`
	Address       string    `gorm:"size:256;not null" json:"address"`
	AdminAddr     string    `gorm:"size:256" json:"admin_addr"`
	AdminToken    string    `gorm:"size:256" json:"admin_token"`
	SSHHost       string    `gorm:"size:256" json:"ssh_host"`
	SSHPort       int       `gorm:"default:22" json:"ssh_port"`
	SSHUser       string    `gorm:"size:128;default:root" json:"ssh_user"`
	SSHPassword   string    `gorm:"size:256" json:"ssh_password"`
	CheckInterval int       `gorm:"default:0" json:"check_interval"`
	InstallStatus string    `gorm:"size:32;default:unknown" json:"install_status"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	Status        string    `gorm:"size:32;default:unknown" json:"status"`
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SSHInfo is a common interface for servers that support SSH operations.
type SSHInfo interface {
	GetSSHHost() string
	GetSSHPort() int
	GetSSHUser() string
	GetSSHPassword() string
	GetAdminAddr() string
	GetAdminToken() string
}

func (s *DecryptServer) GetSSHHost() string    { return s.SSHHost }
func (s *DecryptServer) GetSSHPort() int        { return s.SSHPort }
func (s *DecryptServer) GetSSHUser() string     { return s.SSHUser }
func (s *DecryptServer) GetSSHPassword() string { return s.SSHPassword }
func (s *DecryptServer) GetAdminAddr() string   { return s.AdminAddr }
func (s *DecryptServer) GetAdminToken() string  { return s.AdminToken }

func (s *MergeServer) GetSSHHost() string    { return s.SSHHost }
func (s *MergeServer) GetSSHPort() int        { return s.SSHPort }
func (s *MergeServer) GetSSHUser() string     { return s.SSHUser }
func (s *MergeServer) GetSSHPassword() string { return s.SSHPassword }
func (s *MergeServer) GetAdminAddr() string   { return s.AdminAddr }
func (s *MergeServer) GetAdminToken() string  { return s.AdminToken }
