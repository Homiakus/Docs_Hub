package deploytui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
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
				huh.NewOption("🚀 Запустить демо (тестовый режим)", "demo-run"),
				huh.NewOption("▶️  Быстрый запуск сервера", "run-server"),
				huh.NewOption("🌐 Открыть сайт в браузере", "open-browser"),
				huh.NewOption("📊 Мониторинг сайта (live)", "monitor"),
				huh.NewOption("⚡ Полная автоустановка (всё сразу)", "full-auto-setup"),
				huh.NewOption("🔧 Автонастроить деплой на этом сервере", "auto-setup"),
				huh.NewOption("⚙️  Настроить конфигурацию", "configure"),
				huh.NewOption("📁 Сгенерировать deploy-файлы", "render"),
				huh.NewOption("📦 Установить systemd сервис", "install-systemd"),
				huh.NewOption("🔍 Проверить состояние сервиса", "scan"),
				huh.NewOption("🔄 Перезапустить если завис", "restart-if-hung"),
				huh.NewOption("🔒 Настроить Caddy SSL", "ssl"),
				huh.NewOption("🚪 Выход", "quit"),
			).
			Value(&action).
			Run()
		if err != nil {
			return err
		}

		switch action {
		case "demo-run":
			fmt.Fprintln(out, runDemoServer())
		case "run-server":
			fmt.Fprintln(out, runForegroundServer(cfg, out))
		case "open-browser":
			addr := cfg.ListenAddr
			if cfg.UseCaddy {
				addr = "https://" + cfg.Domain
			} else {
				addr = "http://" + cfg.ListenAddr
			}
			fmt.Fprintf(out, "Открываю %s в браузере...\n", addr)
			if err := openBrowser(addr); err != nil {
				fmt.Fprintln(out, "Ошибка открытия браузера:", err)
			}
		case "monitor":
			monitorLive(cfg, out)
		case "full-auto-setup":
			var confirmed bool
			if err := huh.NewConfirm().
				Title("Это установит Docs Hub как systemd-сервис, настроит Caddy SSL и watchdog. Продолжить?").
				Value(&confirmed).
				Run(); err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(out, "Отменено.")
				continue
			}
			cfg = autoDetectConfig(cfg)
			if err := saveConfig(cfg); err != nil {
				return err
			}

			if runtime.GOOS == "linux" && os.Geteuid() != 0 {
				fmt.Fprintln(out, "Для полной автоустановки нужны root-права. Запустите на сервере:")
				for _, cmd := range sudoInstallCommands(cfg) {
					fmt.Fprintln(out, cmd)
				}
				continue
			}

			if err := writeDeployFiles(cfg); err != nil {
				fmt.Fprintln(out, "Не удалось сгенерировать deploy-файлы:", err)
				continue
			}

			fmt.Fprintln(out, installSystemd(cfg))

			if cfg.UseCaddy {
				fmt.Fprintln(out, configureSSL(cfg))
			}

			scanResult := scanState(cfg)
			fmt.Fprintln(out, scanResult)

			url := "http://" + cfg.ListenAddr + "/healthz"
			if data, _, err2 := pollHealth(url); err2 == nil && data != nil {
				fmt.Fprintln(out, "✅ Установка завершена! Сайт доступен.")
			} else {
				fmt.Fprintln(out, "⚠️ Сервис установлен, но health check не проходит. Проверьте: systemctl status "+cfg.ServiceName)
			}
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
		AdminPassword: "admin123",
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

