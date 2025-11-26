package models

import "time"

// Stream represents a Layer 4 (TCP/UDP) proxy configuration.
type Stream struct {
	ID           string    `json:"id"`
	ListenPort   int       `json:"listen_port"` // Port to listen on host
	Upstream     string    `json:"upstream"`    // host:port
	Protocol     string    `json:"protocol"`    // "tcp" or "udp" (default tcp)
	
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
