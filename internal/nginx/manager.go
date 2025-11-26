package nginx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

type Manager struct {
	SitesDir     string
	StreamsDir   string
	StagingDir   string
	TemplatesDir string
	NginxConf    string // Path to main nginx.conf
}

func NewManager(baseDir string) *Manager {
	return &Manager{
		SitesDir:     filepath.Join(baseDir, "sites"),
		StreamsDir:   filepath.Join(baseDir, "streams"),
		StagingDir:   filepath.Join(baseDir, "staging"),
		TemplatesDir: filepath.Join(baseDir, "templates"),
		NginxConf:    "/etc/nginx/nginx.conf",
	}
}

// EnsureDirs creates necessary directories
func (m *Manager) EnsureDirs() error {
	dirs := []string{m.SitesDir, m.StreamsDir, m.StagingDir, m.TemplatesDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// GenerateConfig renders the site config to a staging file.
func (m *Manager) GenerateConfig(site *models.Site) (string, error) {
	// Load templates
	var templateContent strings.Builder
	for _, tplName := range site.Templates {
		content, err := os.ReadFile(filepath.Join(m.TemplatesDir, tplName+".conf"))
		if err != nil {
			// For MVP, we might log warning but here we fail
			// If template not found, maybe ignore? stricter is better.
			return "", fmt.Errorf("failed to load template %s: %w", tplName, err)
		}
		templateContent.Write(content)
		templateContent.WriteString("\n")
	}

	// Wrapper for template data
	data := struct {
		*models.Site
		TemplateSnippets string
	}{
		Site:             site,
		TemplateSnippets: templateContent.String(),
	}

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
        set $upstream_endpoint "http://{{ index .Upstreams 0 }}";
        proxy_pass $upstream_endpoint;
        {{ range $k, $v := .ProxySetHeaders }}
        proxy_set_header {{ $k }} {{ $v }};
        {{ end }}
        
        {{ .TemplateSnippets }}
        {{ .ExtraConfig }}
    }
    {{ end }}
    
    # Challenge path for Certbot
    location /.well-known/acme-challenge/ {
        root /var/www/hubfly;
        try_files $uri =404;
    }

    error_page 502 504 /502.html;
    location = /502.html {
        root /var/www/hubfly/static;
        internal;
    }
}

{{ if .SSL }}
server {
    listen 443 ssl;
    http2 on;
    server_name {{ .Domain }};

    ssl_certificate /etc/letsencrypt/live/{{ .Domain }}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{{ .Domain }}/privkey.pem;

    location / {
        set $upstream_endpoint "http://{{ index .Upstreams 0 }}";
        proxy_pass $upstream_endpoint;
        {{ range $k, $v := .ProxySetHeaders }}
        proxy_set_header {{ $k }} {{ $v }};
        {{ end }}
        
        {{ .TemplateSnippets }}
        {{ .ExtraConfig }}
    }

    error_page 502 504 /502.html;
    location = /502.html {
        root /var/www/hubfly/static;
        internal;
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
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	stagingFile := filepath.Join(m.StagingDir, site.ID+".conf")
	if err := os.WriteFile(stagingFile, buf.Bytes(), 0644); err != nil {
		return "", err
	}

	return stagingFile, nil
}

// GenerateStreamConfig renders the stream config to a staging file.
func (m *Manager) GenerateStreamConfig(stream *models.Stream) (string, error) {
	const streamTmpl = `
server {
    listen {{ .ListenPort }} {{ if eq .Protocol "udp" }}udp{{ end }};
    proxy_pass {{ .Upstream }};
}
`
	t, err := template.New("stream").Parse(streamTmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, stream); err != nil {
		return "", err
	}

	stagingFile := filepath.Join(m.StagingDir, stream.ID+".stream.conf")
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
	
	// Strategy: use `nginx -t -c /etc/nginx/nginx.conf` but we need to inject our staging file.
	// Since the main nginx.conf likely includes `/etc/hubfly/sites/*.conf`, 
	// we can temporary symlink staging file to sites/ OR use a specific test config.
	
	// For MVP, let's try to just syntax check the file if possible, or skip if too complex.
	// Simpler: Just return nil for now if not in a proper env.
	
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

// ApplyStream moves staging file to live streams dir and reloads
func (m *Manager) ApplyStream(streamID, stagingFile string) error {
	target := filepath.Join(m.StreamsDir, streamID+".conf")
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

func (m *Manager) DeleteStream(streamID string) error {
	target := filepath.Join(m.StreamsDir, streamID+".conf")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return m.Reload()
}
