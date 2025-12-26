package nginx

import (
	"os"
	"strings"
	"testing"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

func TestFirewallIPRules(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nginx_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	if err := mgr.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	site := &models.Site{
		ID:        "test-firewall",
		Domain:    "firewall.local",
		Upstreams: []string{"127.0.0.1:8080"},
		Firewall: &models.FirewallConfig{
			IPRules: []models.IPRule{
				{Action: "allow", Value: "192.168.1.100"},
				{Action: "deny", Value: "192.168.1.0/24"},
				{Action: "allow", Value: "all"},
			},
		},
	}

	configFile, err := mgr.GenerateConfig(site)
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	configStr := string(content)

	expectedRules := []string{
		"allow 192.168.1.100;",
		"deny 192.168.1.0/24;",
		"allow all;",
	}

	for _, rule := range expectedRules {
		if !strings.Contains(configStr, rule) {
			t.Errorf("Config missing rule: %s", rule)
		}
	}
}
