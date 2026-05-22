package deploytui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

const (
	configDir  = "deploy"
	configFile = "deploy/config.json"
)

type Config struct {
	SiteName      string `json:"site_name"`
	Domain        string `json:"domain"`
	Email         string `json:"email"`
	ServiceName   string `json:"service_name"`
	BinaryPath    string `json:"binary_path"`
	InstallDir    string `json:"install_dir"`
	DataDir       string `json:"data_dir"`
	ListenAddr    string `json:"listen_addr"`
	PublicPort    string `json:"public_port"`
	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password"`
	UseCaddy      bool   `json:"use_caddy"`
}

type fileSpec struct {
	Path    string
	Content string
	Mode    os.FileMode
}

var titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("212")).
	PaddingBottom(1)

var mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

func Run(out io.Writer) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, titleStyle.Render("Docs Hub deploy TUI"))
	fmt.Fprintln(out, mutedStyle.Render("Charm TUI для настройки systemd, Caddy HTTPS, health scan и watchdog."))

	for {
		var action string
		err := huh.NewSelect[string]().
			Title("Что сделать?").
			Options(
				huh.NewOption("Настроить деплой сайта", "configure"),
				huh.NewOption("Сгенерировать deploy-файлы", "render"),
				huh.NewOption("Установить/обновить systemd daemon", "install-systemd"),
				huh.NewOption("Проверить состояние сервиса", "scan"),
				huh.NewOption("Перезапустить, если health check не проходит", "restart-if-hung"),
				huh.NewOption("Настроить Caddy и получить SSL", "ssl"),
				huh.NewOption("Выход", "quit"),
			).
			Value(&action).
			Run()
		if err != nil {
			return err
		}

		switch action {
		case "configure":
			if err := configure(&cfg); err != nil {
				return err
			}
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, "Конфигурация сохранена в deploy/config.json")
		case "render":
			if err := writeDeployFiles(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, "Deploy-файлы обновлены в папке deploy/")
		case "install-systemd":
			if err := writeDeployFiles(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, installSystemd(cfg))
		case "scan":
			fmt.Fprintln(out, scanState(cfg))
		case "restart-if-hung":
			fmt.Fprintln(out, restartIfHung(cfg))
		case "ssl":
			if err := writeDeployFiles(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, configureSSL(cfg))
		case "quit":
			return nil
		}
	}
}

func defaultConfig() Config {
	return Config{
		SiteName:      "Docs Hub",
		Domain:        "docs.example.com",
		Email:         "admin@example.com",
		ServiceName:   "docs-hub",
		BinaryPath:    "/opt/docs-hub/docs-hub",
		InstallDir:    "/opt/docs-hub",
		DataDir:       "/var/lib/docs-hub",
		ListenAddr:    "127.0.0.1:8080",
		PublicPort:    "443",
		AdminUser:     "admin",
		AdminPassword: "change-me-before-first-run",
		UseCaddy:      true,
	}
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("read %s: %w", configFile, err)
		}
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	return cfg, err
}

func saveConfig(cfg Config) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, append(data, '\n'), 0o600)
}

func configure(cfg *Config) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Название сайта").Value(&cfg.SiteName),
			huh.NewInput().Title("Домен").Description("Например vault.example.com").Value(&cfg.Domain),
			huh.NewInput().Title("Email для Let's Encrypt").Value(&cfg.Email),
			huh.NewInput().Title("Имя systemd-сервиса").Value(&cfg.ServiceName),
		),
		huh.NewGroup(
			huh.NewInput().Title("Путь к бинарю на сервере").Value(&cfg.BinaryPath),
			huh.NewInput().Title("Рабочая папка").Value(&cfg.InstallDir),
			huh.NewInput().Title("Папка данных").Value(&cfg.DataDir),
			huh.NewInput().Title("Listen address").Description("Обычно 127.0.0.1:8080 за reverse proxy").Value(&cfg.ListenAddr),
		),
		huh.NewGroup(
			huh.NewInput().Title("Публичный HTTPS порт").Value(&cfg.PublicPort),
			huh.NewInput().Title("Первичный admin user").Value(&cfg.AdminUser),
			huh.NewInput().Title("Первичный admin password").EchoMode(huh.EchoModePassword).Value(&cfg.AdminPassword),
			huh.NewConfirm().Title("Использовать Caddy для автоматического SSL?").Value(&cfg.UseCaddy),
		),
	).WithTheme(huh.ThemeCharm()).Run()
}

func writeDeployFiles(cfg Config) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	specs, err := renderFiles(cfg)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if err := os.WriteFile(spec.Path, []byte(spec.Content), spec.Mode); err != nil {
			return err
		}
	}
	return saveConfig(cfg)
}