// scanState performs a health check against the service and returns diagnostic info.
// Enhanced with /healthz JSON parsing, response time, file size, and backup count.
func scanState(cfg Config) string {
	var lines []string
	lines = append(lines, "Health scan:")

	url := "http://" + cfg.ListenAddr + "/healthz"
	data, duration, err := pollHealth(url)
	lines = append(lines, fmt.Sprintf("  Response time: %s", formatDuration(duration)))

	if err != nil {
		lines = append(lines, "  HTTP /healthz: fail - "+err.Error())
	} else {
		lines = append(lines, "  HTTP /healthz: ok (200)")
		if data != nil {
			if v, ok := data["version"]; ok {
				lines = append(lines, fmt.Sprintf("  Version: %v", v))
			}
			if v, ok := data["users"]; ok {
				lines = append(lines, fmt.Sprintf("  Users: %v", v))
			}
			if v, ok := data["articles"]; ok {
				lines = append(lines, fmt.Sprintf("  Articles: %v", v))
			}
		}
	}

	if out, err2 := runCommand("systemctl", "is-active", cfg.ServiceName); err2 == nil {
		lines = append(lines, "  systemd: "+strings.TrimSpace(out))
	} else {
		lines = append(lines, "  systemd: unavailable - "+trimErr(out, err2))
	}

	storagePath := filepath.Join(cfg.DataDir, "storage.json")
	if info, err2 := os.Stat(storagePath); err2 == nil {
		lines = append(lines, fmt.Sprintf("  storage: ok (size: %s)", formatSize(info.Size())))
	} else {
		lines = append(lines, "  storage: "+err2.Error())
	}

	backupDir := filepath.Join(cfg.DataDir, "backups")
	if entries, err2 := os.ReadDir(backupDir); err2 == nil {
		count := 0
		for _, e := range entries {
			if !e.IsDir() {
				count++
			}
		}
		lines = append(lines, fmt.Sprintf("  backups: %d", count))
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

// --------------------------------------------------------------------------
// New helper functions added for enhanced TUI features
// --------------------------------------------------------------------------

// openBrowser opens the given URL in the system default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// pollHealth hits the /healthz JSON endpoint and returns parsed data, response
// duration, and any error.
func pollHealth(url string) (map[string]any, time.Duration, error) {
	start := time.Now()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, time.Since(start), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, time.Since(start), fmt.Errorf("HTTP %s", resp.Status)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		// Body might not be valid JSON; return nil data without error
		return nil, time.Since(start), nil
	}
	return data, time.Since(start), nil
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return "0s"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// formatSize returns a human-readable file size string.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// runDemoServer creates a temporary data directory, starts the HTTP server in
// demo mode, opens the browser, and blocks until Ctrl+C.
func runDemoServer() string {
	tmpDir, err := os.MkdirTemp("", "docs-hub-demo-*")
	if err != nil {
		return "Ошибка создания временной папки: " + err.Error()
	}

	dataFile := filepath.Join(tmpDir, "storage.json")

	exe, err := os.Executable()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "Не удалось найти исполняемый файл: " + err.Error()
	}

	cmd := exec.Command(exe, "serve")
	cmd.Env = append(os.Environ(),
		"DOCS_HUB_MODE=server",
		"DATA_FILE="+dataFile,
		"ADDR=:8080",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return "Не удалось запустить демо-сервер: " + err.Error()
	}

	time.Sleep(500 * time.Millisecond)
	openBrowser("http://localhost:8080")

	fmt.Println("Демо-сервер запущен на http://localhost:8080. Нажмите Ctrl+C для остановки.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	signal.Stop(sigCh)

	cmd.Process.Kill()
	cmd.Wait()
	os.RemoveAll(tmpDir)

	return "Демо остановлен."
}

// runForegroundServer runs the server in the foreground using the real
// configuration, opens the browser, and blocks until Ctrl+C.
func runForegroundServer(cfg Config, out io.Writer) string {
	cfg = autoDetectConfig(cfg)
	dataFile := filepath.Join(cfg.DataDir, "storage.json")
	uploadsDir := filepath.Join(cfg.DataDir, "uploads")

	exe, err := os.Executable()
	if err != nil {
		return "Не удалось найти исполняемый файл: " + err.Error()
	}

	cmd := exec.Command(exe, "serve")
	cmd.Env = append(os.Environ(),
		"DOCS_HUB_MODE=server",
		"DATA_FILE="+dataFile,
		"ADDR="+cfg.ListenAddr,
		"UPLOAD_DIR="+uploadsDir,
		"ADMIN_USER="+cfg.AdminUser,
		"ADMIN_PASSWORD="+cfg.AdminPassword,
		"DOCS_HUB_SITE_NAME="+cfg.SiteName,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "Не удалось запустить сервер: " + err.Error()
	}

	time.Sleep(500 * time.Millisecond)

	url := "http://" + cfg.ListenAddr
	openBrowser(url)

	fmt.Fprintln(out, "Сервер запущен на "+url+". Нажмите Ctrl+C для остановки.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh

	signal.Stop(sigCh)

	cmd.Process.Kill()
	cmd.Wait()

	return "Сервер остановлен."
}

// clearScreen clears the terminal and hides the cursor.
func clearScreen(out io.Writer) {
	fmt.Fprint(out, "\033[H\033[2J")
	fmt.Fprint(out, "\033[?25l")
}

// renderMonitor draws the live monitoring dashboard.
func renderMonitor(out io.Writer, start time.Time, status string, data map[string]any, duration time.Duration, lastError string, cfg Config) {
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))   // green
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)

	var statusStr string
	switch status {
	case "OK":
		statusStr = okStyle.Render("● OK")
	case "FAIL":
		statusStr = failStyle.Render("● FAIL")
	default:
		statusStr = dimStyle.Render("● …")
	}

	var lines []string
	lines = append(lines, headerStyle.Render("📊 Docs Hub Monitor"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Status:    %s", statusStr))
	lines = append(lines, fmt.Sprintf("  URL:       http://%s/healthz", cfg.ListenAddr))
	lines = append(lines, fmt.Sprintf("  Response:  %s", formatDuration(duration)))
	lines = append(lines, fmt.Sprintf("  Uptime:    %s", formatDuration(time.Since(start))))

	if data != nil {
		if v, ok := data["version"]; ok {
			lines = append(lines, fmt.Sprintf("  Version:   %v", v))
		}
		if v, ok := data["users"]; ok {
			lines = append(lines, fmt.Sprintf("  Users:     %v", v))
		}
		if v, ok := data["articles"]; ok {
			lines = append(lines, fmt.Sprintf("  Articles:  %v", v))
		}
	}

	if status == "FAIL" {
		lines = append(lines, "")
		lines = append(lines, failStyle.Render("⚠️  Сервер не отвечает!"))
		if lastError != "" {
			lines = append(lines, dimStyle.Render("  Last error: "+lastError))
		}
	}

	// Systemd section
	lines = append(lines, "")
	lines = append(lines, headerStyle.Render("Systemd:"))
	if runtime.GOOS == "windows" {
		lines = append(lines, dimStyle.Render("  systemd: недоступен на Windows"))
	} else {
		out2, err := runCommand("systemctl", "status", cfg.ServiceName, "--no-pager", "-l")
		if err != nil {
			lines = append(lines, dimStyle.Render("  "+trimErr(out2, err)))
		} else {
			systemLines := strings.Split(strings.TrimSpace(out2), "\n")
			// Show last 5 lines
			if len(systemLines) > 5 {
				systemLines = systemLines[len(systemLines)-5:]
			}
			for _, l := range systemLines {
				lines = append(lines, dimStyle.Render("  "+strings.TrimSpace(l)))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Нажмите 'q' + Enter для выхода"))

	fmt.Fprintln(out, box.Render(strings.Join(lines, "\n")))
}

// monitorLive opens a live-updating dashboard that polls /healthz every 3
// seconds. Press 'q' + Enter to exit.
func monitorLive(cfg Config, out io.Writer) {
	url := "http://" + cfg.ListenAddr + "/healthz"

	keyCh := make(chan byte, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "q" {
				keyCh <- 'q'
				return
			}
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	start := time.Now()
	var lastError string
	var lastData map[string]any
	var lastDuration time.Duration
	var lastStatus string

	// Initial poll
	{
		data, d, err := pollHealth(url)
		lastDuration = d
		if err != nil {
			lastStatus = "FAIL"
			lastError = err.Error()
		} else {
			lastStatus = "OK"
			lastError = ""
			lastData = data
		}
		clearScreen(out)
		renderMonitor(out, start, lastStatus, lastData, lastDuration, lastError, cfg)
	}

	for {
		select {
		case <-ticker.C:
			data, d, err := pollHealth(url)
			lastDuration = d
			if err != nil {
				lastStatus = "FAIL"
				lastError = err.Error()
			} else {
				lastStatus = "OK"
				lastError = ""
				lastData = data
			}
			clearScreen(out)
			renderMonitor(out, start, lastStatus, lastData, lastDuration, lastError, cfg)
		case <-keyCh:
			fmt.Fprint(out, "\033[?25h") // show cursor
			return
		}
	}
}

// --------------------------------------------------------------------------
// Templates
// --------------------------------------------------------------------------

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
