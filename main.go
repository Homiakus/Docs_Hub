package main

import (
	"archive/zip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"docs-hub/deploytui"
)

const (
	AppName            = "Docs Hub"
	SessionCookie      = "docs_hub_session"
	SessionTTL         = 7 * 24 * time.Hour
	MaxLoginFails      = 8
	CurrentDBVersion   = 4
	MaxUploadBytes     = 25 << 20
	MaxImportBytes     = 100 << 20
	MaxImportFileBytes = 5 << 20
)

var (
	PBKDF2Rounds    = 600000
	LoginFailWindow = 10 * time.Minute
)

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	SaltHex      string    `json:"salt_hex"`
	PasswordHash string    `json:"password_hash"`
	Role         UserRole  `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MemberIDs []string  `json:"member_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Version struct {
	At      time.Time `json:"at"`
	ActorID string    `json:"actor_id"`
	Title   string    `json:"title"`
	Slug    string    `json:"slug"`
	Content string    `json:"content"`
}

type Article struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Content         string    `json:"content"`
	Tags            []string  `json:"tags"`
	AllUsers        bool      `json:"all_users"`
	AllowedUserIDs  []string  `json:"allowed_user_ids"`
	AllowedGroupIDs []string  `json:"allowed_group_ids"`
	OwnerID         string    `json:"owner_id"`
	Archived        bool      `json:"archived"`
	Versions        []Version `json:"versions"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"token_hash"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type AuditEvent struct {
	At        time.Time `json:"at"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	RemoteIP  string    `json:"remote_ip"`
	UserAgent string    `json:"user_agent"`
}

type MigrationRecord struct {
	At   time.Time `json:"at"`
	From int       `json:"from"`
	To   int       `json:"to"`
	Name string    `json:"name"`
}

type Attachment struct {
	ID           string    `json:"id"`
	ArticleID    string    `json:"article_id"`
	StoredName   string    `json:"stored_name"`
	OriginalName string    `json:"original_name"`
	MIME         string    `json:"mime"`
	Size         int64     `json:"size"`
	UploadedBy   string    `json:"uploaded_by"`
	CreatedAt    time.Time `json:"created_at"`
}

