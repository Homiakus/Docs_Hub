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

func TestAutoDetectConfigKeepsCustomBinaryPath(t *testing.T) {
	cfg := defaultConfig()
	cfg.BinaryPath = "/srv/docs-hub/custom-docs-hub"

	got := autoDetectConfig(cfg)

	if got.BinaryPath != cfg.BinaryPath {
		t.Fatalf("custom binary path was overwritten: got %q want %q", got.BinaryPath, cfg.BinaryPath)
	}
}

func TestSudoInstallCommandsQuotePaths(t *testing.T) {
	cfg := defaultConfig()
	cfg.InstallDir = "/opt/docs hub"
	cfg.BinaryPath = "/opt/docs hub/docs-hub"
	cfg.UseCaddy = false

	commands := strings.Join(sudoInstallCommands(cfg), "\n")

	if !strings.Contains(commands, "'/opt/docs hub'") {
		t.Fatalf("install dir with spaces is not shell-quoted:\n%s", commands)
	}
	if !strings.Contains(commands, "'/opt/docs hub/docs-hub'") {
		t.Fatalf("binary path with spaces is not shell-quoted:\n%s", commands)
	}
}
