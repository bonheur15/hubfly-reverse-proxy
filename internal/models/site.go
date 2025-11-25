package models

import (
	"time"
)

// Site represents a virtual host configuration.
type Site struct {
	ID              string            `json:"id"`
	Domain          string            `json:"domain"`
	Upstreams       []string          `json:"upstreams"`
	ForceSSL        bool              `json:"force_ssl"` // Redirect HTTP to HTTPS
	SSL             bool              `json:"ssl"`       // Enable SSL (requires cert)
	Templates       []string          `json:"templates"`
	ExtraConfig     string            `json:"extra_config,omitempty"`
	ProxySetHeaders map[string]string `json:"proxy_set_header,omitempty"`

	// Status fields
	Status          string    `json:"status"` // "active", "provisioning", "error"
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CertIssueStatus string    `json:"cert_issue_status,omitempty"` // "pending", "valid", "failed"
}

// APIResponse Standard API response wrapper (optional, but good for consistency)
type APIResponse struct {
	Error string      `json:"error,omitempty"`
	Code  int         `json:"code,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}
