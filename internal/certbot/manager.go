package certbot

import (
	"fmt"
	"os/exec"
)

type Manager struct {
	Webroot string
	Email   string
}

func NewManager(webroot, email string) *Manager {
	return &Manager{
		Webroot: webroot,
		Email:   email,
	}
}

func (m *Manager) Issue(domain string) error {
	// certbot certonly --webroot -w /var/www/hubfly -d example.com --non-interactive --agree-tos -m email
	path, err := exec.LookPath("certbot")
	if err != nil {
		return fmt.Errorf("certbot not found")
	}

	args := []string{
		"certonly",
		"--webroot",
		"-w", m.Webroot,
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
		"-m", m.Email,
	}

	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed: %s, output: %s", err, string(out))
	}
	return nil
}

func (m *Manager) Revoke(domain string) error {
	// certbot revoke --cert-path ...
	// For simplicity, we assume standard letsencrypt path
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/cert.pem", domain)

	path, err := exec.LookPath("certbot")
	if err != nil {
		return fmt.Errorf("certbot not found")
	}

	cmd := exec.Command(path, "revoke", "--cert-path", certPath, "--reason", "unspecified", "--non-interactive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot revoke failed: %s, output: %s", err, string(out))
	}
	return nil
}