type Database struct {
	Version          int                    `json:"version"`
	SecretHex        string                 `json:"secret_hex"`
	NextUserID       int                    `json:"next_user_id"`
	NextGroupID      int                    `json:"next_group_id"`
	NextArticleID    int                    `json:"next_article_id"`
	NextSessionID    int                    `json:"next_session_id"`
	NextAttachmentID int                    `json:"next_attachment_id"`
	Users            map[string]*User       `json:"users"`
	Groups           map[string]*Group      `json:"groups"`
	Articles         map[string]*Article    `json:"articles"`
	Attachments      map[string]*Attachment `json:"attachments"`
	Sessions         map[string]*Session    `json:"sessions"`
	Audit            []AuditEvent           `json:"audit"`
	Migrations       []MigrationRecord      `json:"migrations"`
	RibbonArticleIDs []string               `json:"ribbon_article_ids"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	db   Database
}

type LoginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

type App struct {
	store   *Store
	limiter *LoginLimiter
	now     func() time.Time
}

type RequestContext struct {
	User    *User
	Session *Session
}

type LayoutData struct {
	Title  string
	App    string
	User   *User
	CSRF   string
	Body   template.HTML
	Ribbon []RibbonArticle
}

type RibbonArticle struct {
	Title  string
	Slug   string
	Active bool
}

func main() {
	if shouldOpenDeployMenu() {
		if err := deploytui.Run(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	runServer()
}

func shouldOpenDeployMenu() bool {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve", "server", "--serve":
			return false
		case "deploy", "menu", "tui", "--tui":
			return true
		case "-h", "--help", "help":
			fmt.Println("Usage: docs-hub [serve|deploy]")
			fmt.Println("  serve   run the HTTP service")
			fmt.Println("  deploy  open the deployment TUI")
			os.Exit(0)
		}
	}
	if strings.EqualFold(os.Getenv("DOCS_HUB_MODE"), "server") || strings.EqualFold(os.Getenv("MINIVAULT_MODE"), "server") {
		return false
	}
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runServer() {
	store, err := LoadStore(getenv("DATA_FILE", "storage.json"))
	if err != nil {
		log.Fatal(err)
	}
	app := NewApp(store)
	addr := getenv("ADDR", ":8080")
	log.Printf("%s started at http://localhost%s", AppName, addr)
	log.Printf("data file: %s", store.path)
	log.Fatal(http.ListenAndServe(addr, app.routes()))
}

func NewApp(store *Store) *App {
	return &App{store: store, limiter: &LoginLimiter{fails: map[string][]time.Time{}}, now: time.Now}
}

func (app *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", app.login)
	mux.HandleFunc("/logout", app.requireAuth(app.logout))
	mux.HandleFunc("/healthz", app.healthz)
	mux.HandleFunc("/", app.requireAuth(app.home))
	mux.HandleFunc("/a/", app.requireAuth(app.articleView))
	mux.HandleFunc("/edit/new", app.requireAdmin(app.articleEdit))
	mux.HandleFunc("/edit/", app.requireAdmin(app.articleEdit))
	mux.HandleFunc("/save", app.requireAdmin(app.articleSave))
	mux.HandleFunc("/archive/", app.requireAdmin(app.articleArchive))
	mux.HandleFunc("/preview/article/", app.requireAuth(app.articlePreview))
	mux.HandleFunc("/admin/users", app.requireAdmin(app.adminUsers))
	mux.HandleFunc("/admin/users/create", app.requireAdmin(app.adminUserCreate))
	mux.HandleFunc("/admin/users/password", app.requireAdmin(app.adminUserPassword))
	mux.HandleFunc("/admin/users/toggle", app.requireAdmin(app.adminUserToggle))
	mux.HandleFunc("/admin/groups", app.requireAdmin(app.adminGroups))
	mux.HandleFunc("/admin/groups/save", app.requireAdmin(app.adminGroupSave))
	mux.HandleFunc("/admin/groups/delete", app.requireAdmin(app.adminGroupDelete))
	mux.HandleFunc("/admin/ribbon", app.requireAdmin(app.adminRibbon))
	mux.HandleFunc("/admin/ribbon/save", app.requireAdmin(app.adminRibbonSave))
	mux.HandleFunc("/admin/draft", app.requireAdmin(app.adminDraft))
	mux.HandleFunc("/admin/import", app.requireAdmin(app.adminImport))
	mux.HandleFunc("/admin/preview", app.requireAdmin(app.adminPreview))
	mux.HandleFunc("/admin/audit", app.requireAdmin(app.adminAudit))
	mux.HandleFunc("/attachments/upload", app.requireAdmin(app.attachmentUpload))
	mux.HandleFunc("/attachments/drop", app.requireAdmin(app.attachmentDrop))
	mux.HandleFunc("/attachments/delete", app.requireAdmin(app.attachmentDelete))
	mux.HandleFunc("/files/", app.requireAuth(app.attachmentServe))
	mux.HandleFunc("/admin/backups", app.requireAdmin(app.adminBackups))
	mux.HandleFunc("/admin/backups/create", app.requireAdmin(app.adminBackupCreate))
	mux.HandleFunc("/admin/backups/download/", app.requireAdmin(app.adminBackupDownload))
	return securityHeaders(mux)
}

func (app *App) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	app.store.mu.RLock()
	version := app.store.db.Version
	users := len(app.store.db.Users)
	articles := len(app.store.db.Articles)
	app.store.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"app":      AppName,
		"version":  version,
		"users":    users,
		"articles": articles,
		"time":     app.now().UTC().Format(time.RFC3339),
	})
}

func getenv(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func LoadStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &s.db); err != nil {
			return nil, err
		}
		if s.normalize() {
			s.mu.Lock()
			err := s.saveLocked()
			s.mu.Unlock()
			if err != nil {
				return nil, err
			}
		}
		return s, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	adminUser := getenv("ADMIN_USER", "admin")
	adminPass := getenv("ADMIN_PASSWORD", "admin123")
	salt, hash, err := HashPassword(adminPass)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	s.db = Database{Version: CurrentDBVersion, SecretHex: hex.EncodeToString(secret), NextUserID: 2, NextGroupID: 2, NextArticleID: 2, NextSessionID: 1, NextAttachmentID: 1, Users: map[string]*User{}, Groups: map[string]*Group{}, Articles: map[string]*Article{}, Attachments: map[string]*Attachment{}, Sessions: map[string]*Session{}, Audit: []AuditEvent{}, Migrations: []MigrationRecord{{At: now, From: 0, To: CurrentDBVersion, Name: "bootstrap"}}, RibbonArticleIDs: []string{"1"}}
	s.db.Users["1"] = &User{ID: "1", Username: adminUser, SaltHex: salt, PasswordHash: hash, Role: RoleAdmin, Active: true, CreatedAt: now, UpdatedAt: now}
	s.db.Groups["1"] = &Group{ID: "1", Name: "Команда", MemberIDs: []string{}, CreatedAt: now, UpdatedAt: now}
	s.db.Articles["1"] = &Article{ID: "1", Title: "Стартовая статья", Slug: "start", Content: "# Стартовая статья\n\nЭто первая статья Docs Hub.\n\n- Админ создаёт пользователей и группы.\n- Доступ можно давать всем, пользователям или группам.\n- Поддерживаются [[start|wiki-ссылки]], #теги и backlinks.\n\n> Редактор похож на Obsidian: слева Markdown, справа live preview.", Tags: []string{"start"}, AllUsers: true, OwnerID: "1", CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	log.Printf("created initial admin: username=%q", adminUser)
	log.Printf("IMPORTANT: change the default password immediately")
	return s, nil
}

func (s *Store) normalize() bool {
	dirty := false
	if s.db.Users == nil {
		s.db.Users = map[string]*User{}
		dirty = true
	}
	if s.db.Groups == nil {
		s.db.Groups = map[string]*Group{}
		dirty = true
	}
	if s.db.Articles == nil {
		s.db.Articles = map[string]*Article{}
		dirty = true
	}
	if s.db.Attachments == nil {
		s.db.Attachments = map[string]*Attachment{}
		dirty = true
	}
	if s.db.Sessions == nil {
		s.db.Sessions = map[string]*Session{}
		dirty = true
	}
	if s.db.Audit == nil {
		s.db.Audit = []AuditEvent{}
		dirty = true
	}
	if s.db.Migrations == nil {
		s.db.Migrations = []MigrationRecord{}
		dirty = true
	}
	if s.db.NextUserID == 0 {
		s.db.NextUserID = 1
		dirty = true
	}
	if s.db.NextGroupID == 0 {
		s.db.NextGroupID = 1
		dirty = true
	}
	if s.db.NextArticleID == 0 {
		s.db.NextArticleID = 1
		dirty = true
	}
	if s.db.NextSessionID == 0 {
		s.db.NextSessionID = 1
		dirty = true
	}
	if s.db.NextAttachmentID == 0 {
		s.db.NextAttachmentID = 1
		dirty = true
	}
	ribbon := uniqueStrings(s.db.RibbonArticleIDs)
	if !equalStrings(s.db.RibbonArticleIDs, ribbon) {
		s.db.RibbonArticleIDs = ribbon
		dirty = true
	}
	if s.db.SecretHex == "" {
		secret := make([]byte, 32)
		_, _ = rand.Read(secret)
		s.db.SecretHex = hex.EncodeToString(secret)
		dirty = true
	}
	return s.migrate() || dirty
}

func (s *Store) saveLocked() error {
	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) migrate() bool {
	from := s.db.Version
	if from >= CurrentDBVersion {
		return false
	}
	now := time.Now().UTC()
	if s.db.Attachments == nil {
		s.db.Attachments = map[string]*Attachment{}
	}
	if s.db.Migrations == nil {
		s.db.Migrations = []MigrationRecord{}
	}
	if s.db.NextAttachmentID == 0 {
		s.db.NextAttachmentID = 1
	}
	stepFrom := from
	if stepFrom < 3 {
		s.db.Migrations = append(s.db.Migrations, MigrationRecord{At: now, From: stepFrom, To: 3, Name: "v3_attachments_backups_fts"})
		stepFrom = 3
	}
	if stepFrom < 4 {
		s.db.RibbonArticleIDs = uniqueStrings(s.db.RibbonArticleIDs)
		s.db.Migrations = append(s.db.Migrations, MigrationRecord{At: now, From: stepFrom, To: 4, Name: "v4_ribbon_live_editor"})
	}
	s.db.Version = CurrentDBVersion
	return true
}

func (s *Store) dataDir() string {
	dir := filepath.Dir(s.path)
	if dir == "." || dir == "" {
		return "."
	}
	return dir
}

func (s *Store) uploadsDir() string { return filepath.Join(s.dataDir(), "uploads") }
func (s *Store) backupsDir() string { return filepath.Join(s.dataDir(), "backups") }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data: blob:; media-src 'self' https: blob:; frame-src https://www.youtube-nocookie.com https://www.youtube.com; style-src 'self' 'unsafe-inline' https://uicdn.toast.com; script-src 'self' 'unsafe-inline' https://esm.sh https://uicdn.toast.com; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (app *App) requireAuth(next func(http.ResponseWriter, *http.Request, *RequestContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := app.currentContext(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, ctx)
	}
}

func (app *App) requireAdmin(next func(http.ResponseWriter, *http.Request, *RequestContext)) http.HandlerFunc {
	return app.requireAuth(func(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
		if ctx.User.Role != RoleAdmin {
			http.Error(w, "403: доступ только для администратора", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path != "/attachments/upload" && r.URL.Path != "/attachments/drop" && r.URL.Path != "/admin/import" && !app.validCSRF(r, ctx.Session) {
			http.Error(w, "403: CSRF token mismatch", http.StatusForbidden)
			return
		}
		next(w, r, ctx)
	})
}

func (app *App) validCSRF(r *http.Request, s *Session) bool {
	if s == nil {
		return false
	}
	got := r.FormValue("csrf")
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.CSRFToken)) == 1
}

func (app *App) currentContext(r *http.Request) (*RequestContext, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return nil, false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return nil, false
	}
	sid, token := parts[0], parts[1]
	app.store.mu.RLock()
	defer app.store.mu.RUnlock()
	sess := app.store.db.Sessions[sid]
	if sess == nil || app.now().After(sess.ExpiresAt) {
		return nil, false
	}
	if !constantEqualHex(sess.TokenHash, sha256Hex(token)) {
		return nil, false
	}
	u := app.store.db.Users[sess.UserID]
	if u == nil || !u.Active {
		return nil, false
	}
	ucp, scp := *u, *sess
	return &RequestContext{User: &ucp, Session: &scp}, true
}

func (app *App) createSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return err
	}
	now := app.now().UTC()
	app.store.mu.Lock()
	defer app.store.mu.Unlock()
	sid := strconv.Itoa(app.store.db.NextSessionID)
	app.store.db.NextSessionID++
	app.store.db.Sessions[sid] = &Session{ID: sid, UserID: userID, TokenHash: sha256Hex(token), CSRFToken: csrf, CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}
	app.pruneSessionsLocked(now)
	if err := app.store.saveLocked(); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: sid + "." + token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: int(SessionTTL.Seconds())})
	return nil
}

func (app *App) destroySession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		parts := strings.Split(c.Value, ".")
		if len(parts) == 2 {
			app.store.mu.Lock()
			delete(app.store.db.Sessions, parts[0])
			_ = app.store.saveLocked()
			app.store.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (app *App) pruneSessionsLocked(now time.Time) {
	for id, s := range app.store.db.Sessions {
		if now.After(s.ExpiresAt) {
			delete(app.store.db.Sessions, id)
		}
	}
}
func (app *App) auditLocked(r *http.Request, actorID, action, target string) {
	app.store.db.Audit = append(app.store.db.Audit, AuditEvent{At: app.now().UTC(), ActorID: actorID, Action: action, Target: target, RemoteIP: clientIP(r), UserAgent: r.UserAgent()})
	if len(app.store.db.Audit) > 1000 {
		app.store.db.Audit = app.store.db.Audit[len(app.store.db.Audit)-1000:]
	}
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (l *LoginLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-LoginFailWindow)
	arr := l.fails[key]
	keep := arr[:0]
	for _, t := range arr {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	l.fails[key] = keep
	return len(keep) < MaxLoginFails
}
func (l *LoginLimiter) recordFail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.fails[key], now)
}
func (l *LoginLimiter) clear(key string) { l.mu.Lock(); defer l.mu.Unlock(); delete(l.fails, key) }

func (app *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, ok := app.currentContext(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		app.render(w, r, "Вход", nil, nil, loginBody(""))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, password := strings.TrimSpace(r.FormValue("username")), r.FormValue("password")
	key := clientIP(r) + ":" + strings.ToLower(username)
	if !app.limiter.allow(key, app.now()) {
		app.render(w, r, "Вход", nil, nil, loginBody("Слишком много попыток. Подождите 10 минут."))
		return
	}
	app.store.mu.RLock()
	var found *User
	for _, u := range app.store.db.Users {
		if strings.EqualFold(u.Username, username) {
			cp := *u
			found = &cp
			break
		}
	}
	app.store.mu.RUnlock()
	if found == nil || !found.Active || !VerifyPassword(password, found.SaltHex, found.PasswordHash) {
		app.limiter.recordFail(key, app.now())
		app.render(w, r, "Вход", nil, nil, loginBody("Неверный логин или пароль."))
		return
	}
	if err := app.createSession(w, r, found.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	app.limiter.clear(key)
	app.store.mu.Lock()
	app.auditLocked(r, found.ID, "login", "user:"+found.ID)
	_ = app.store.saveLocked()
	app.store.mu.Unlock()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func loginBody(msg string) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="login-shell"><div class="login-card"><div class="logo-mark">DH</div><h1>Docs Hub</h1><p class="muted">Закрытая база знаний с правами доступа.</p>`)
	if msg != "" {
		b.WriteString(`<p class="error">` + html.EscapeString(msg) + `</p>`)
	}
	b.WriteString(`<form method="post" action="/login"><label>Логин<input name="username" autocomplete="username" required autofocus></label><label>Пароль<input name="password" type="password" autocomplete="current-password" required></label><button class="primary">Войти</button></form></div></div>`)
	return template.HTML(b.String())
}
func (app *App) logout(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	app.destroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (app *App) home(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	tag := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tag")))
	app.store.mu.RLock()
	articles := SearchArticles(ctx.User, q, tag, app.store.db.Articles, app.store.db.Groups)
	tagCount := map[string]int{}
	for _, a := range app.store.db.Articles {
		if a.Archived || !CanRead(ctx.User, a, app.store.db.Groups) {
			continue
		}
		for _, t := range a.Tags {
			tagCount[t]++
		}
	}
	app.store.mu.RUnlock()
	if q == "" {
		sort.Slice(articles, func(i, j int) bool { return articles[i].UpdatedAt.After(articles[j].UpdatedAt) })
	}
	var b strings.Builder
	b.WriteString(`<section class="hero"><div><h1>Ваш серверный Obsidian</h1><p>Markdown, ACL, Toast UI Editor, вложения, backups, миграции и полнотекстовый поиск.</p></div><div class="hero-stat"><b>` + strconv.Itoa(len(articles)) + `</b><span>доступных статей</span></div></section>`)
	b.WriteString(`<div class="toolbar"><form class="search" method="get"><input name="q" value="` + html.EscapeString(q) + `" placeholder="Поиск по доступным статьям..."><button class="secondary">Найти</button></form>`)
	if ctx.User.Role == RoleAdmin {
		b.WriteString(`<a class="button primary" href="/edit/new">+ Новая</a><a class="button secondary" href="/admin/users">Пользователи</a><a class="button secondary" href="/admin/groups">Группы</a><a class="button secondary" href="/admin/audit">Аудит</a>`)
	}
	b.WriteString(`</div>`)
	if len(tagCount) > 0 {
		keys := []string{}
		for k := range tagCount {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(`<div class="tagbar"><a class="chip" href="/">все</a>`)
		for _, k := range keys {
			b.WriteString(`<a class="chip" href="/?tag=` + html.EscapeString(k) + `">#` + html.EscapeString(k) + `</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div class="grid">`)
	if len(articles) == 0 {
		b.WriteString(`<div class="card"><p>Нет доступных статей.</p></div>`)
	}
	for _, a := range articles {
		b.WriteString(`<a class="article-card" href="/a/` + html.EscapeString(a.Slug) + `"><div><h2>` + html.EscapeString(a.Title) + `</h2><p class="muted">/` + html.EscapeString(a.Slug) + ` · ` + html.EscapeString(a.UpdatedAt.Format("2006-01-02 15:04")) + `</p></div><div class="tags">`)
		for _, t := range a.Tags {
			b.WriteString(`<span>#` + html.EscapeString(t) + `</span>`)
		}
		b.WriteString(`</div>`)
		if a.AllUsers {
			b.WriteString(`<span class="badge">all users</span>`)
		} else {
			b.WriteString(`<span class="badge private">restricted</span>`)
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	app.render(w, r, "Статьи", ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) articleView(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	slug := sanitizeSlug(strings.TrimPrefix(r.URL.Path, "/a/"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	app.store.mu.RLock()
	var article *Article
	canView := false
	backlinks := []*Article{}
	attachments := []*Attachment{}
	for _, a := range app.store.db.Articles {
		if a.Slug == slug {
			cp := *a
			article = &cp
		}
	}
	if article != nil {
		canView = CanRead(ctx.User, article, app.store.db.Groups)
		for _, at := range app.store.db.Attachments {
			if at.ArticleID == article.ID {
				cp := *at
				attachments = append(attachments, &cp)
			}
		}
		for _, a := range app.store.db.Articles {
			if a.Archived || a.Slug == article.Slug || !CanRead(ctx.User, a, app.store.db.Groups) {
				continue
			}
			if articleLinksTo(a.Content, article.Slug) {
				cp := *a
				backlinks = append(backlinks, &cp)
			}
		}
	}
	app.store.mu.RUnlock()
	if article == nil || article.Archived || !canView {
		http.NotFound(w, r)
		return
	}
	var b strings.Builder
	b.WriteString(`<article class="article-layout"><main class="card article"><div class="article-top"><div><h1>` + html.EscapeString(article.Title) + `</h1><p class="muted">/` + html.EscapeString(article.Slug) + `</p></div>`)
	if ctx.User.Role == RoleAdmin {
		b.WriteString(`<a class="button primary" href="/edit/` + html.EscapeString(article.ID) + `">Редактировать</a>`)
	}
	b.WriteString(`</div><div class="tags">`)
	for _, t := range article.Tags {
		b.WriteString(`<a href="/?tag=` + html.EscapeString(t) + `">#` + html.EscapeString(t) + `</a>`)
	}
	b.WriteString(`</div><div class="markdown">` + string(RenderMarkdown(article.Content)) + `</div></main><aside class="side card"><h3>Вложения</h3>`)
	if len(attachments) == 0 {
		b.WriteString(`<p class="muted">Нет вложений.</p>`)
	} else {
		for _, f := range attachments {
			b.WriteString(`<a class="side-link" href="/files/` + html.EscapeString(f.ID) + `/` + html.EscapeString(f.OriginalName) + `">📎 ` + html.EscapeString(f.OriginalName) + `<br><span class="muted">` + html.EscapeString(humanBytes(f.Size)) + ` · ` + html.EscapeString(f.MIME) + `</span></a>`)
		}
	}
	b.WriteString(`<h3>Backlinks</h3>`)
	if len(backlinks) == 0 {
		b.WriteString(`<p class="muted">Нет обратных ссылок.</p>`)
	} else {
		for _, x := range backlinks {
			b.WriteString(`<a class="side-link" href="/a/` + html.EscapeString(x.Slug) + `">` + html.EscapeString(x.Title) + `</a>`)
		}
	}
	b.WriteString(`</aside></article>`)
	app.render(w, r, article.Title, ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) articlePreview(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	slug := sanitizeSlug(strings.TrimPrefix(r.URL.Path, "/preview/article/"))
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	app.store.mu.RLock()
	var article *Article
	for _, a := range app.store.db.Articles {
		if a.Slug == slug {
			cp := *a
			article = &cp
			break
		}
	}
	can := article != nil && !article.Archived && CanRead(ctx.User, article, app.store.db.Groups)
	app.store.mu.RUnlock()
	if !can {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<h3>` + html.EscapeString(article.Title) + `</h3>`)
	b.WriteString(`<div class="markdown">` + string(RenderMarkdown(article.Content)) + `</div>`)
	_, _ = io.WriteString(w, b.String())
}

func (app *App) articleEdit(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	id := strings.TrimPrefix(r.URL.Path, "/edit/")
	var a *Article
	title := "Новая статья"
	if id != "new" && id != "" {
		app.store.mu.RLock()
		if existing := app.store.db.Articles[id]; existing != nil {
			cp := *existing
			a = &cp
			title = "Редактирование: " + a.Title
		}
		app.store.mu.RUnlock()
		if a == nil {
			http.NotFound(w, r)
			return
		}
	} else {
		a = &Article{Content: "# Новая статья\n\nТекст...", Tags: []string{}}
	}
	app.store.mu.RLock()
	users := make([]*User, 0, len(app.store.db.Users))
	groups := make([]*Group, 0, len(app.store.db.Groups))
	attachments := []*Attachment{}
	for _, u := range app.store.db.Users {
		if u.Role != RoleAdmin {
			cp := *u
			users = append(users, &cp)
		}
	}
	for _, g := range app.store.db.Groups {
		cp := *g
		groups = append(groups, &cp)
	}
	if a.ID != "" {
		for _, at := range app.store.db.Attachments {
			if at.ArticleID == a.ID {
				cp := *at
				attachments = append(attachments, &cp)
			}
		}
	}
	app.store.mu.RUnlock()
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	allowedUsers := set(a.AllowedUserIDs)
	allowedGroups := set(a.AllowedGroupIDs)
	var b strings.Builder
	b.WriteString(`<form method="post" action="/save" id="editorForm"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="id" value="` + html.EscapeString(a.ID) + `"><section class="editor-meta card"><label>Название<input name="title" value="` + html.EscapeString(a.Title) + `" required></label><label>Slug<input name="slug" value="` + html.EscapeString(a.Slug) + `" placeholder="primer-stati"></label><label>Теги через запятую<input name="tags" value="` + html.EscapeString(strings.Join(a.Tags, ", ")) + `" placeholder="go, docs, internal"></label><label class="check"><input type="checkbox" name="all_users" value="1" ` + checked(a.AllUsers) + `> Доступна всем авторизованным</label><details open><summary>Доступ пользователям</summary><div class="checks">`)
	for _, u := range users {
		b.WriteString(`<label class="check"><input type="checkbox" name="allowed_users" value="` + html.EscapeString(u.ID) + `" ` + checked(allowedUsers[u.ID]) + `> ` + html.EscapeString(u.Username) + `</label>`)
	}
	b.WriteString(`</div></details><details open><summary>Доступ группам</summary><div class="checks">`)
	for _, g := range groups {
		b.WriteString(`<label class="check"><input type="checkbox" name="allowed_groups" value="` + html.EscapeString(g.ID) + `" ` + checked(allowedGroups[g.ID]) + `> ` + html.EscapeString(g.Name) + `</label>`)
	}
	b.WriteString(`</div></details><div class="actions"><button class="primary">Сохранить</button><a class="button secondary" href="/">Отмена</a>`)
	if a.ID != "" {
		b.WriteString(`<button class="danger" type="submit" form="archiveForm">В архив</button>`)
	}
	b.WriteString(`</div></section><section class="editor-toolbar card"><button type="button" class="secondary" onclick="insertWikiLink()">[[link]]</button><button type="button" class="secondary" onclick="insertVideoLink()">Видео/YouTube</button><span class="muted">WYSIWYG Markdown · media resize · GFM tables</span><span id="previewStatus" class="editor-status">загрузка</span></section><section class="live-editor-shell card" data-article-id="` + html.EscapeString(a.ID) + `"><textarea id="md" name="content" class="hidden-textarea" spellcheck="false">` + html.EscapeString(a.Content) + `</textarea><div id="toastEditor" class="toast-editor-host"></div><div id="dropHint" class="drop-hint">Отпустите файл, чтобы загрузить и вставить в статью</div></section></form>`)
	if a.ID != "" {
		b.WriteString(`<section class="card attachments-admin"><h2>Вложения</h2><form method="post" action="/attachments/upload" enctype="multipart/form-data" class="inline-form"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="article_id" value="` + html.EscapeString(a.ID) + `"><label>Файл<input type="file" name="file" required></label><button class="primary">Загрузить</button></form>`)
		if len(attachments) == 0 {
			b.WriteString(`<p class="muted">Файлов пока нет.</p>`)
		} else {
			b.WriteString(`<table><thead><tr><th>Файл</th><th>Размер</th><th>Markdown</th><th></th></tr></thead><tbody>`)
			for _, f := range attachments {
				link := `/files/` + html.EscapeString(f.ID) + `/` + html.EscapeString(f.OriginalName)
				b.WriteString(`<tr><td><a href="` + link + `">` + html.EscapeString(f.OriginalName) + `</a><br><span class="muted">` + html.EscapeString(f.MIME) + `</span></td><td>` + html.EscapeString(humanBytes(f.Size)) + `</td><td><code>[` + html.EscapeString(f.OriginalName) + `](` + link + `)</code></td><td><form method="post" action="/attachments/delete"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="id" value="` + html.EscapeString(f.ID) + `"><button class="danger">Удалить</button></form></td></tr>`)
			}
			b.WriteString(`</tbody></table>`)
		}
		b.WriteString(`</section>`)
	} else {
		b.WriteString(`<section class="card"><h2>Вложения</h2><p class="muted">Сначала сохраните статью, потом загрузите файлы.</p></section>`)
	}
	if a.ID != "" {
		b.WriteString(`<form method="post" action="/archive/` + html.EscapeString(a.ID) + `" id="archiveForm" onsubmit="return confirm('Отправить статью в архив?')"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"></form>`)
	}
	b.WriteString(editorScript())
	app.render(w, r, title, ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) adminPreview(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, string(RenderMarkdown(r.FormValue("content"))))
}

func (app *App) adminDraft(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := app.now().UTC()
	app.store.mu.Lock()
	id := strconv.Itoa(app.store.db.NextArticleID)
	app.store.db.NextArticleID++
	slug := "draft-" + id
	app.store.db.Articles[id] = &Article{ID: id, Title: "Новая статья", Slug: slug, Content: "", Tags: []string{}, AllUsers: false, OwnerID: ctx.User.ID, CreatedAt: now, UpdatedAt: now}
	app.auditLocked(r, ctx.User.ID, "article.draft", "article:"+id)
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "slug": slug, "edit_url": "/edit/" + id})
}

type markdownImportArticle struct {
	Path    string
	Title   string
	Slug    string
	Content string
	Tags    []string
}

type markdownImportResult struct {
	Title string
	Slug  string
}

func (app *App) adminImport(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method == http.MethodGet {
		var b strings.Builder
		b.WriteString(`<section class="card"><h1>Импорт Markdown</h1><p class="muted">Выберите папку: все .md файлы из неё и подпапок будут загружены как отдельные закрытые статьи.</p><form method="post" action="/admin/import" enctype="multipart/form-data" class="inline-form"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><label>Папка<input type="file" name="files" multiple webkitdirectory accept=".md,text/markdown,text/plain" required></label><button class="primary">Импортировать</button></form></section>`)
		app.render(w, r, "Импорт Markdown", ctx.User, ctx.Session, template.HTML(b.String()))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxImportBytes+1<<20)
	if err := r.ParseMultipartForm(MaxImportBytes); err != nil {
		http.Error(w, "папка слишком большая или форма повреждена", http.StatusBadRequest)
		return
	}
	if !app.validCSRF(r, ctx.Session) {
		http.Error(w, "403: CSRF token mismatch", http.StatusForbidden)
		return
	}
	files := r.MultipartForm.File["files"]
	articles, skipped, err := parseMarkdownImportFiles(files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	imported, err := app.createImportedArticles(r, ctx, articles)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Импорт Markdown</h1><p class="muted">Импортировано: ` + strconv.Itoa(len(imported)) + `. Пропущено: ` + strconv.Itoa(skipped) + `.</p>`)
	if len(imported) == 0 {
		b.WriteString(`<p>Подходящих .md файлов не найдено.</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>Статья</th><th>Slug</th></tr></thead><tbody>`)
		for _, it := range imported {
			b.WriteString(`<tr><td><a href="/a/` + html.EscapeString(it.Slug) + `">` + html.EscapeString(it.Title) + `</a></td><td><span class="muted">/` + html.EscapeString(it.Slug) + `</span></td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`<div class="actions"><a class="button primary" href="/admin/import">Импортировать ещё</a><a class="button secondary" href="/">На главную</a></div></section>`)
	app.render(w, r, "Импорт Markdown", ctx.User, ctx.Session, template.HTML(b.String()))
}

func parseMarkdownImportFiles(files []*multipart.FileHeader) ([]markdownImportArticle, int, error) {
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].Filename) < strings.ToLower(files[j].Filename)
	})
	out := []markdownImportArticle{}
	skipped := 0
	for _, header := range files {
		if strings.ToLower(filepath.Ext(header.Filename)) != ".md" {
			skipped++
			continue
		}
		if header.Size == 0 {
			skipped++
			continue
		}
		if header.Size > MaxImportFileBytes {
			return nil, skipped, fmt.Errorf("%s превышает лимит 5 MiB", header.Filename)
		}
		file, err := header.Open()
		if err != nil {
			return nil, skipped, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, MaxImportFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, skipped, readErr
		}
		if closeErr != nil {
			return nil, skipped, closeErr
		}
		if len(data) == 0 {
			skipped++
			continue
		}
		if int64(len(data)) > MaxImportFileBytes {
			return nil, skipped, fmt.Errorf("%s превышает лимит 5 MiB", header.Filename)
		}
		content := string(data)
		out = append(out, markdownImportArticle{
			Path:    header.Filename,
			Title:   markdownImportTitle(content, header.Filename),
			Slug:    markdownImportSlug(header.Filename),
			Content: content,
			Tags:    markdownImportTags(content),
		})
	}
	return out, skipped, nil
}

func (app *App) createImportedArticles(r *http.Request, ctx *RequestContext, imports []markdownImportArticle) ([]markdownImportResult, error) {
	if len(imports) == 0 {
		return nil, nil
	}
	now := app.now().UTC()
	app.store.mu.Lock()
	defer app.store.mu.Unlock()

	existingSlugs := map[string]bool{}
	for _, a := range app.store.db.Articles {
		if a != nil && !a.Archived && a.Slug != "" {
			existingSlugs[a.Slug] = true
		}
	}
	oldNextArticleID := app.store.db.NextArticleID
	oldAudit := append([]AuditEvent(nil), app.store.db.Audit...)
	addedIDs := []string{}
	imported := []markdownImportResult{}
	for _, in := range imports {
		id := strconv.Itoa(app.store.db.NextArticleID)
		app.store.db.NextArticleID++
		slug := nextAvailableSlug(in.Slug, existingSlugs)
		existingSlugs[slug] = true
		app.store.db.Articles[id] = &Article{
			ID:              id,
			Title:           in.Title,
			Slug:            slug,
			Content:         in.Content,
			Tags:            in.Tags,
			AllUsers:        false,
			AllowedUserIDs:  []string{},
			AllowedGroupIDs: []string{},
			OwnerID:         ctx.User.ID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		addedIDs = append(addedIDs, id)
		imported = append(imported, markdownImportResult{Title: in.Title, Slug: slug})
	}
	app.auditLocked(r, ctx.User.ID, "article.import", "articles:"+strconv.Itoa(len(imported)))
	if err := app.store.saveLocked(); err != nil {
		for _, id := range addedIDs {
			delete(app.store.db.Articles, id)
		}
		app.store.db.NextArticleID = oldNextArticleID
		app.store.db.Audit = oldAudit
		return nil, err
	}
	return imported, nil
}

func markdownImportTitle(content, filename string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.Trim(line[2:], "# \t"))
			if title != "" {
				return title
			}
		}
	}
	name := importBaseName(filename)
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if title == "" {
		return "Без названия"
	}
	return title
}

func markdownImportSlug(filename string) string {
	name := importBaseName(filename)
	slug := sanitizeSlug(strings.TrimSuffix(name, filepath.Ext(name)))
	if slug == "" {
		return "article"
	}
	return slug
}

func markdownImportTags(content string) []string {
	re := regexp.MustCompile(`(^|[\s\(\[\{])#([\p{L}\p{N}_-]+)`)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		tag := sanitizeSlug(m[2])
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

func importBaseName(filename string) string {
	filename = strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")
	if i := strings.LastIndex(filename, "/"); i >= 0 {
		filename = filename[i+1:]
	}
	return filename
}

func nextAvailableSlug(base string, existing map[string]bool) string {
	base = sanitizeSlug(base)
	if base == "" {
		base = "article"
	}
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !existing[candidate] {
			return candidate
		}
	}
}

func (app *App) articleSave(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, title, slug := strings.TrimSpace(r.FormValue("id")), strings.TrimSpace(r.FormValue("title")), sanitizeSlug(r.FormValue("slug"))
	if title == "" {
		title = "Без названия"
	}
	if slug == "" {
		slug = sanitizeSlug(title)
	}
	if slug == "" {
		slug = fmt.Sprintf("article-%d", app.now().Unix())
	}
	content := r.FormValue("content")
	tags := parseTags(r.FormValue("tags"))
	allUsers := r.FormValue("all_users") == "1"
	allowedUsers := uniqueStrings(r.Form["allowed_users"])
	allowedGroups := uniqueStrings(r.Form["allowed_groups"])
	now := app.now().UTC()
	app.store.mu.Lock()
	defer app.store.mu.Unlock()
	for _, other := range app.store.db.Articles {
		if other.Slug == slug && other.ID != id && !other.Archived {
			http.Error(w, "slug уже занят", http.StatusBadRequest)
			return
		}
	}
	if id == "" {
		id = strconv.Itoa(app.store.db.NextArticleID)
		app.store.db.NextArticleID++
		app.store.db.Articles[id] = &Article{ID: id, OwnerID: ctx.User.ID, CreatedAt: now}
	}
	a := app.store.db.Articles[id]
	if a == nil {
		http.NotFound(w, r)
		return
	}
	if a.Content != "" || a.Title != "" {
		a.Versions = append(a.Versions, Version{At: now, ActorID: ctx.User.ID, Title: a.Title, Slug: a.Slug, Content: a.Content})
		if len(a.Versions) > 20 {
			a.Versions = a.Versions[len(a.Versions)-20:]
		}
	}
	a.Title = title
	a.Slug = slug
	a.Content = content
	a.Tags = tags
	a.AllUsers = allUsers
	a.AllowedUserIDs = allowedUsers
	a.AllowedGroupIDs = allowedGroups
	a.UpdatedAt = now
	a.Archived = false
	app.auditLocked(r, ctx.User.ID, "article.save", "article:"+id)
	if err := app.store.saveLocked(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/a/"+a.Slug, http.StatusSeeOther)
}

func (app *App) articleArchive(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/archive/")
	app.store.mu.Lock()
	defer app.store.mu.Unlock()
	if a := app.store.db.Articles[id]; a != nil {
		a.Archived = true
		a.UpdatedAt = app.now().UTC()
		app.store.db.RibbonArticleIDs = removeString(app.store.db.RibbonArticleIDs, id)
		app.auditLocked(r, ctx.User.ID, "article.archive", "article:"+id)
	}
	if err := app.store.saveLocked(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *App) adminUsers(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	app.render(w, r, "Пользователи", ctx.User, ctx.Session, app.adminUsersBody(ctx, ""))
}
func (app *App) adminUsersBody(ctx *RequestContext, msg string) template.HTML {
	app.store.mu.RLock()
	users := []*User{}
	for _, u := range app.store.db.Users {
		cp := *u
		users = append(users, &cp)
	}
	app.store.mu.RUnlock()
	sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.Before(users[j].CreatedAt) })
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Пользователи</h1>`)
	if msg != "" {
		b.WriteString(`<p class="ok">` + html.EscapeString(msg) + `</p>`)
	}
	b.WriteString(`<form method="post" action="/admin/users/create" class="inline-form"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><label>Логин<input name="username" required></label><label>Пароль <span class="muted">пусто = сгенерировать</span><input name="password"></label><label>Роль<select name="role"><option value="user">user</option><option value="admin">admin</option></select></label><button class="primary">Создать</button></form></section><section class="card"><table><thead><tr><th>ID</th><th>Логин</th><th>Роль</th><th>Статус</th><th>Пароль</th><th></th></tr></thead><tbody>`)
	for _, u := range users {
		b.WriteString(`<tr><td>` + html.EscapeString(u.ID) + `</td><td>` + html.EscapeString(u.Username) + `</td><td>` + html.EscapeString(string(u.Role)) + `</td><td>`)
		if u.Active {
			b.WriteString(`<span class="badge">active</span>`)
		} else {
			b.WriteString(`<span class="badge private">disabled</span>`)
		}
		b.WriteString(`</td><td><form method="post" action="/admin/users/password" class="mini-form"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="id" value="` + html.EscapeString(u.ID) + `"><input name="password" placeholder="новый пароль" required><button>Сменить</button></form></td><td><form method="post" action="/admin/users/toggle"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="id" value="` + html.EscapeString(u.ID) + `"><button class="secondary">`)
		if u.Active {
			b.WriteString(`Выключить`)
		} else {
			b.WriteString(`Включить`)
		}
		b.WriteString(`</button></form></td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return template.HTML(b.String())
}
func (app *App) adminUserCreate(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	role := UserRole(r.FormValue("role"))
	if role != RoleAdmin {
		role = RoleUser
	}
	if username == "" {
		http.Error(w, "логин пустой", 400)
		return
	}
	generated := false
	if password == "" {
		generated = true
		password, _ = randomToken(9)
	}
	salt, hash, err := HashPassword(password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	app.store.mu.Lock()
	for _, ex := range app.store.db.Users {
		if strings.EqualFold(ex.Username, username) {
			app.store.mu.Unlock()
			http.Error(w, "пользователь уже существует", 400)
			return
		}
	}
	id := strconv.Itoa(app.store.db.NextUserID)
	app.store.db.NextUserID++
	now := app.now().UTC()
	app.store.db.Users[id] = &User{ID: id, Username: username, SaltHex: salt, PasswordHash: hash, Role: role, Active: true, CreatedAt: now, UpdatedAt: now}
	app.auditLocked(r, ctx.User.ID, "user.create", "user:"+id)
	err = app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	msg := "Пользователь создан."
	if generated {
		msg = "Пользователь создан. Сгенерированный пароль: " + password
	}
	app.render(w, r, "Пользователи", ctx.User, ctx.Session, app.adminUsersBody(ctx, msg))
}
func (app *App) adminUserPassword(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id, password := r.FormValue("id"), r.FormValue("password")
	if password == "" {
		http.Error(w, "пароль пустой", 400)
		return
	}
	salt, hash, err := HashPassword(password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	app.store.mu.Lock()
	u := app.store.db.Users[id]
	if u == nil {
		app.store.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	u.SaltHex = salt
	u.PasswordHash = hash
	u.UpdatedAt = app.now().UTC()
	for sid, sess := range app.store.db.Sessions {
		if sess.UserID == id {
			delete(app.store.db.Sessions, sid)
		}
	}
	app.auditLocked(r, ctx.User.ID, "user.password", "user:"+id)
	err = app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	app.render(w, r, "Пользователи", ctx.User, ctx.Session, app.adminUsersBody(ctx, "Пароль изменён."))
}
func (app *App) adminUserToggle(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := r.FormValue("id")
	app.store.mu.Lock()
	u := app.store.db.Users[id]
	if u == nil {
		app.store.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if u.ID != ctx.User.ID {
		u.Active = !u.Active
		u.UpdatedAt = app.now().UTC()
		app.auditLocked(r, ctx.User.ID, "user.toggle", "user:"+id)
	}
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/users", 303)
}

func (app *App) adminGroups(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	app.render(w, r, "Группы", ctx.User, ctx.Session, app.adminGroupsBody(ctx, ""))
}
func (app *App) adminGroupsBody(ctx *RequestContext, msg string) template.HTML {
	app.store.mu.RLock()
	users := []*User{}
	groups := []*Group{}
	for _, u := range app.store.db.Users {
		if u.Role != RoleAdmin {
			cp := *u
			users = append(users, &cp)
		}
	}
	for _, g := range app.store.db.Groups {
		cp := *g
		groups = append(groups, &cp)
	}
	app.store.mu.RUnlock()
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Группы</h1>`)
	if msg != "" {
		b.WriteString(`<p class="ok">` + html.EscapeString(msg) + `</p>`)
	}
	b.WriteString(`</section>`)
	for _, g := range groups {
		members := set(g.MemberIDs)
		b.WriteString(`<section class="card"><form method="post" action="/admin/groups/save"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><input type="hidden" name="id" value="` + html.EscapeString(g.ID) + `"><label>Название<input name="name" value="` + html.EscapeString(g.Name) + `" required></label><div class="checks">`)
		for _, u := range users {
			b.WriteString(`<label class="check"><input type="checkbox" name="members" value="` + html.EscapeString(u.ID) + `" ` + checked(members[u.ID]) + `> ` + html.EscapeString(u.Username) + `</label>`)
		}
		b.WriteString(`</div><div class="actions"><button class="primary">Сохранить</button><button class="danger" formaction="/admin/groups/delete">Удалить</button></div></form></section>`)
	}
	b.WriteString(`<section class="card"><h2>Новая группа</h2><form method="post" action="/admin/groups/save" class="inline-form"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><label>Название<input name="name" required></label><button class="primary">Создать</button></form></section>`)
	return template.HTML(b.String())
}
func (app *App) adminGroupSave(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id, name := strings.TrimSpace(r.FormValue("id")), strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "название пустое", 400)
		return
	}
	members := uniqueStrings(r.Form["members"])
	now := app.now().UTC()
	app.store.mu.Lock()
	if id == "" {
		id = strconv.Itoa(app.store.db.NextGroupID)
		app.store.db.NextGroupID++
		app.store.db.Groups[id] = &Group{ID: id, CreatedAt: now}
	}
	g := app.store.db.Groups[id]
	if g == nil {
		app.store.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	g.Name = name
	g.MemberIDs = members
	g.UpdatedAt = now
	app.auditLocked(r, ctx.User.ID, "group.save", "group:"+id)
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/groups", 303)
}
func (app *App) adminGroupDelete(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	id := r.FormValue("id")
	app.store.mu.Lock()
	delete(app.store.db.Groups, id)
	for _, a := range app.store.db.Articles {
		a.AllowedGroupIDs = removeString(a.AllowedGroupIDs, id)
	}
	app.auditLocked(r, ctx.User.ID, "group.delete", "group:"+id)
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/groups", 303)
}

func (app *App) adminRibbon(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	app.store.mu.RLock()
	articles := make([]*Article, 0, len(app.store.db.Articles))
	for _, a := range app.store.db.Articles {
		if a.Archived {
			continue
		}
		cp := *a
		articles = append(articles, &cp)
	}
	pinned := set(app.store.db.RibbonArticleIDs)
	order := map[string]int{}
	for i, id := range app.store.db.RibbonArticleIDs {
		order[id] = i + 1
	}
	app.store.mu.RUnlock()
	sort.Slice(articles, func(i, j int) bool {
		oi, okI := order[articles[i].ID]
		oj, okJ := order[articles[j].ID]
		if okI != okJ {
			return okI
		}
		if okI && oj != oi {
			return oi < oj
		}
		return strings.ToLower(articles[i].Title) < strings.ToLower(articles[j].Title)
	})
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Лента статей</h1><p class="muted">Выберите статьи, которые будут закреплены в левой ленте для пользователей с доступом к ним.</p><form method="post" action="/admin/ribbon/save"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><table><thead><tr><th>В ленте</th><th>Позиция</th><th>Статья</th><th>Slug</th></tr></thead><tbody>`)
	for i, a := range articles {
		pos := order[a.ID]
		if pos == 0 {
			pos = i + 1
		}
		b.WriteString(`<tr><td><label class="check"><input type="checkbox" name="article_ids" value="` + html.EscapeString(a.ID) + `" ` + checked(pinned[a.ID]) + `> показать</label></td><td><input class="order-input" name="order_` + html.EscapeString(a.ID) + `" value="` + strconv.Itoa(pos) + `" inputmode="numeric"></td><td><a href="/a/` + html.EscapeString(a.Slug) + `">` + html.EscapeString(a.Title) + `</a></td><td><span class="muted">/` + html.EscapeString(a.Slug) + `</span></td></tr>`)
	}
	b.WriteString(`</tbody></table><div class="actions"><button class="primary">Сохранить ленту</button><a class="button secondary" href="/">Отмена</a></div></form></section>`)
	app.render(w, r, "Лента статей", ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) adminRibbonSave(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	type chosen struct {
		id    string
		order int
		idx   int
	}
	items := []chosen{}
	for i, id := range uniqueStrings(r.Form["article_ids"]) {
		order, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("order_" + id)))
		if order <= 0 {
			order = 9999 + i
		}
		items = append(items, chosen{id: id, order: order, idx: i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].order == items[j].order {
			return items[i].idx < items[j].idx
		}
		return items[i].order < items[j].order
	})
	app.store.mu.Lock()
	out := []string{}
	for _, it := range items {
		a := app.store.db.Articles[it.id]
		if a == nil || a.Archived {
			continue
		}
		out = append(out, it.id)
	}
	app.store.db.RibbonArticleIDs = out
	app.auditLocked(r, ctx.User.ID, "ribbon.save", "articles:"+strconv.Itoa(len(out)))
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/ribbon", http.StatusSeeOther)
}

func (app *App) adminAudit(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	app.store.mu.RLock()
	events := append([]AuditEvent(nil), app.store.db.Audit...)
	users := map[string]string{}
	for id, u := range app.store.db.Users {
		users[id] = u.Username
	}
	app.store.mu.RUnlock()
	sort.Slice(events, func(i, j int) bool { return events[i].At.After(events[j].At) })
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Аудит</h1><table><thead><tr><th>Время</th><th>Кто</th><th>Действие</th><th>Цель</th><th>IP</th></tr></thead><tbody>`)
	for _, e := range events {
		b.WriteString(`<tr><td>` + html.EscapeString(e.At.Format("2006-01-02 15:04:05")) + `</td><td>` + html.EscapeString(users[e.ActorID]) + `</td><td>` + html.EscapeString(e.Action) + `</td><td>` + html.EscapeString(e.Target) + `</td><td>` + html.EscapeString(e.RemoteIP) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	app.render(w, r, "Аудит", ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) attachmentUpload(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		http.Error(w, "файл слишком большой или форма повреждена", http.StatusBadRequest)
		return
	}
	if !app.validCSRF(r, ctx.Session) {
		http.Error(w, "403: CSRF token mismatch", http.StatusForbidden)
		return
	}
	articleID := strings.TrimSpace(r.FormValue("article_id"))
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "файл не найден", http.StatusBadRequest)
		return
	}
	defer file.Close()
	att, status, msg := app.saveUploadedAttachment(r, ctx, articleID, file, header)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	http.Redirect(w, r, "/edit/"+att.ArticleID, http.StatusSeeOther)
}

func (app *App) attachmentDrop(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		http.Error(w, "файл слишком большой или форма повреждена", http.StatusBadRequest)
		return
	}
	if !app.validCSRF(r, ctx.Session) {
		http.Error(w, "403: CSRF token mismatch", http.StatusForbidden)
		return
	}
	articleID := strings.TrimSpace(r.FormValue("article_id"))
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "файл не найден", http.StatusBadRequest)
		return
	}
	defer file.Close()
	att, status, msg := app.saveUploadedAttachment(r, ctx, articleID, file, header)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	url := "/files/" + att.ID + "/" + att.OriginalName
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":       att.ID,
		"name":     att.OriginalName,
		"url":      url,
		"mime":     att.MIME,
		"markdown": attachmentMarkdown(att, url),
		"html":     attachmentHTML(att, url),
	})
}

func (app *App) saveUploadedAttachment(r *http.Request, ctx *RequestContext, articleID string, file multipart.File, header *multipart.FileHeader) (*Attachment, int, string) {
	if header.Size > MaxUploadBytes {
		return nil, http.StatusBadRequest, "файл больше лимита 25 MB"
	}
	original := safeFileName(header.Filename)
	if original == "" {
		return nil, http.StatusBadRequest, "пустое имя файла"
	}
	ext := strings.ToLower(filepath.Ext(original))
	if !allowedUploadExt(ext) {
		return nil, http.StatusBadRequest, "тип файла запрещён"
	}
	buf := make([]byte, 512)
	n, _ := io.ReadFull(file, buf)
	buf = buf[:n]
	mimeType := http.DetectContentType(buf)
	if m := mime.TypeByExtension(ext); m != "" && mimeType == "application/octet-stream" {
		mimeType = strings.Split(m, ";")[0]
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, http.StatusInternalServerError, err.Error()
	}

	app.store.mu.Lock()
	article := app.store.db.Articles[articleID]
	if article == nil || article.Archived {
		app.store.mu.Unlock()
		return nil, http.StatusNotFound, "статья не найдена"
	}
	id := strconv.Itoa(app.store.db.NextAttachmentID)
	app.store.db.NextAttachmentID++
	randPart, _ := randomToken(10)
	stored := id + "_" + randPart + ext
	att := &Attachment{ID: id, ArticleID: articleID, StoredName: stored, OriginalName: original, MIME: mimeType, Size: header.Size, UploadedBy: ctx.User.ID, CreatedAt: app.now().UTC()}
	app.store.db.Attachments[id] = att
	article.UpdatedAt = app.now().UTC()
	app.auditLocked(r, ctx.User.ID, "attachment.upload", "attachment:"+id)
	uploadsDir := app.store.uploadsDir()
	if err := os.MkdirAll(uploadsDir, 0750); err != nil {
		delete(app.store.db.Attachments, id)
		app.store.mu.Unlock()
		return nil, http.StatusInternalServerError, err.Error()
	}
	dstPath := filepath.Join(uploadsDir, stored)
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		delete(app.store.db.Attachments, id)
		app.store.mu.Unlock()
		return nil, http.StatusInternalServerError, err.Error()
	}
	_, copyErr := io.Copy(dst, io.LimitReader(file, MaxUploadBytes+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dstPath)
		delete(app.store.db.Attachments, id)
		app.store.mu.Unlock()
		return nil, http.StatusInternalServerError, "ошибка сохранения файла"
	}
	if err := app.store.saveLocked(); err != nil {
		_ = os.Remove(dstPath)
		delete(app.store.db.Attachments, id)
		app.store.mu.Unlock()
		return nil, http.StatusInternalServerError, err.Error()
	}
	app.store.mu.Unlock()
	return att, http.StatusOK, ""
}

func (app *App) attachmentDelete(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	app.store.mu.Lock()
	att := app.store.db.Attachments[id]
	if att == nil {
		app.store.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(app.store.uploadsDir(), att.StoredName)
	articleID := att.ArticleID
	delete(app.store.db.Attachments, id)
	if a := app.store.db.Articles[articleID]; a != nil {
		a.UpdatedAt = app.now().UTC()
	}
	app.auditLocked(r, ctx.User.ID, "attachment.delete", "attachment:"+id)
	err := app.store.saveLocked()
	app.store.mu.Unlock()
	_ = os.Remove(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/edit/"+articleID, http.StatusSeeOther)
}

func (app *App) attachmentServe(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/files/"), "/")
	if i := strings.IndexByte(id, '/'); i >= 0 {
		id = id[:i]
	}
	app.store.mu.RLock()
	att := app.store.db.Attachments[id]
	if att == nil {
		app.store.mu.RUnlock()
		http.NotFound(w, r)
		return
	}
	article := app.store.db.Articles[att.ArticleID]
	can := article != nil && !article.Archived && CanRead(ctx.User, article, app.store.db.Groups)
	copyAtt := *att
	path := filepath.Join(app.store.uploadsDir(), copyAtt.StoredName)
	app.store.mu.RUnlock()
	if !can {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", copyAtt.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if isInlineSafe(copyAtt.MIME) {
		w.Header().Set("Content-Disposition", `inline; filename="`+escapeHeaderFilename(copyAtt.OriginalName)+`"`)
	} else {
		w.Header().Set("Content-Disposition", `attachment; filename="`+escapeHeaderFilename(copyAtt.OriginalName)+`"`)
	}
	http.ServeFile(w, r, path)
}

func (app *App) adminBackups(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	_ = os.MkdirAll(app.store.backupsDir(), 0750)
	entries, _ := os.ReadDir(app.store.backupsDir())
	type item struct {
		name string
		size int64
		mod  time.Time
	}
	items := []item{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err == nil {
			items = append(items, item{e.Name(), info.Size(), info.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	var b strings.Builder
	b.WriteString(`<section class="card"><h1>Резервные копии</h1><p class="muted">Backup включает storage.json и каталог uploads. Храните копии вне сервера.</p><form method="post" action="/admin/backups/create"><input type="hidden" name="csrf" value="` + html.EscapeString(ctx.Session.CSRFToken) + `"><button class="primary">Создать backup</button></form></section><section class="card"><table><thead><tr><th>Файл</th><th>Размер</th><th>Дата</th></tr></thead><tbody>`)
	for _, it := range items {
		b.WriteString(`<tr><td><a href="/admin/backups/download/` + html.EscapeString(it.name) + `">` + html.EscapeString(it.name) + `</a></td><td>` + html.EscapeString(humanBytes(it.size)) + `</td><td>` + html.EscapeString(it.mod.Format("2006-01-02 15:04:05")) + `</td></tr>`)
	}
	if len(items) == 0 {
		b.WriteString(`<tr><td colspan="3" class="muted">Пока нет резервных копий.</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	app.render(w, r, "Backups", ctx.User, ctx.Session, template.HTML(b.String()))
}

func (app *App) adminBackupCreate(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, err := app.createBackup(r, ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/backups/download/"+name, http.StatusSeeOther)
}

func (app *App) adminBackupDownload(w http.ResponseWriter, r *http.Request, ctx *RequestContext) {
	name := safeFileName(strings.TrimPrefix(r.URL.Path, "/admin/backups/download/"))
	if name == "" || !strings.HasSuffix(name, ".zip") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(app.store.backupsDir(), name)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+escapeHeaderFilename(name)+`"`)
	http.ServeFile(w, r, path)
}

func (app *App) createBackup(r *http.Request, ctx *RequestContext) (string, error) {
	if err := os.MkdirAll(app.store.backupsDir(), 0750); err != nil {
		return "", err
	}
	name := "docs-hub-backup-" + app.now().UTC().Format("20060102-150405") + ".zip"
	outPath := filepath.Join(app.store.backupsDir(), name)
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(out)
	closed := false
	closeOut := func() error {
		if closed {
			return nil
		}
		closed = true
		return out.Close()
	}
	defer func() { _ = closeOut() }()
	cleanup := func() {
		_ = zw.Close()
		_ = closeOut()
		_ = os.Remove(outPath)
	}
	addFile := func(zipName, realPath string) error {
		info, err := os.Stat(realPath)
		if err != nil || info.IsDir() {
			return err
		}
		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = zipName
		fh.Method = zip.Deflate
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		f, err := os.Open(realPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
	if err := addFile("storage.json", app.store.path); err != nil {
		cleanup()
		return "", err
	}
	uploadsDir := app.store.uploadsDir()
	if info, err := os.Stat(uploadsDir); err == nil {
		if !info.IsDir() {
			cleanup()
			return "", fmt.Errorf("uploads path is not a directory: %s", uploadsDir)
		}
		if err := filepath.WalkDir(uploadsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(uploadsDir, path)
			if err != nil {
				return err
			}
			return addFile(filepath.ToSlash(filepath.Join("uploads", rel)), path)
		}); err != nil {
			cleanup()
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		cleanup()
		return "", err
	}
	if err := zw.Close(); err != nil {
		_ = closeOut()
		_ = os.Remove(outPath)
		return "", err
	}
	if err := closeOut(); err != nil {
		_ = os.Remove(outPath)
		return "", err
	}
	app.store.mu.Lock()
	app.auditLocked(r, ctx.User.ID, "backup.create", "backup:"+name)
	err = app.store.saveLocked()
	app.store.mu.Unlock()
	return name, err
}

func SearchArticles(u *User, query, tag string, articles map[string]*Article, groups map[string]*Group) []*Article {
	query = strings.TrimSpace(strings.ToLower(query))
	tag = strings.TrimSpace(strings.ToLower(tag))
	tokens := tokenize(query)
	type scored struct {
		a     *Article
		score int
	}
	out := []scored{}
	for _, a := range articles {
		if a.Archived || !CanRead(u, a, groups) {
			continue
		}
		if tag != "" && !contains(a.Tags, tag) {
			continue
		}
		score := 1
		if query != "" {
			title := strings.ToLower(a.Title)
			body := strings.ToLower(a.Content)
			hay := title + " " + strings.ToLower(a.Slug) + " " + body + " " + strings.Join(a.Tags, " ")
			if strings.Contains(title, query) {
				score += 50
			}
			if strings.Contains(body, query) {
				score += 20
			}
			for _, tok := range tokens {
				if strings.Contains(title, tok) {
					score += 15
				}
				if strings.Contains(hay, tok) {
					score += strings.Count(hay, tok)
				}
			}
			if score == 1 {
				continue
			}
		}
		cp := *a
		out = append(out, scored{&cp, score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score == out[j].score {
			return out[i].a.UpdatedAt.After(out[j].a.UpdatedAt)
		}
		return out[i].score > out[j].score
	})
	res := make([]*Article, 0, len(out))
	for _, x := range out {
		res = append(res, x.a)
	}
	return res
}

func tokenize(s string) []string {
	m := map[string]bool{}
	var b strings.Builder
	flush := func(out *[]string) {
		if b.Len() >= 2 {
			t := b.String()
			if !m[t] {
				m[t] = true
				*out = append(*out, t)
			}
		}
		b.Reset()
	}
	out := []string{}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			flush(&out)
		}
	}
	flush(&out)
	return out
}

func allowedUploadExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".pdf", ".txt", ".md", ".csv", ".json", ".zip", ".docx", ".xlsx", ".pptx", ".mp3", ".wav", ".ogg", ".m4a", ".mp4", ".webm", ".mov":
		return true
	default:
		return false
	}
}

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' || r == ' ' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, ". -")
	if len(name) > 180 {
		name = name[:180]
	}
	return name
}

func isInlineSafe(m string) bool {
	return strings.HasPrefix(m, "image/") || strings.HasPrefix(m, "audio/") || strings.HasPrefix(m, "video/") || strings.HasPrefix(m, "text/plain") || m == "application/pdf"
}

func attachmentMarkdown(att *Attachment, url string) string {
	if att == nil {
		return ""
	}
	label := markdownLabel(att.OriginalName)
	if strings.HasPrefix(att.MIME, "image/") {
		return "![" + label + "|100%](" + url + ")"
	}
	if strings.HasPrefix(att.MIME, "video/") {
		return "![" + label + "|760](" + url + ")"
	}
	if strings.HasPrefix(att.MIME, "audio/") {
		return "![" + label + "](" + url + ")"
	}
	return "[" + label + "](" + url + ")"
}

func attachmentHTML(att *Attachment, url string) string {
	if att == nil {
		return ""
	}
	return renderEmbeddedFile(att.OriginalName, url, att.MIME)
}

func renderEmbeddedFile(alt, url, mimeType string) string {
	return renderEmbeddedFileSized(alt, url, mimeType, "")
}

func renderEmbeddedFileSized(alt, rawURL, mimeType, sizeOverride string) string {
	label, size := splitMediaLabel(alt)
	if sizeOverride != "" {
		size = sizeOverride
	}
	cleanURL, ok := cleanMediaURL(rawURL)
	if !ok {
		return html.EscapeString(label)
	}
	altEsc := html.EscapeString(label)
	urlEsc := html.EscapeString(cleanURL)
	ext := urlExt(cleanURL)
	style := mediaSizeStyle(size, true)
	if strings.HasPrefix(mimeType, "image/") || isImageExt(ext) {
		return `<img class="md-img" alt="` + altEsc + `" src="` + urlEsc + `"` + style + `>`
	}
	style = mediaSizeStyle(size, false)
	if strings.HasPrefix(mimeType, "audio/") || isAudioExt(ext) {
		return `<audio class="md-media" controls preload="metadata" src="` + urlEsc + `"` + style + `>` + altEsc + `</audio>`
	}
	if strings.HasPrefix(mimeType, "video/") || isVideoExt(ext) {
		return `<video class="md-media" controls preload="metadata" src="` + urlEsc + `"` + style + `>` + altEsc + `</video>`
	}
	return `<a href="` + urlEsc + `" target="_blank" rel="noopener noreferrer">` + altEsc + `</a>`
}

func renderMarkdownLink(label, rawURL string) string {
	label, size := splitMediaLabel(label)
	cleanURL, ok := cleanMediaURL(rawURL)
	if !ok {
		return html.EscapeString(label)
	}
	if embedURL, ok := youtubeEmbedURL(cleanURL); ok {
		return renderYouTubeEmbed(label, embedURL, size)
	}
	ext := urlExt(cleanURL)
	if isImageExt(ext) || isAudioExt(ext) || isVideoExt(ext) {
		return renderEmbeddedFileSized(label, cleanURL, "", size)
	}
	return `<a href="` + html.EscapeString(cleanURL) + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(label) + `</a>`
}

func renderAutoMediaURL(rawURL string) string {
	cleanURL, ok := cleanMediaURL(rawURL)
	if !ok {
		return html.EscapeString(rawURL)
	}
	if embedURL, ok := youtubeEmbedURL(cleanURL); ok {
		return renderYouTubeEmbed("YouTube video", embedURL, "")
	}
	ext := urlExt(cleanURL)
	if isImageExt(ext) || isAudioExt(ext) || isVideoExt(ext) {
		return renderEmbeddedFile(filepath.Base(urlPath(cleanURL)), cleanURL, "")
	}
	return `<a href="` + html.EscapeString(cleanURL) + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(cleanURL) + `</a>`
}

func renderYouTubeEmbed(label, embedURL, size string) string {
	style := mediaSizeStyle(size, false)
	return `<div class="md-embed"` + style + `><iframe src="` + html.EscapeString(embedURL) + `" title="` + html.EscapeString(label) + `" loading="lazy" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe></div>`
}

func markdownLabel(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "]", "-", "[", "-", "(", "-", ")", "-").Replace(strings.TrimSpace(s))
	if s == "" {
		return "file"
	}
	return s
}

func splitMediaLabel(label string) (string, string) {
	parts := strings.Split(label, "|")
	if len(parts) < 2 {
		return strings.TrimSpace(label), ""
	}
	size := strings.TrimSpace(parts[len(parts)-1])
	if mediaSizeStyle(size, false) == "" {
		return strings.TrimSpace(label), ""
	}
	return strings.TrimSpace(strings.Join(parts[:len(parts)-1], "|")), size
}

func mediaSizeStyle(spec string, image bool) string {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" {
		return ""
	}
	spec = strings.ReplaceAll(spec, "×", "x")
	var width, height string
	if strings.Contains(spec, "x") {
		parts := strings.SplitN(spec, "x", 2)
		width = mediaDimension(parts[0])
		height = mediaDimension(parts[1])
	} else {
		width = mediaDimension(spec)
	}
	styles := []string{}
	if width != "" {
		styles = append(styles, "width:"+width)
	}
	if height != "" {
		styles = append(styles, "height:"+height)
		if image {
			styles = append(styles, "object-fit:contain")
		}
	}
	if len(styles) == 0 {
		return ""
	}
	return ` style="` + strings.Join(styles, ";") + `"`
}

