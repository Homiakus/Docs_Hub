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
	configDir       = "deploy"
	configFile      = "deploy/config.json"
	defaultUser     = "docshub"
	defaultGroup    = "docshub"
	systemEnvPath   = "/etc/docs-hub.env"
	watchdogBinPath = "/usr/local/bin/docs-hub-watchdog.sh"
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
				huh.NewOption("Автонастроить деплой на этом сервере", "auto-setup"),
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
		case "auto-setup":
			cfg = autoDetectConfig(cfg)
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, autoSetup(cfg))
		case "configure":
			cfg = autoDetectConfig(cfg)
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
		return autoDetectConfig(cfg), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return autoDetectConfig(cfg), nil
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
	*cfg = autoDetectConfig(*cfg)
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Название сайта").Value(&cfg.SiteName),
			huh.NewInput().Title("Домен").Description("Например vault.example.com").Value(&cfg.Domain),
			huh.NewInput().Title("Email для Let's Encrypt").Value(&cfg.Email),
			huh.NewInput().Title("Имя systemd-сервиса").Value(&cfg.ServiceName),
		),
		huh.NewGroup(
			huh.NewInput().Title("Путь к бинарю на сервере").Description("Определяется автоматически: текущий executable, PATH или стандартный /opt/docs-hub/docs-hub").Value(&cfg.BinaryPath),
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
	cfg = autoDetectConfig(cfg)
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

func autoDetectConfig(cfg Config) Config {
	defaults := defaultConfig()
	if strings.TrimSpace(cfg.SiteName) == "" {
		cfg.SiteName = defaults.SiteName
	}
	if strings.TrimSpace(cfg.Domain) == "" {
		cfg.Domain = defaults.Domain
	}
	if strings.TrimSpace(cfg.Email) == "" {
		cfg.Email = defaults.Email
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = defaults.ServiceName
	}
	if strings.TrimSpace(cfg.InstallDir) == "" {
		cfg.InstallDir = defaults.InstallDir
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = defaults.DataDir
	}
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = defaults.ListenAddr
	}
	if strings.TrimSpace(cfg.PublicPort) == "" {
		cfg.PublicPort = defaults.PublicPort
	}
	if strings.TrimSpace(cfg.AdminUser) == "" {
		cfg.AdminUser = defaults.AdminUser
	}
	if strings.TrimSpace(cfg.AdminPassword) == "" {
		cfg.AdminPassword = defaults.AdminPassword
	}
	if shouldAutofillBinary(cfg.BinaryPath, defaults.BinaryPath) {
		cfg.BinaryPath = detectServiceBinaryPath(cfg)
	}
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		cfg.BinaryPath = filepath.ToSlash(filepath.Join(cfg.InstallDir, cfg.ServiceName))
	}
	return cfg
}

func shouldAutofillBinary(current, fallback string) bool {
	current = strings.TrimSpace(current)
	return current == "" || current == fallback
}

func detectServiceBinaryPath(cfg Config) string {
	defaultPath := filepath.ToSlash(filepath.Join(cfg.InstallDir, cfg.ServiceName))
	if runtime.GOOS == "windows" {
		return defaultPath
	}
	for _, candidate := range binaryCandidates(cfg) {
		if candidate == "" || looksLikeGoRunTemp(candidate) {
			continue
		}
		if sameBaseName(candidate, cfg.ServiceName) && fileExists(candidate) {
			return filepath.ToSlash(candidate)
		}
	}
	return defaultPath
}

func binaryCandidates(cfg Config) []string {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, exe)
	}
	for _, name := range uniqueStrings([]string{cfg.ServiceName, "docs-hub"}) {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, cfg.ServiceName),
			filepath.Join(cwd, cfg.ServiceName+".exe"),
			filepath.Join(cwd, "docs-hub"),
			filepath.Join(cwd, "docs-hub.exe"),
			filepath.Join(cwd, "dist", "docs-hub-linux-amd64"),
		)
	}
	candidates = append(candidates,
		cfg.BinaryPath,
		filepath.Join(cfg.InstallDir, cfg.ServiceName),
		"/usr/local/bin/docs-hub",
		"/usr/bin/docs-hub",
		"/opt/docs-hub/docs-hub",
	)
	return uniqueStrings(candidates)
}