func renderFiles(cfg Config) ([]fileSpec, error) {
	uploadsDir := filepath.ToSlash(filepath.Join(cfg.DataDir, "uploads"))
	dataFile := filepath.ToSlash(filepath.Join(cfg.DataDir, "storage.json"))
	values := map[string]string{
		"SiteName":      cfg.SiteName,
		"Domain":        cfg.Domain,
		"Email":         cfg.Email,
		"ServiceName":   cfg.ServiceName,
		"BinaryPath":    cfg.BinaryPath,
		"InstallDir":    cfg.InstallDir,
		"DataDir":       cfg.DataDir,
		"DataFile":      dataFile,
		"UploadsDir":    uploadsDir,
		"ListenAddr":    cfg.ListenAddr,
		"PublicPort":    cfg.PublicPort,
		"AdminUser":     cfg.AdminUser,
		"AdminPassword": cfg.AdminPassword,
		"LocalHealth":   "http://" + cfg.ListenAddr + "/healthz",
	}
	files := []struct {
		path string
		mode os.FileMode
		tpl  string
	}{
		{"deploy/docs-hub.env", 0o600, envTemplate},
		{"deploy/docs-hub.service", 0o644, systemdTemplate},
		{"deploy/docs-hub-watchdog.sh", 0o755, watchdogTemplate},
		{"deploy/docs-hub-watchdog.service", 0o644, watchdogServiceTemplate},
		{"deploy/docs-hub-watchdog.timer", 0o644, watchdogTimerTemplate},
		{"deploy/Caddyfile", 0o644, caddyTemplate},
		{"deploy/docker-compose.deploy.yml", 0o644, composeTemplate},
	}
	var specs []fileSpec
	for _, f := range files {
		content, err := executeTemplate(f.path, f.tpl, values)
		if err != nil {
			return nil, err
		}
		specs = append(specs, fileSpec{Path: f.path, Content: content, Mode: f.mode})
	}
	return specs, nil
}

func executeTemplate(name, text string, data map[string]string) (string, error) {
	tpl, err := template.New(name).Funcs(template.FuncMap{"envq": quoteEnv}).Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func quoteEnv(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func scanState(cfg Config) string {
	var lines []string
	lines = append(lines, "Health scan:")
	if ok, detail := checkHTTP("http://" + cfg.ListenAddr + "/healthz"); ok {
		lines = append(lines, "  HTTP /healthz: ok - "+detail)
	} else {
		lines = append(lines, "  HTTP /healthz: fail - "+detail)
	}
	if out, err := runCommand("systemctl", "is-active", cfg.ServiceName); err == nil {
		lines = append(lines, "  systemd: "+strings.TrimSpace(out))
	} else {
		lines = append(lines, "  systemd: unavailable - "+trimErr(out, err))
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "storage.json")); err == nil {
		lines = append(lines, "  storage: ok")
	} else {
		lines = append(lines, "  storage: "+err.Error())
	}
	return strings.Join(lines, "\n")
}

func restartIfHung(cfg Config) string {
	if ok, detail := checkHTTP("http://" + cfg.ListenAddr + "/healthz"); ok {
		return "Health check проходит, перезапуск не нужен: " + detail
	}
	if runtime.GOOS == "windows" {
		return "Health check не проходит, но systemd недоступен на Windows. На Linux выполните: sudo systemctl restart " + cfg.ServiceName
	}
	out, err := runCommand("systemctl", "restart", cfg.ServiceName)
	if err != nil {
		return "Не удалось перезапустить service: " + trimErr(out, err)
	}
	return "Сервис перезапущен через systemd."
}

func installSystemd(cfg Config) string {
	if runtime.GOOS == "windows" {
		return strings.Join([]string{
			"Файлы сгенерированы. На Linux-сервере выполните:",
			"sudo install -d -o docshub -g docshub " + cfg.InstallDir + " " + cfg.DataDir,
			"sudo install -m 755 ./docs-hub " + cfg.BinaryPath,
			"sudo install -m 600 deploy/docs-hub.env /etc/docs-hub.env",
			"sudo install -m 644 deploy/docs-hub.service /etc/systemd/system/" + cfg.ServiceName + ".service",
			"sudo install -m 755 deploy/docs-hub-watchdog.sh /usr/local/bin/docs-hub-watchdog.sh",
			"sudo install -m 644 deploy/docs-hub-watchdog.service /etc/systemd/system/docs-hub-watchdog.service",
			"sudo install -m 644 deploy/docs-hub-watchdog.timer /etc/systemd/system/docs-hub-watchdog.timer",
			"sudo systemctl daemon-reload",
			"sudo systemctl enable --now " + cfg.ServiceName + " docs-hub-watchdog.timer",
		}, "\n")
	}
	commands := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", cfg.ServiceName},
		{"systemctl", "enable", "--now", "docs-hub-watchdog.timer"},
	}
	var lines []string
	for _, cmd := range commands {
		out, err := runCommand(cmd[0], cmd[1:]...)
		if err != nil {
			lines = append(lines, strings.Join(cmd, " ")+": "+trimErr(out, err))
		} else {
			lines = append(lines, strings.Join(cmd, " ")+": ok")
		}
	}
	return strings.Join(lines, "\n")
}

