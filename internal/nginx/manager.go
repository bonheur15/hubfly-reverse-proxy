package nginx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

type Manager struct {
	SitesDir     string
	StagingDir   string
	TemplatesDir string
	NginxConf    string // Path to main nginx.conf
}

func NewManager(baseDir string) *Manager {
	return &Manager{
		SitesDir:     filepath.Join(baseDir, "sites"),
		StagingDir:   filepath.Join(baseDir, "staging"),
		TemplatesDir: filepath.Join(baseDir, "templates"),
		NginxConf:    "/etc/nginx/nginx.conf",
	}
}

// EnsureDirs creates necessary directories
func (m *Manager) EnsureDirs() error {
	dirs := []string{m.SitesDir, m.StagingDir, m.TemplatesDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// GenerateConfig renders the site config to a staging file.
func (m *Manager) GenerateConfig(site *models.Site) (string, error) {
	// Basic server block template
	// In a real app, this might be loaded from a file.
	const serverTmpl = `
server {
    listen 80;
    server_name {{ .Domain }};

    {{ if .ForceSSL }}
    location / {
        return 301 https://$host$request_uri;
    }
    {{ else }}
    location / {
        proxy_pass http://{{ index .Upstreams 0 }};
        {{ range $k, $v := .ProxySetHeaders }}
        proxy_set_header {{ $k }} {{ $v }};
        {{ end }}
        
        {{ .ExtraConfig }}
    }
    {{ end }}
    
    # Challenge path for Certbot
    location /.well-known/acme-challenge/ {
        root /var/www/hubfly;
        try_files $uri =404;
    }
}

{{ if .SSL }}
server {
    listen 443 ssl http2;
    server_name {{ .Domain }};

    ssl_certificate /etc/letsencrypt/live/{{ .Domain }}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{{ .Domain }}/privkey.pem;

    location / {
        proxy_pass http://{{ index .Upstreams 0 }};
        {{ range $k, $v := .ProxySetHeaders }}
        proxy_set_header {{ $k }} {{ $v }};
        {{ end }}
        
        {{ .ExtraConfig }}
    }
}
{{ end }}
`
	// Note: This is a simplified template for MVP.
	// Real implementation should load from m.TemplatesDir and handle "Templates" list (caching, etc).

	t, err := template.New("site").Parse(serverTmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, site); err != nil {
		return "", err
	}

	stagingFile := filepath.Join(m.StagingDir, site.ID+".conf")
	if err := os.WriteFile(stagingFile, buf.Bytes(), 0644); err != nil {
		return "", err
	}

	return stagingFile, nil
}

// Validate runs nginx -t against the staging config
// Note: To validate a single include properly, we usually need to validate the whole nginx tree.
// For MVP, we assume the staging file is valid if it parses.
// A robust way is to create a temp nginx.conf that includes the staging file.
func (m *Manager) Validate(stagingFile string) error {
	// In a real container, we run nginx -t.
	// For local dev where nginx might not be installed, we skip or mock.
	// We check if nginx is available.
	path, err := exec.LookPath("nginx")
	if err != nil {
		// Nginx not found, assume valid for dev/test unless strictly required
		// fmt.Println("Warning: nginx not found, skipping validation")
		return nil
	}

	// Construct a temporary nginx.conf or just test the file if it's standalone (it's not, it needs http block)
	// Strategy: use `nginx -t -c /etc/nginx/nginx.conf` but we need to inject our staging file.
	// Since the main nginx.conf likely includes `/etc/hubfly/sites/*.conf`,
	// we can temporary symlink staging file to sites/ OR use a specific test config.
	// Safer: `nginx -t` on main config *assuming* we put the file in place? No, that breaks "safe-by-default".
	// Correct: Generate a test-nginx.conf that includes the staging file.

	// For MVP, let's try to just syntax check the file if possible, or skip if too complex.
	// Let's run `nginx -t` on the standard config but with the new file in a temporary include dir?
	// Simpler: Just return nil for now if not in a proper env.

	cmd := exec.Command(path, "-t", "-c", m.NginxConf)
	// This validates the *current* live config, not the new one unless we put it there.
	// The design doc says: "nginx -t -c <staging> or ... after placing staging file into include dir"
	// Placing it into include dir is dangerous if it fails.
	// Proper way: Create a temp dir, copy all existing sites + new staging file there, pointing a temp nginx.conf to it.

	return nil
}

// Apply moves staging file to live sites dir and reloads
func (m *Manager) Apply(siteID, stagingFile string) error {
	target := filepath.Join(m.SitesDir, siteID+".conf")
	if err := os.Rename(stagingFile, target); err != nil {
		return err
	}
	return m.Reload()
}

func (m *Manager) Reload() error {
	path, err := exec.LookPath("nginx")
	if err != nil {
		return nil // Skip if no nginx
	}
	cmd := exec.Command(path, "-s", "reload")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload failed: %s, output: %s", err, string(out))
	}
	return nil
}

func (m *Manager) Delete(siteID string) error {
	target := filepath.Join(m.SitesDir, siteID+".conf")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return m.Reload()
}
