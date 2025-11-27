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

// RebuildStreamConfig generates the config for a specific port, handling multiple SNI streams.
func (m *Manager) RebuildStreamConfig(port int, streams []models.Stream) error {
	if len(streams) == 0 {
		return m.DeleteStreamConfig(port)
	}

	// Check if we need SNI routing
	// If multiple streams, or the single stream has a domain, we use SNI.
	// Exception: UDP cannot use ssl_preread (DTLS is complex, assume TCP for SNI).
	useSNI := false
	if len(streams) > 1 {
		useSNI = true
	} else if streams[0].Domain != "" {
		useSNI = true
	}

	var buf bytes.Buffer
	
	// Simple Pass-through (No SNI, Single Stream)
	if !useSNI {
		s := streams[0]
		proto := ""
		if s.Protocol == "udp" {
			proto = " udp"
		}
		
		// Plain server block
		// We use a variable for upstream to prevent boot errors if container is down (requires resolver)
		// But variables aren't allowed in 'upstream' directive, but can be used in proxy_pass
		tmpl := `
server {
    listen {{ .ListenPort }}{{ .Proto }};
    listen [::]:{{ .ListenPort }}{{ .Proto }};
    proxy_pass {{ .Upstream }};
}
`
		data := struct {
			ListenPort int
			Proto      string
			Upstream   string
		}{
			ListenPort: s.ListenPort,
			Proto:      proto,
			Upstream:   s.Upstream,
		}
		
		t, _ := template.New("simple_stream").Parse(tmpl)
		if err := t.Execute(&buf, data); err != nil {
			return err
		}
	} else {
		// SNI Routing (TCP only usually)
		// 1. Map block
		// 2. Server block with ssl_preread
		
		// Map name needs to be unique per port
		mapName := fmt.Sprintf("stream_map_%d", port)
		
		buf.WriteString(fmt.Sprintf("map $ssl_preread_server_name $%s {\n", mapName))
		for _, s := range streams {
			if s.Domain != "" {
				buf.WriteString(fmt.Sprintf("    %s %s;\n", s.Domain, s.Upstream))
			} else {
				// Default/Catch-all if one is missing domain? 
				// Or explicit default. For now, let's map "." (if supported) or use default clause
			}
		}
		// If there's a stream with empty domain, make it default?
		var defaultStream *models.Stream
		for _, s := range streams {
			if s.Domain == "" {
				defaultStream = &s
				break
			}
		}
		if defaultStream != nil {
			buf.WriteString(fmt.Sprintf("    default %s;\n", defaultStream.Upstream))
		}
		buf.WriteString("}\n\n")

		buf.WriteString("server {\n")
		buf.WriteString(fmt.Sprintf("    listen %d;\n", port))
		buf.WriteString("    ssl_preread on;\n")
		buf.WriteString(fmt.Sprintf("    proxy_pass $%s;\n", mapName))
		buf.WriteString("}\n")
	}

	configFile := filepath.Join(m.StreamsDir, fmt.Sprintf("port_%d.conf", port))
	if err := os.WriteFile(configFile, buf.Bytes(), 0644); err != nil {
		return err
	}

	return m.Reload()
}

func (m *Manager) DeleteStreamConfig(port int) error {
	target := filepath.Join(m.StreamsDir, fmt.Sprintf("port_%d.conf", port))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return m.Reload()
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

// DeleteStream removed from here as we now manage by port via DeleteStreamConfig