func mediaDimension(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "auto" {
		return ""
	}
	if regexp.MustCompile(`^\d{1,4}$`).MatchString(v) {
		return v + "px"
	}
	if regexp.MustCompile(`^\d{1,4}(\.\d{1,2})?(px|%|rem|em|vw|vh)$`).MatchString(v) {
		return v
	}
	return ""
}

func cleanMediaURL(raw string) (string, bool) {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	raw = strings.Trim(raw, "<>")
	if strings.HasPrefix(raw, "/files/") {
		return raw, true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func urlPath(raw string) string {
	if strings.HasPrefix(raw, "/") {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			return raw[:i]
		}
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func urlExt(raw string) string {
	return strings.ToLower(filepath.Ext(urlPath(raw)))
}

func youtubeEmbedURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(html.UnescapeString(raw)))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch {
	case host == "youtu.be":
		id = firstPathSegment(u.Path)
	case host == "youtube.com" || host == "youtube-nocookie.com":
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case strings.HasPrefix(u.Path, "/embed/"):
			id = firstPathSegment(strings.TrimPrefix(u.Path, "/embed/"))
		case strings.HasPrefix(u.Path, "/shorts/"):
			id = firstPathSegment(strings.TrimPrefix(u.Path, "/shorts/"))
		case strings.HasPrefix(u.Path, "/live/"):
			id = firstPathSegment(strings.TrimPrefix(u.Path, "/live/"))
		}
	}
	if !validYouTubeID(id) {
		return "", false
	}
	return "https://www.youtube-nocookie.com/embed/" + id, true
}

func firstPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

func validYouTubeID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func trimAutoURLTail(raw string) (string, string) {
	tail := ""
	for raw != "" {
		last := raw[len(raw)-1]
		if strings.ContainsRune(".,;:!?", rune(last)) {
			tail = string(last) + tail
			raw = raw[:len(raw)-1]
			continue
		}
		break
	}
	return raw, tail
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return true
	default:
		return false
	}
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".m4a":
		return true
	default:
		return false
	}
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov":
		return true
	default:
		return false
	}
}

func escapeHeaderFilename(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(safeFileName(s), "\\", "-"), "\"", "-")
}

func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func CanRead(u *User, a *Article, groups map[string]*Group) bool {
	if u == nil || a == nil || !u.Active {
		return false
	}
	if u.Role == RoleAdmin {
		return true
	}
	if a.AllUsers {
		return true
	}
	if a.OwnerID == u.ID {
		return true
	}
	if contains(a.AllowedUserIDs, u.ID) {
		return true
	}
	if groups != nil {
		for _, gid := range a.AllowedGroupIDs {
			if g := groups[gid]; g != nil && contains(g.MemberIDs, u.ID) {
				return true
			}
		}
	}
	return false
}
func checked(ok bool) string {
	if ok {
		return "checked"
	}
	return ""
}
func contains(in []string, x string) bool {
	for _, v := range in {
		if v == x {
			return true
		}
	}
	return false
}
func set(in []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range in {
		m[v] = true
	}
	return m
}
func removeString(in []string, x string) []string {
	out := in[:0]
	for _, v := range in {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}
func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func parseTags(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) })
	out := []string{}
	seen := map[string]bool{}
	for _, p := range parts {
		p = sanitizeSlug(strings.TrimPrefix(strings.TrimSpace(p), "#"))
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	last := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			last = false
			continue
		}
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			if !last {
				b.WriteByte('-')
				last = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func HashPassword(password string) (string, string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	key := pbkdf2Key([]byte(password), salt, PBKDF2Rounds, 32)
	return hex.EncodeToString(salt), hex.EncodeToString(key), nil
}
func VerifyPassword(password, saltHex, hashHex string) bool {
	salt, err1 := hex.DecodeString(saltHex)
	want, err2 := hex.DecodeString(hashHex)
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2Key([]byte(password), salt, PBKDF2Rounds, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	hLen := 32
	blocks := (keyLen + hLen - 1) / hLen
	out := []byte{}
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var ibuf [4]byte
		binary.BigEndian.PutUint32(ibuf[:], uint32(block))
		mac.Write(ibuf[:])
		u := mac.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for x := range t {
				t[x] ^= u[x]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func sha256Hex(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func constantEqualHex(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var wikiRE = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
var tagRE = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_-]+)`)
var boldRE = regexp.MustCompile(`\*\*([^*]+)\*\*`)
var italicRE = regexp.MustCompile(`\*([^*]+)\*`)
var codeRE = regexp.MustCompile("`([^`]+)`")
var imageRE = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
var mdLinkRE = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+|/files/[^\s)]+)\)`)
var autoURLRE = regexp.MustCompile(`(^|[\s(])((?:https?://|/files/)[^\s<]+)`)

func RenderMarkdown(src string) template.HTML {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")
	var b strings.Builder
	inCode := false
	inList := false
	tableRows := [][]string{}
	flushTable := func() {
		if len(tableRows) == 0 {
			return
		}
		b.WriteString(`<div class="table-wrap"><table>`)
		for _, row := range tableRows {
			b.WriteString(`<tr>`)
			for _, cell := range row {
				b.WriteString(`<td>` + inlineMarkdown(strings.TrimSpace(cell)) + `</td>`)
			}
			b.WriteString(`</tr>`)
		}
		b.WriteString(`</table></div>`)
		tableRows = nil
	}
	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			closeList()
			if !inCode {
				b.WriteString("<pre><code>")
				inCode = true
			} else {
				b.WriteString("</code></pre>")
				inCode = false
			}
			continue
		}
		if inCode {
			b.WriteString(html.EscapeString(line) + "\n")
			continue
		}
		if strings.HasPrefix(strings.ToLower(trim), "<table") {
			flushTable()
			closeList()
			raw := []string{line}
			for !strings.Contains(strings.ToLower(trim), "</table>") && i+1 < len(lines) {
				i++
				trim = strings.TrimSpace(lines[i])
				raw = append(raw, lines[i])
			}
			b.WriteString(sanitizeTableHTML(strings.Join(raw, "\n")))
			continue
		}
		if trim == "" {
			flushTable()
			closeList()
			continue
		}
		if strings.HasPrefix(trim, "|") && strings.HasSuffix(trim, "|") {
			rowRaw := strings.Trim(trim, "|")
			cells := strings.Split(rowRaw, "|")
			isSep := true
			for _, c := range cells {
				c = strings.TrimSpace(c)
				if len(c) < 3 || strings.Trim(c, ":-") != "" {
					isSep = false
				}
			}
			if !isSep {
				closeList()
				tableRows = append(tableRows, cells)
			}
			continue
		}
		flushTable()
		if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + inlineMarkdown(strings.TrimSpace(trim[2:])) + "</li>")
			continue
		}
		closeList()
		flushTable()
		if strings.HasPrefix(trim, ">") {
			b.WriteString("<blockquote>" + inlineMarkdown(strings.TrimSpace(strings.TrimPrefix(trim, ">"))) + "</blockquote>")
			continue
		}
		lvl := 0
		for lvl < len(trim) && lvl < 6 && trim[lvl] == '#' {
			lvl++
		}
		if lvl > 0 && len(trim) > lvl && trim[lvl] == ' ' {
			text := strings.TrimSpace(trim[lvl+1:])
			b.WriteString(fmt.Sprintf("<h%d>%s</h%d>", lvl, inlineMarkdown(text), lvl))
			continue
		}
		b.WriteString("<p>" + inlineMarkdown(trim) + "</p>")
	}
	flushTable()
	closeList()
	if inCode {
		b.WriteString("</code></pre>")
	}
	return template.HTML(b.String())
}
func inlineMarkdown(s string) string {
	esc := html.EscapeString(s)
	stashed := []string{}
	stash := func(rendered string) string {
		token := fmt.Sprintf("\x00MVHTML%d\x00", len(stashed))
		stashed = append(stashed, rendered)
		return token
	}
	esc = codeRE.ReplaceAllStringFunc(esc, func(m string) string {
		p := codeRE.FindStringSubmatch(m)
		if len(p) != 2 {
			return m
		}
		return stash(`<code>` + p[1] + `</code>`)
	})
	esc = boldRE.ReplaceAllString(esc, `<strong>$1</strong>`)
	esc = italicRE.ReplaceAllString(esc, `<em>$1</em>`)
	esc = imageRE.ReplaceAllStringFunc(esc, func(m string) string {
		p := imageRE.FindStringSubmatch(m)
		if len(p) != 3 {
			return m
		}
		return stash(renderEmbeddedFile(html.UnescapeString(p[1]), html.UnescapeString(p[2]), ""))
	})
	esc = mdLinkRE.ReplaceAllStringFunc(esc, func(m string) string {
		p := mdLinkRE.FindStringSubmatch(m)
		if len(p) != 3 {
			return m
		}
		return stash(renderMarkdownLink(html.UnescapeString(p[1]), html.UnescapeString(p[2])))
	})
	esc = autoURLRE.ReplaceAllStringFunc(esc, func(m string) string {
		p := autoURLRE.FindStringSubmatch(m)
		if len(p) != 3 {
			return m
		}
		cleanURL, tail := trimAutoURLTail(p[2])
		return p[1] + stash(renderAutoMediaURL(html.UnescapeString(cleanURL))) + tail
	})
	esc = wikiRE.ReplaceAllStringFunc(esc, func(m string) string {
		p := wikiRE.FindStringSubmatch(m)
		if len(p) < 2 {
			return m
		}
		slug := sanitizeSlug(p[1])
		label := p[1]
		if len(p) >= 3 && strings.TrimSpace(p[2]) != "" {
			label = p[2]
		}
		if slug == "" {
			return m
		}
		return `<a href="/a/` + html.EscapeString(slug) + `">` + html.EscapeString(label) + `</a>`
	})
	esc = tagRE.ReplaceAllString(esc, `${1}<a class="tag" href="/?tag=$2">#$2</a>`)
	for i, rendered := range stashed {
		esc = strings.ReplaceAll(esc, fmt.Sprintf("\x00MVHTML%d\x00", i), rendered)
	}
	return esc
}

func sanitizeTableHTML(raw string) string {
	raw = regexp.MustCompile(`(?is)<!--.*?-->`).ReplaceAllString(raw, "")
	for _, tag := range []string{"script", "style", "iframe", "object", "embed"} {
		raw = regexp.MustCompile(`(?is)<\s*`+tag+`[^>]*>.*?<\s*/\s*`+tag+`\s*>`).ReplaceAllString(raw, "")
	}
	var b strings.Builder
	for i := 0; i < len(raw); {
		open := strings.IndexByte(raw[i:], '<')
		if open < 0 {
			b.WriteString(html.EscapeString(raw[i:]))
			break
		}
		open += i
		b.WriteString(html.EscapeString(raw[i:open]))
		close := strings.IndexByte(raw[open:], '>')
		if close < 0 {
			b.WriteString(html.EscapeString(raw[open:]))
			break
		}
		close += open
		b.WriteString(sanitizeTableTag(raw[open : close+1]))
		i = close + 1
	}
	return `<div class="table-wrap">` + b.String() + `</div>`
}

func sanitizeTableTag(tag string) string {
	m := regexp.MustCompile(`(?is)^<\s*(/)?\s*([a-z0-9]+)([^>]*)>$`).FindStringSubmatch(tag)
	if len(m) != 4 {
		return html.EscapeString(tag)
	}
	name := strings.ToLower(m[2])
	allowed := map[string]bool{"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true, "th": true, "br": true}
	if !allowed[name] {
		return html.EscapeString(tag)
	}
	if m[1] == "/" {
		if name == "br" {
			return ""
		}
		return "</" + name + ">"
	}
	if name == "br" {
		return "<br>"
	}
	attrs := ""
	if name == "td" || name == "th" {
		attrRE := regexp.MustCompile(`(?i)([a-z0-9_-]+)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
		for _, a := range attrRE.FindAllStringSubmatch(m[3], -1) {
			key := strings.ToLower(a[1])
			val := strings.Trim(a[2], `"'`)
			switch key {
			case "colspan", "rowspan":
				n, err := strconv.Atoi(val)
				if err == nil && n > 1 && n <= 20 {
					attrs += " " + key + `="` + strconv.Itoa(n) + `"`
				}
			case "align":
				val = strings.ToLower(val)
				if val == "left" || val == "center" || val == "right" {
					attrs += ` align="` + val + `"`
				}
			}
		}
	}
	return "<" + name + attrs + ">"
}
func articleLinksTo(content, slug string) bool {
	found := false
	wikiRE.ReplaceAllStringFunc(content, func(m string) string {
		p := wikiRE.FindStringSubmatch(m)
		if len(p) >= 2 && sanitizeSlug(p[1]) == slug {
			found = true
		}
		return m
	})
	return found
}

func editorScript() string {
	return `<script>
const md=document.getElementById('md');const host=document.getElementById('toastEditor');const shell=document.querySelector('.live-editor-shell');const previewStatus=document.getElementById('previewStatus');const csrf=document.querySelector('#editorForm input[name="csrf"]')?.value||'';let articleId=shell?.dataset.articleId||'',toastEditor=null;
function esc(s){return String(s||'').replace(/[&<>"']/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));}
function slug(s){return s.toLowerCase().trim().replace(/[^\p{L}\p{N}]+/gu,'-').replace(/^-+|-+$/g,'');}
function mdLabel(s){s=String(s||'file').replace(/[\r\n\[\]()]/g,' ').trim();return s||'file'}
function mdURL(s){return String(s||'').trim().replace(/[)\s]+/g,'')}
function mediaMarkdown(data){const mime=data.mime||'',url=mdURL(data.url||''),name=mdLabel(data.name||'file');if(mime.startsWith('image/'))return '!['+name+'|100%]('+url+')';if(mime.startsWith('video/'))return '!['+name+'|760]('+url+')';if(mime.startsWith('audio/'))return '!['+name+']('+url+')';return data.markdown||('['+name+']('+url+')')}
function setPreviewStatus(text,state){if(previewStatus){previewStatus.textContent=text;previewStatus.dataset.state=state||''}}
function syncMarkdown(){if(md&&toastEditor){md.value=toastEditor.getMarkdown()}}
function loadStyle(href){if(document.querySelector('link[href="'+href+'"]'))return;const link=document.createElement('link');link.rel='stylesheet';link.href=href;document.head.appendChild(link)}
function loadScript(src){return new Promise((resolve,reject)=>{if(document.querySelector('script[src="'+src+'"]')){resolve();return}const s=document.createElement('script');s.src=src;s.onload=resolve;s.onerror=reject;document.head.appendChild(s)})}
function normalizeInsert(markdown){markdown=String(markdown||'');return /\n$/.test(markdown)?markdown:markdown+'\n'}
function insertMarkdown(markdown){if(!toastEditor)return;markdown=normalizeInsert(markdown);toastEditor.focus();try{const sel=toastEditor.getSelection&&toastEditor.getSelection();if(typeof toastEditor.replaceSelection==='function'){toastEditor.replaceSelection(markdown,sel&&sel[0],sel&&sel[1])}else if(typeof toastEditor.insertText==='function'){toastEditor.insertText(markdown)}else{const current=toastEditor.getMarkdown();toastEditor.setMarkdown(current+(current.endsWith('\n')?'':'\n')+markdown,false)}}catch(e){const current=toastEditor.getMarkdown();toastEditor.setMarkdown(current+(current.endsWith('\n')?'':'\n')+markdown,false)}syncMarkdown();setPreviewStatus('изменено','ok')}
function insertUploadedMedia(data){if(!toastEditor||!data)return;toastEditor.focus();insertMarkdown(mediaMarkdown(data))}
function insertWikiLink(){const name=prompt('Статья для wiki-ссылки','');if(!name)return;const s=slug(name);insertMarkdown('[['+s+'|'+name+']]')}
function insertVideoLink(){const raw=prompt('YouTube или video URL','');const url=mdURL(raw);if(!url)return;const title=mdLabel(prompt('Подпись','Видео')||'Видео');insertMarkdown('['+title+']('+url+')')}
function htmlForAttachment(x){if(x.html)return x.html;const name=esc(x.name||'file'),url=esc(x.url||'#'),m=x.mime||'';if(m.startsWith('image/'))return '<img class="md-img" alt="'+name+'" src="'+url+'">';if(m.startsWith('audio/'))return '<audio class="md-media" controls src="'+url+'"></audio>';if(m.startsWith('video/'))return '<video class="md-media" controls src="'+url+'"></video>';return '<a href="'+url+'">'+name+'</a>'}
async function ensureArticle(){if(articleId)return articleId;setPreviewStatus('черновик','loading');const res=await fetch('/admin/draft',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded; charset=UTF-8'},body:new URLSearchParams({csrf:csrf})});if(!res.ok)throw new Error('draft '+res.status);const data=await res.json();articleId=data.id;shell.dataset.articleId=articleId;const idInput=document.querySelector('#editorForm input[name="id"]');if(idInput)idInput.value=articleId;if(data.edit_url)history.replaceState(null,'',data.edit_url);return articleId}
async function uploadOne(file){await ensureArticle();const fd=new FormData();fd.append('csrf',csrf);fd.append('article_id',articleId);fd.append('file',file);const res=await fetch('/attachments/drop',{method:'POST',body:fd});if(!res.ok)throw new Error('upload '+res.status);return await res.json()}
function allImages(files){return Array.from(files||[]).every(f=>String(f.type||'').startsWith('image/'))}
async function uploadFiles(files){if(!files||!files.length)return;try{setPreviewStatus('загрузка','loading');for(const file of files){const data=await uploadOne(file);insertUploadedMedia(data)}setPreviewStatus('live','ok')}catch(e){setPreviewStatus('ошибка загрузки','fallback');console.warn(e)}}
function bindDropAndPaste(){const root=host.querySelector('.toastui-editor-defaultUI')||host;root.addEventListener('dragover',e=>{if(e.dataTransfer?.files?.length){shell.classList.add('dragging');if(!allImages(e.dataTransfer.files))e.preventDefault()}},true);root.addEventListener('dragleave',()=>shell.classList.remove('dragging'),true);root.addEventListener('drop',e=>{const files=e.dataTransfer?.files;if(files&&files.length&&!allImages(files)){e.preventDefault();e.stopPropagation();shell.classList.remove('dragging');uploadFiles(files)}},true);root.addEventListener('paste',e=>{const files=e.clipboardData?.files;if(files&&files.length&&!allImages(files)){e.preventDefault();e.stopPropagation();uploadFiles(files)}},true)}
async function initToastEditor(){if(!host||!md)return;loadStyle('https://uicdn.toast.com/editor/latest/toastui-editor.min.css');loadStyle('https://uicdn.toast.com/editor/latest/theme/toastui-editor-dark.min.css');loadStyle('https://uicdn.toast.com/editor-plugin-table-merged-cell/latest/toastui-editor-plugin-table-merged-cell.min.css');try{await loadScript('https://uicdn.toast.com/editor/latest/toastui-editor-all.min.js');await loadScript('https://uicdn.toast.com/editor-plugin-table-merged-cell/latest/toastui-editor-plugin-table-merged-cell.min.js')}catch(e){host.innerHTML='<textarea id="fallbackEditor"></textarea>';const fb=document.getElementById('fallbackEditor');fb.value=md.value;fb.addEventListener('input',()=>{md.value=fb.value});setPreviewStatus('offline','fallback');return}const Editor=toastui.Editor;const tableMergedCell=Editor.plugin&&Editor.plugin.tableMergedCell;toastEditor=new Editor({el:host,height:'calc(100vh - 310px)',initialValue:md.value||'',initialEditType:'wysiwyg',previewStyle:'vertical',hideModeSwitch:false,theme:'dark',usageStatistics:false,plugins:tableMergedCell?[tableMergedCell]:[],toolbarItems:[['heading','bold','italic','strike'],['hr','quote'],['ul','ol','task'],['table','image','link'],['code','codeblock']],hooks:{addImageBlobHook:async(blob)=>{try{setPreviewStatus('загрузка','loading');const data=await uploadOne(blob);insertUploadedMedia(data);setPreviewStatus('вставлено','ok')}catch(e){setPreviewStatus('ошибка загрузки','fallback')}return false}}});toastEditor.on('change',()=>{syncMarkdown();setPreviewStatus('live','ok')});bindDropAndPaste();syncMarkdown();setPreviewStatus(tableMergedCell?'live + tables':'live','ok')}
if(host){initToastEditor();document.getElementById('editorForm')?.addEventListener('submit',syncMarkdown);document.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='s'){e.preventDefault();syncMarkdown();document.getElementById('editorForm').submit()}})}
</script>`
}

func (app *App) render(w http.ResponseWriter, r *http.Request, title string, u *User, s *Session, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	csrf := ""
	if s != nil {
		csrf = s.CSRFToken
	}
	data := LayoutData{Title: title, App: AppName, User: u, CSRF: csrf, Body: body, Ribbon: app.ribbonArticles(u, r)}
	if err := layoutTmpl.Execute(w, data); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (app *App) ribbonArticles(u *User, r *http.Request) []RibbonArticle {
	if u == nil {
		return nil
	}
	app.store.mu.RLock()
	defer app.store.mu.RUnlock()
	out := []RibbonArticle{}
	activeSlug := ""
	if strings.HasPrefix(r.URL.Path, "/a/") {
		activeSlug = sanitizeSlug(strings.TrimPrefix(r.URL.Path, "/a/"))
	}
	for _, id := range app.store.db.RibbonArticleIDs {
		a := app.store.db.Articles[id]
		if a == nil || a.Archived || !CanRead(u, a, app.store.db.Groups) {
			continue
		}
		out = append(out, RibbonArticle{Title: a.Title, Slug: a.Slug, Active: a.Slug == activeSlug})
	}
	return out
}

var layoutTmpl = template.Must(template.New("layout").Parse(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · {{.App}}</title><style>` + css() + `</style></head><body><div class="bg"></div><header><a class="brand" href="/"><span>DH</span>{{.App}}</a>{{if .User}}<nav><span class="user-pill">{{.User.Username}} · {{.User.Role}}</span>{{if eq .User.Role "admin"}}<a href="/edit/new">Новая</a><a href="/admin/import">Импорт</a><a href="/admin/ribbon">Лента</a><a href="/admin/users">Пользователи</a><a href="/admin/groups">Группы</a><a href="/admin/backups">Backups</a>{{end}}<a href="/logout">Выйти</a></nav>{{end}}</header>{{if .User}}<aside class="vault-ribbon"><a class="ribbon-home" href="/">Все</a>{{range .Ribbon}}<a class="ribbon-note{{if .Active}} active{{end}}" href="/a/{{.Slug}}" data-note-preview="/preview/article/{{.Slug}}" title="{{.Title}}"><span>{{.Title}}</span></a>{{end}}{{if eq .User.Role "admin"}}<a class="ribbon-settings" href="/admin/ribbon">Настроить</a>{{end}}</aside>{{end}}<main>{{.Body}}</main><div id="pagePreviewPopup" class="page-preview-popup" hidden></div><script>` + pagePreviewScript() + `</script></body></html>`))

func pagePreviewScript() string {
	return `
(()=>{const popup=document.getElementById('pagePreviewPopup');if(!popup)return;let timer=null,hideTimer=null,active=null;
function previewURL(el){if(el.dataset&&el.dataset.notePreview)return el.dataset.notePreview;const href=el.getAttribute&&el.getAttribute('href');return href&&href.startsWith('/a/')?'/preview/article/'+encodeURIComponent(href.slice(3)):''}
function place(e){const pad=14;let x=e.clientX+18,y=e.clientY+18;popup.style.left=x+'px';popup.style.top=y+'px';const r=popup.getBoundingClientRect();if(r.right>innerWidth-pad)popup.style.left=Math.max(pad,e.clientX-r.width-18)+'px';if(r.bottom>innerHeight-pad)popup.style.top=Math.max(pad,e.clientY-r.height-18)+'px'}
async function show(el,e){const url=previewURL(el);if(!url)return;active=url;place(e);popup.hidden=false;popup.innerHTML='<p class="muted">Загрузка...</p>';try{const res=await fetch(url);if(active!==url)return;if(!res.ok)throw new Error('HTTP '+res.status);popup.innerHTML=await res.text()}catch(err){popup.hidden=true}}
document.addEventListener('mouseover',e=>{const a=e.target.closest('a[href],a[data-note-preview]');if(!a)return;const url=previewURL(a);if(!url)return;clearTimeout(hideTimer);timer=setTimeout(()=>show(a,e),260)});
document.addEventListener('mousemove',e=>{if(!popup.hidden)place(e)});
document.addEventListener('mouseout',e=>{const a=e.target.closest('a[href],a[data-note-preview]');if(!a)return;clearTimeout(timer);hideTimer=setTimeout(()=>{popup.hidden=true;active=null},180)});
})();`
}

func css() string {
	return `
:root{
	--bg:#0c0d0f;
	--surface:#131619;
	--surface-2:#191d20;
	--surface-3:#1f2529;
	--text:#edf0ee;
	--muted:#9aa39e;
	--line:#2d3438;
	--line-strong:#3a4449;
	--accent:#38d6ad;
	--accent-soft:rgba(56,214,173,.13);
	--info:#5eb8ff;
	--warning:#e0b45f;
	--danger:#ff6673;
	--ok:#5fd48a;
	--shadow:0 18px 44px rgba(0,0,0,.28);
}
*{box-sizing:border-box;border-radius:0!important;letter-spacing:0}
html{color-scheme:dark}
body{
	margin:0;
	background:var(--bg);
	color:var(--text);
	font:15px/1.55 Inter,ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial,sans-serif;
}
.bg{display:none}
a{color:var(--accent);text-decoration:none}
a:hover{color:#78e8cc}
header{
	position:sticky;
	top:0;
	z-index:10;
	display:flex;
	gap:18px;
	align-items:center;
	justify-content:space-between;
	padding:12px 24px;
	background:#101316;
	border-bottom:1px solid var(--line);
	box-shadow:0 1px 0 rgba(255,255,255,.02);
}
.brand{
	display:flex;
	gap:11px;
	align-items:center;
	color:var(--text);
	font-weight:760;
	text-transform:uppercase;
	font-size:14px;
}
.brand span,.logo-mark{
	display:inline-grid;
	place-items:center;
	width:34px;
	height:34px;
	background:var(--accent);
	color:#07100d;
	font-weight:850;
}
nav{
	display:flex;
	gap:6px;
	align-items:center;
	flex-wrap:wrap;
	color:var(--muted);
	font-size:13px;
}
nav a,.user-pill{
	border:1px solid transparent;
	padding:8px 10px;
	color:var(--muted);
	background:transparent;
}
nav a:hover,.user-pill{border-color:var(--line);background:var(--surface)}
.vault-ribbon{
	position:fixed;
	left:0;
	top:59px;
	bottom:0;
	z-index:8;
	width:230px;
	padding:12px;
	background:#101316;
	border-right:1px solid var(--line);
	overflow:auto;
}
.vault-ribbon a{
	display:block;
	color:var(--muted);
	border:1px solid transparent;
	padding:9px 10px;
	margin-bottom:6px;
	font-size:13px;
}
.vault-ribbon a:hover,.vault-ribbon a.active{background:var(--surface);border-color:var(--line);color:var(--text)}
.ribbon-home,.ribbon-settings{font-weight:750;text-transform:uppercase}
.ribbon-settings{margin-top:12px;color:var(--accent)!important}
main{position:relative;max-width:1220px;margin:0 auto;padding:24px}
.vault-ribbon+main{max-width:none;margin-left:230px}
.card,.article-card,.login-card{
	background:var(--surface);
	border:1px solid var(--line);
	padding:20px;
	box-shadow:var(--shadow);
}
.login-shell{min-height:calc(100vh - 96px);display:grid;place-items:center}
.login-card{width:min(420px,100%)}
h1,h2,h3{font-weight:760}
h1{font-size:clamp(28px,4vw,44px);line-height:1.05;margin:0 0 12px}
h2{font-size:22px;line-height:1.15;margin:0 0 10px}
h3{font-size:15px;text-transform:uppercase;color:var(--muted);margin:22px 0 10px}
p{margin:0 0 12px}
.muted{color:var(--muted);font-size:13px}
.hero{
	display:grid;
	grid-template-columns:1fr auto;
	gap:20px;
	align-items:end;
	margin-bottom:18px;
	padding:24px;
	background:var(--surface);
	border:1px solid var(--line);
	box-shadow:var(--shadow);
}
.hero p{color:var(--muted);font-size:16px;margin:0;max-width:760px}
.hero-stat{
	min-width:160px;
	text-align:left;
	background:var(--surface-2);
	border-left:3px solid var(--accent);
	padding:16px;
}
.hero-stat b{display:block;font-size:38px;line-height:1;color:var(--accent)}
.hero-stat span{color:var(--muted);font-size:13px}
.toolbar{display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap}
.search{display:flex;gap:8px;flex:1;min-width:260px}
input,textarea,select{
	width:100%;
	border:1px solid var(--line);
	background:#0f1214;
	color:var(--text);
	padding:11px 12px;
	font:inherit;
	outline:none;
}
input::placeholder,textarea::placeholder{color:#6f7a74}
input:focus,textarea:focus,select:focus{
	border-color:var(--accent);
	box-shadow:inset 3px 0 0 var(--accent);
}
textarea{
	min-height:calc(100vh - 310px);
	resize:vertical;
	font:14px/1.62 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
}
label{display:block;margin:9px 0;color:#cbd2ce;font-size:13px}
button,.button{
	display:inline-flex;
	align-items:center;
	justify-content:center;
	border:1px solid var(--line-strong);
	background:var(--surface-2);
	color:var(--text);
	padding:10px 14px;
	font-weight:700;
	cursor:pointer;
	font:inherit;
	white-space:nowrap;
	min-height:42px;
}
button:hover,.button:hover{background:var(--surface-3);color:var(--text);border-color:#4a555b}
.primary{
	background:var(--accent);
	border-color:var(--accent);
	color:#07100d;
}
.primary:hover{background:#62e2c1;color:#07100d}
.secondary{background:var(--surface-2);color:var(--text)}
.danger{background:transparent;border-color:rgba(255,102,115,.6);color:#ff9aa3}
.danger:hover{background:rgba(255,102,115,.12);color:#ffc4ca}
.ok,.error{
	padding:11px 12px;
	border:1px solid;
	margin:12px 0;
}
.ok{background:rgba(95,212,138,.1);border-color:rgba(95,212,138,.45);color:#a6efc1}
.error{background:rgba(255,102,115,.1);border-color:rgba(255,102,115,.45);color:#ffc4ca}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(270px,1fr));gap:12px}
.article-card{
	display:flex;
	min-height:158px;
	color:var(--text);
	flex-direction:column;
	justify-content:space-between;
	transition:.16s background,.16s border-color,.16s box-shadow;
}
.article-card:hover{
	background:var(--surface-2);
	border-color:var(--accent);
	box-shadow:0 0 0 1px rgba(56,214,173,.2),var(--shadow);
	color:var(--text);
}
.badge,.chip{
	display:inline-block;
	background:var(--accent-soft);
	border:1px solid rgba(56,214,173,.42);
	padding:4px 8px;
	font-size:12px;
	color:#8af0d4;
	text-transform:uppercase;
}
.badge.private{
	background:rgba(224,180,95,.11);
	border-color:rgba(224,180,95,.46);
	color:#f0d092;
}
.tagbar,.tags{display:flex;gap:7px;flex-wrap:wrap}
.tagbar{margin:0 0 16px}
.tags span,.tags a,.tag{
	color:#8af0d4;
	background:var(--accent-soft);
	border:1px solid rgba(56,214,173,.3);
	padding:2px 7px;
	font-size:12px;
}
.article-layout{display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:16px}
.article-top{display:flex;justify-content:space-between;gap:16px;align-items:flex-start}
.side{height:max-content;position:sticky;top:76px}
.side-link{
	display:block;
	padding:10px;
	background:#0f1214;
	border:1px solid var(--line);
	margin:8px 0;
	color:var(--text);
}
.side-link:hover{border-color:var(--accent);background:var(--surface-2);color:var(--text)}
.markdown{font-size:16px}
.markdown h1,.markdown h2,.markdown h3{line-height:1.15}
.markdown h1{font-size:32px}
.markdown h2{font-size:24px}
.markdown pre{
	overflow:auto;
	background:#0a0c0e;
	border:1px solid var(--line);
	padding:14px;
}
.markdown code{
	background:#0a0c0e;
	border:1px solid var(--line);
	padding:1px 5px;
}
.markdown blockquote{
	border-left:3px solid var(--accent);
	margin:16px 0;
	padding:10px 14px;
	color:#c9d0cc;
	background:#101316;
}
.editor-meta{
	display:grid;
	grid-template-columns:1fr 240px 240px;
	gap:10px;
	margin-bottom:12px;
}
.editor-meta details{grid-column:span 3}
summary{cursor:pointer;color:var(--text);font-weight:700;margin:8px 0}
.actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}
.check{
	display:inline-flex;
	gap:8px;
	align-items:center;
	margin:6px 14px 6px 0;
	color:#cbd2ce;
}
.check input{width:auto}
input[type=checkbox]{accent-color:var(--accent)}
.editor-toolbar{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px}
.editor-status{
	margin-left:auto;
	align-self:center;
	color:var(--accent);
	font-size:12px;
	font-weight:760;
	text-transform:uppercase;
}
.editor-grid{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:12px}
.live-editor-shell{
	position:relative;
	min-height:calc(100vh - 310px);
	padding:0;
	overflow:hidden;
}
.toast-editor-host{
	min-height:calc(100vh - 310px);
	background:var(--surface);
}
.toastui-editor-defaultUI{
	border:0!important;
	background:var(--surface)!important;
}
.toastui-editor-dark .toastui-editor-toolbar,.toastui-editor-defaultUI-toolbar{
	border-color:var(--line)!important;
}
.toastui-editor-contents{
	font:16px/1.62 Inter,ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial,sans-serif!important;
}
.live-editor{
	min-height:calc(100vh - 310px);
	padding:28px;
	outline:none;
	background:var(--surface);
}
.live-editor:focus{box-shadow:inset 3px 0 0 var(--accent)}
.live-editor:empty:before{
	content:"Пишите заметку, перетащите изображение или вставьте файл из буфера";
	color:var(--muted);
}
.live-editor-shell.dragging{border-color:var(--accent)}
.drop-hint{
	position:absolute;
	inset:0;
	display:none;
	place-items:center;
	background:rgba(12,13,15,.82);
	color:var(--accent);
	font-weight:760;
	pointer-events:none;
}
.live-editor-shell.dragging .drop-hint{display:grid}
.cmhost{
	background:#0f1214;
	border:1px solid var(--line);
	overflow:hidden;
}
.hidden-textarea{position:absolute;left:-10000px;width:1px;height:1px;opacity:0}
.preview{
	background:var(--surface);
	border:1px solid var(--line);
	min-height:calc(100vh - 310px);
	overflow:auto;
}
.preview-head{
	position:sticky;
	top:0;
	z-index:1;
	display:flex;
	align-items:center;
	justify-content:space-between;
	gap:12px;
	padding:10px 12px;
	background:#101316;
	border-bottom:1px solid var(--line);
	color:var(--muted);
	font-size:12px;
	font-weight:700;
	text-transform:uppercase;
}
#previewStatus{
	color:var(--muted);
	font-size:11px;
}
#previewStatus[data-state=ok]{color:var(--accent)}
#previewStatus[data-state=loading]{color:var(--warning)}
#previewStatus[data-state=fallback]{color:var(--info)}
.preview-body{padding:20px}
.markdown .table-wrap{overflow:auto;border:1px solid var(--line);margin:16px 0}
.markdown .table-wrap table{margin:0}
.markdown .md-img{
	max-width:100%;
	border:1px solid var(--line);
	display:block;
	margin:14px 0;
	height:auto;
}
.markdown .md-media{
	display:block;
	width:min(100%,760px);
	max-width:100%;
	margin:14px 0;
	background:#0a0c0e;
	border:1px solid var(--line);
}
.markdown .md-embed{
	width:min(100%,760px);
	max-width:100%;
	aspect-ratio:16/9;
	margin:14px 0;
	background:#0a0c0e;
	border:1px solid var(--line);
	overflow:hidden;
}
.markdown .md-embed iframe{
	width:100%;
	height:100%;
	border:0;
	display:block;
}
.order-input{max-width:90px}
.page-preview-popup{
	position:fixed;
	z-index:50;
	width:min(420px,calc(100vw - 32px));
	max-height:460px;
	overflow:auto;
	padding:16px;
	background:#101316;
	border:1px solid var(--line-strong);
	box-shadow:var(--shadow);
}
.page-preview-popup h3{margin-top:0;color:var(--text)}
.attachments-admin{margin-top:12px}
.inline-form{
	display:grid;
	grid-template-columns:1fr 1fr 160px auto;
	gap:10px;
	align-items:end;
}
.mini-form{display:flex;gap:8px}
.mini-form input{min-width:160px}
table{width:100%;border-collapse:collapse;background:var(--surface)}
th,td{border-bottom:1px solid var(--line);padding:11px;text-align:left;vertical-align:top}
th{
	color:var(--muted);
	font-size:12px;
	text-transform:uppercase;
	background:#101316;
}
tr:hover td{background:rgba(255,255,255,.018)}
@media(max-width:900px){
	main{padding:16px}
	.vault-ribbon{position:static;width:auto;display:flex;gap:6px;overflow:auto;border-right:0;border-bottom:1px solid var(--line)}
	.vault-ribbon a{white-space:nowrap;margin:0}
	.vault-ribbon+main{margin-left:0}
	.hero,.article-layout,.editor-meta,.editor-grid,.inline-form{grid-template-columns:1fr}
	.editor-meta details{grid-column:span 1}
	header{align-items:flex-start;flex-direction:column}
	.toolbar,.search{align-items:stretch;flex-direction:column}
	.side{position:static}
	button,.button{width:100%}
	nav a,.user-pill{width:100%}
}`
}
