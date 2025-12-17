package certbot

import (
	"fmt"
	"log/slog"
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

	slog.Info("Running certbot issue", "domain", domain, "command", path, "args", args)

	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	
	slog.Debug("Certbot output", "domain", domain, "output", string(out))

	if err != nil {
		slog.Error("Certbot issue failed", "domain", domain, "error", err, "output", string(out))
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

	slog.Info("Running certbot revoke", "domain", domain, "cert_path", certPath)

	cmd := exec.Command(path, "revoke", "--cert-path", certPath, "--reason", "unspecified", "--non-interactive")
	out, err := cmd.CombinedOutput()

	slog.Debug("Certbot revoke output", "domain", domain, "output", string(out))

	if err != nil {
		slog.Error("Certbot revoke failed", "domain", domain, "error", err, "output", string(out))
		return fmt.Errorf("certbot revoke failed: %s, output: %s", err, string(out))
	}
	return nil
}