func sourceBinaryPath(cfg Config) (string, bool) {
	for _, candidate := range binaryCandidates(cfg) {
		if candidate == "" || looksLikeGoRunTemp(candidate) {
			continue
		}
		if fileExists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func looksLikeGoRunTemp(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(path, "/go-build") || strings.Contains(path, "/tmp/go-build")
}

func sameBaseName(path, serviceName string) bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe")
	service := strings.TrimSuffix(strings.ToLower(serviceName), ".exe")
	return base == service || base == "docs-hub" || strings.HasPrefix(base, "docs-hub-")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := filepath.Clean(value)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
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

func autoSetup(cfg Config) string {
	cfg = autoDetectConfig(cfg)
	if err := writeDeployFiles(cfg); err != nil {
		return "Не удалось сгенерировать deploy-файлы: " + err.Error()
	}
	lines := []string{
		"Автоконфигурация:",
		"  binary: " + cfg.BinaryPath,
		"  install dir: " + cfg.InstallDir,
		"  data dir: " + cfg.DataDir,
		"  listen: " + cfg.ListenAddr,
	}
	if runtime.GOOS == "windows" {
		lines = append(lines,
			"",
			"На Windows выполнена подготовка deploy-файлов. Автоустановка systemd/Caddy доступна на Linux-сервере.",
			installSystemd(cfg),
		)
		return strings.Join(lines, "\n")
	}
	if os.Geteuid() != 0 {
		lines = append(lines,
			"",
			"Для автоустановки нужны root-права. Запустите на сервере:",
		)
		lines = append(lines, sudoInstallCommands(cfg)...)
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "")
	lines = append(lines, installOnLinuxAsRoot(cfg)...)
	return strings.Join(lines, "\n")
}

func installSystemd(cfg Config) string {
	cfg = autoDetectConfig(cfg)
	if runtime.GOOS == "windows" {
		return strings.Join([]string{
			"Файлы сгенерированы. На Linux-сервере выполните:",
			strings.Join(sudoInstallCommands(cfg), "\n"),
		}, "\n")
	}
	if os.Geteuid() != 0 {
		return strings.Join(append([]string{
			"Файлы сгенерированы. Для установки systemd нужны root-права:",
		}, sudoInstallCommands(cfg)...), "\n")
	}
	return strings.Join(installOnLinuxAsRoot(cfg), "\n")
}

func installOnLinuxAsRoot(cfg Config) []string {
	specs, err := renderFiles(cfg)
	if err != nil {
		return []string{"render deploy files: " + err.Error()}
	}
	content := map[string]fileSpec{}
	for _, spec := range specs {
		content[spec.Path] = spec
	}
	var lines []string
	runStep := func(label string, name string, args ...string) {
		out, err := runCommand(name, args...)
		if err != nil {
			lines = append(lines, label+": "+trimErr(out, err))
			return
		}
		lines = append(lines, label+": ok")
	}

	if _, err := runCommand("getent", "group", defaultGroup); err != nil {
		runStep("groupadd "+defaultGroup, "groupadd", "--system", defaultGroup)
	} else {
		lines = append(lines, "group "+defaultGroup+": exists")
	}
	if _, err := runCommand("id", "-u", defaultUser); err != nil {
		runStep("useradd "+defaultUser, "useradd", "--system", "--no-create-home", "--home-dir", cfg.InstallDir, "--shell", "/usr/sbin/nologin", "--gid", defaultGroup, defaultUser)
	} else {
		lines = append(lines, "user "+defaultUser+": exists")
	}

	if err := os.MkdirAll(cfg.InstallDir, 0o755); err != nil {
		lines = append(lines, "create "+cfg.InstallDir+": "+err.Error())
	} else {
		lines = append(lines, "create "+cfg.InstallDir+": ok")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		lines = append(lines, "create "+cfg.DataDir+": "+err.Error())
	} else {
		lines = append(lines, "create "+cfg.DataDir+": ok")
	}
	runStep("chown install/data dirs", "chown", "-R", defaultUser+":"+defaultGroup, cfg.InstallDir, cfg.DataDir)

	source, ok := sourceBinaryPath(cfg)
	if !ok {
		lines = append(lines, "install binary: не найден локальный бинарь. Соберите его: go build -o docs-hub .")
	} else if sameFile(source, cfg.BinaryPath) {
		if err := os.Chmod(cfg.BinaryPath, 0o755); err != nil {
			lines = append(lines, "chmod binary: "+err.Error())
		} else {
			lines = append(lines, "binary already in place: "+cfg.BinaryPath)
		}
	} else if err := copyFile(source, cfg.BinaryPath, 0o755); err != nil {
		lines = append(lines, "install binary: "+err.Error())
	} else {
		lines = append(lines, "install binary: "+source+" -> "+cfg.BinaryPath)
	}

	installSpecs := []fileSpec{
		{Path: systemEnvPath, Content: content["deploy/docs-hub.env"].Content, Mode: 0o600},
		{Path: "/etc/systemd/system/" + cfg.ServiceName + ".service", Content: content["deploy/docs-hub.service"].Content, Mode: 0o644},
		{Path: watchdogBinPath, Content: content["deploy/docs-hub-watchdog.sh"].Content, Mode: 0o755},
		{Path: "/etc/systemd/system/docs-hub-watchdog.service", Content: content["deploy/docs-hub-watchdog.service"].Content, Mode: 0o644},
		{Path: "/etc/systemd/system/docs-hub-watchdog.timer", Content: content["deploy/docs-hub-watchdog.timer"].Content, Mode: 0o644},
	}
	for _, spec := range installSpecs {
		if err := writeSystemFile(spec.Path, spec.Content, spec.Mode); err != nil {
			lines = append(lines, "install "+spec.Path+": "+err.Error())
		} else {
			lines = append(lines, "install "+spec.Path+": ok")
		}
	}

	commands := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", cfg.ServiceName},
		{"systemctl", "enable", "--now", "docs-hub-watchdog.timer"},
	}
	for _, cmd := range commands {
		out, err := runCommand(cmd[0], cmd[1:]...)
		if err != nil {
			lines = append(lines, strings.Join(cmd, " ")+": "+trimErr(out, err))
		} else {
			lines = append(lines, strings.Join(cmd, " ")+": ok")
		}
	}
	if cfg.UseCaddy {
		lines = append(lines, installCaddyAsRoot(cfg, content["deploy/Caddyfile"].Content)...)
	}
	return lines
}

func sudoInstallCommands(cfg Config) []string {
	source, ok := sourceBinaryPath(cfg)
	if !ok {
		source = "./docs-hub"
	}
	commands := []string{
		"sudo groupadd --system " + shellQuote(defaultGroup) + " || true",
		"sudo id -u " + shellQuote(defaultUser) + " >/dev/null 2>&1 || sudo useradd --system --no-create-home --home-dir " + shellQuote(cfg.InstallDir) + " --shell /usr/sbin/nologin --gid " + shellQuote(defaultGroup) + " " + shellQuote(defaultUser),
		"sudo install -d -m 755 -o " + shellQuote(defaultUser) + " -g " + shellQuote(defaultGroup) + " " + shellQuote(cfg.InstallDir),
		"sudo install -d -m 750 -o " + shellQuote(defaultUser) + " -g " + shellQuote(defaultGroup) + " " + shellQuote(cfg.DataDir),
		"sudo install -m 755 " + shellQuote(source) + " " + shellQuote(cfg.BinaryPath),
		"sudo install -m 600 deploy/docs-hub.env " + shellQuote(systemEnvPath),
		"sudo install -m 644 deploy/docs-hub.service " + shellQuote("/etc/systemd/system/"+cfg.ServiceName+".service"),
		"sudo install -m 755 deploy/docs-hub-watchdog.sh " + shellQuote(watchdogBinPath),
		"sudo install -m 644 deploy/docs-hub-watchdog.service /etc/systemd/system/docs-hub-watchdog.service",
		"sudo install -m 644 deploy/docs-hub-watchdog.timer /etc/systemd/system/docs-hub-watchdog.timer",
		"sudo systemctl daemon-reload",
		"sudo systemctl enable --now " + shellQuote(cfg.ServiceName) + " docs-hub-watchdog.timer",
	}
	if cfg.UseCaddy {
		commands = append(commands,
			"sudo install -m 644 deploy/Caddyfile /etc/caddy/Caddyfile",
			"sudo caddy validate --config /etc/caddy/Caddyfile",
			"sudo systemctl reload caddy",
		)
	}
	return commands
}

func installCaddyAsRoot(cfg Config, caddyfile string) []string {
	if _, err := exec.LookPath("caddy"); err != nil {
		return []string{"caddy: не найден в PATH, Caddyfile оставлен в deploy/Caddyfile"}
	}
	if _, err := os.Stat("/etc/caddy"); err != nil {
		return []string{"caddy: /etc/caddy не найден, Caddyfile оставлен в deploy/Caddyfile"}
	}
	path := "/etc/caddy/Caddyfile"
	var lines []string
	if fileExists(path) {
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := copyFile(path, backup, 0o644); err != nil {
			lines = append(lines, "backup existing Caddyfile: "+err.Error())
		} else {
			lines = append(lines, "backup existing Caddyfile: "+backup)
		}
	}
	if err := writeSystemFile(path, caddyfile, 0o644); err != nil {
		lines = append(lines, "install Caddyfile: "+err.Error())
		return lines
	}
	lines = append(lines, "install Caddyfile: ok")
	if out, err := runCommand("caddy", "validate", "--config", path); err != nil {
		lines = append(lines, "caddy validate: "+trimErr(out, err))
	} else {
		lines = append(lines, "caddy validate: ok")
	}
	if out, err := runCommand("systemctl", "reload", "caddy"); err != nil {
		lines = append(lines, "systemctl reload caddy: "+trimErr(out, err))
	} else {
		lines = append(lines, "systemctl reload caddy: ok")
	}
	return lines
}

func writeSystemFile(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), mode)
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func sameFile(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr != nil || berr != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return os.SameFile(ai, bi)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '=' || r == '+' || r == '@' ||
			(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func configureSSL(cfg Config) string {
	if !cfg.UseCaddy {
		return "Caddy выключен в конфигурации. Включите его, чтобы получать SSL автоматически."
	}
	cfg = autoDetectConfig(cfg)
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
	if os.Geteuid() == 0 {
		caddyfile, err := renderCaddy(cfg)
		if err != nil {
			lines = append(lines, "render Caddyfile: "+err.Error())
			return strings.Join(lines, "\n")
		}
		return strings.Join(append(lines, installCaddyAsRoot(cfg, caddyfile)...), "\n")
	}
	lines = append(lines,
		"Для установки Caddyfile нужны root-права:",
		"sudo install -m 644 deploy/Caddyfile /etc/caddy/Caddyfile",
		"sudo caddy validate --config /etc/caddy/Caddyfile",
		"sudo systemctl reload caddy",
	)
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

func renderCaddy(cfg Config) (string, error) {
	files, err := renderFiles(cfg)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.Path == "deploy/Caddyfile" {
			return file.Content, nil
		}
	}
	return "", errors.New("Caddyfile template not found")
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