func configureSSL(cfg Config) string {
	if !cfg.UseCaddy {
		return "Caddy выключен в конфигурации. Включите его, чтобы получать SSL автоматически."
	}
	lines := []string{
		"Caddyfile создан: deploy/Caddyfile",
		"Caddy сам получает и продлевает Let's Encrypt сертификаты для " + cfg.Domain + ".",
	}
	if runtime.GOOS == "windows" {
		lines = append(lines,
			"На Linux-сервере выполните:",
			"sudo install -m 644 deploy/Caddyfile /etc/caddy/Caddyfile",
			"sudo caddy validate --config /etc/caddy/Caddyfile",
			"sudo systemctl reload caddy",
		)
		return strings.Join(lines, "\n")
	}
	if out, err := runCommand("caddy", "validate", "--config", "deploy/Caddyfile"); err != nil {
		lines = append(lines, "caddy validate: "+trimErr(out, err))
	} else {
		lines = append(lines, "caddy validate: ok")
	}
	if out, err := runCommand("systemctl", "reload", "caddy"); err != nil {
		lines = append(lines, "systemctl reload caddy: "+trimErr(out, err))
	} else {
		lines = append(lines, "systemctl reload caddy: ok")
	}
	return strings.Join(lines, "\n")
}

func checkHTTP(url string) (bool, string) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, resp.Status
	}
	return true, resp.Status
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func trimErr(out string, err error) string {
	text := strings.TrimSpace(out)
	if text == "" && err != nil {
		text = err.Error()
	}
	return text
}

const envTemplate = `ADDR={{ envq .ListenAddr }}
DATA_FILE={{ envq .DataFile }}
UPLOAD_DIR={{ envq .UploadsDir }}
ADMIN_USER={{ envq .AdminUser }}
ADMIN_PASSWORD={{ envq .AdminPassword }}
DOCS_HUB_SITE_NAME={{ envq .SiteName }}
DOCS_HUB_MODE=server
`

const systemdTemplate = `[Unit]
Description={{ .SiteName }}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=docshub
Group=docshub
WorkingDirectory={{ .InstallDir }}
EnvironmentFile=/etc/docs-hub.env
ExecStart={{ .BinaryPath }} serve
Restart=on-failure
RestartSec=5
TimeoutStopSec=20
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths={{ .DataDir }}

[Install]
WantedBy=multi-user.target
`

const watchdogTemplate = `#!/usr/bin/env sh
set -eu

SERVICE="{{ .ServiceName }}"
HEALTH="{{ .LocalHealth }}"

if ! curl -fsS --max-time 5 "$HEALTH" >/dev/null; then
  systemctl restart "$SERVICE"
fi
`

const watchdogServiceTemplate = `[Unit]
Description=Docs Hub health watchdog

[Service]
Type=oneshot
ExecStart=/usr/local/bin/docs-hub-watchdog.sh
`

const watchdogTimerTemplate = `[Unit]
Description=Run Docs Hub health watchdog every minute

[Timer]
OnBootSec=2min
OnUnitActiveSec=1min
AccuracySec=10s
Unit=docs-hub-watchdog.service

[Install]
WantedBy=timers.target
`

const caddyTemplate = `{
	email {{ .Email }}
}

{{ .Domain }} {
	encode zstd gzip
	reverse_proxy {{ .ListenAddr }}
}
`

const composeTemplate = `services:
  docs-hub:
    build: ..
    container_name: {{ .ServiceName }}
    command: ["/app/docs-hub", "serve"]
    environment:
      ADDR: ":8080"
      DATA_FILE: "/data/storage.json"
      ADMIN_USER: {{ envq .AdminUser }}
      ADMIN_PASSWORD: {{ envq .AdminPassword }}
      DOCS_HUB_MODE: "server"
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - {{ .DataDir }}:/data
    restart: unless-stopped
`
