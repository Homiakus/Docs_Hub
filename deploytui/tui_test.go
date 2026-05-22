package deploytui

import (
	"strings"
	"testing"
)

func TestRenderFilesIncludesServeModeAndTLSProxy(t *testing.T) {
	files, err := renderFiles(defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		byPath[file.Path] = file.Content
	}

	service := byPath["deploy/docs-hub.service"]
	if !strings.Contains(service, "ExecStart=/opt/docs-hub/docs-hub serve") {
		t.Fatalf("systemd service does not run serve mode:\n%s", service)
	}

	watchdog := byPath["deploy/docs-hub-watchdog.sh"]
	if !strings.Contains(watchdog, "curl -fsS --max-time 5") || !strings.Contains(watchdog, "systemctl restart") {
		t.Fatalf("watchdog does not check health and restart service:\n%s", watchdog)
	}

	caddy := byPath["deploy/Caddyfile"]
	if !strings.Contains(caddy, "docs.example.com") || !strings.Contains(caddy, "reverse_proxy 127.0.0.1:8080") {
		t.Fatalf("caddy config does not proxy the configured domain:\n%s", caddy)
	}
}
